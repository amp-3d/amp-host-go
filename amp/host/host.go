package host

import (
	"fmt"
	"net/url"
	"path"
	"sync"
	"sync/atomic"
	"time"

	planet "github.com/amp-3d/amp-host-go/amp/apps/amp-app-planet"

	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/registry"
	"github.com/amp-3d/amp-sdk-go/stdlib/media"
	"github.com/amp-3d/amp-sdk-go/stdlib/tag"
	"github.com/amp-3d/amp-sdk-go/stdlib/task"
	"github.com/amp-3d/amp-sdk-go/stdlib/utils"
)

type host struct {
	amp.Registry // registry local to this host -- forked from the global registry
	task.Context
	Opts Opts
}

// StartNewHost starts a new host with the given opts
func StartNewHost(opts Opts) (amp.Host, error) {
	var err error
	if opts.StatePath, err = utils.ExpandAndCheckPath(opts.StatePath, true); err != nil {
		return nil, err
	}
	if opts.CachePath == "" {
		opts.CachePath = path.Join(path.Dir(opts.StatePath), "_.archost-cache")
	}
	if opts.CachePath, err = utils.ExpandAndCheckPath(opts.CachePath, true); err != nil {
		return nil, err
	}

	host := &host{
		Opts:     opts,
		Registry: registry.Global(),
	}

	host.Context, err = task.Start(&task.Task{
		Label:     host.Opts.Desc,
		IdleClose: time.Nanosecond,
		OnClosed: func() {
			host.Info(1, "amp.Host shutdown complete")
		},
	})
	if err != nil {
		return nil, err
	}

	err = host.Opts.AssetServer.StartService(host.Context)
	if err != nil {
		host.Close()
		return nil, err
	}

	return host, nil
}

func (host *host) HostRegistry() amp.Registry {
	return host.Registry
}

func (host *host) StartNewSession(from amp.HostService, via amp.Transport) (amp.HostSession, error) {
	sess := &hostSess{
		host:       host,
		txIn:       make(chan *amp.TxMsg),
		txOut:      make(chan *amp.TxMsg, 4),
		openOps:    make(map[tag.ID]*clientOp),
		activeApps: make(map[tag.ID]*appContext),
	}

	sess.Registry = amp.NewRegistry()
	err := sess.Registry.Import(host.Registry)
	if err != nil {
		return nil, err
	}

	sess.Context, err = host.StartChild(&task.Task{
		Label:     "amp.HostSession",
		IdleClose: time.Nanosecond,
		OnRun: func(ctx task.Context) {
			if err := sess.handleLogin(); err != nil {
				ctx.Warnf("login failed: %v", err)
				ctx.Close()
				return
			}
			sess.consumeInbox()
		},
	})
	if err != nil {
		return nil, err
	}

	sessDesc := fmt.Sprintf("%s(%d)", sess.Label(), sess.ContextID())

	// Start a child contexts for send & recv that drives hostSess the inbox & outbox.
	// We start them as children of the HostService, not the HostSession since we want to keep the stream running until hostSess completes closing.
	//
	// Possible paths:
	//   - If the amp.Transport errors out, initiate hostSess.Close()
	//   - If sess.Close() is called elsewhere, and when once complete, <-sessDone will signal and close the amp.Transport.
	from.StartChild(&task.Task{
		Label:     fmt.Sprint(via.Label(), " <- ", sessDesc),
		IdleClose: time.Nanosecond,
		OnRun: func(ctx task.Context) {
			sessDone := sess.Done()

			// Forward outgoing msgs from host to stream outlet until the host session says its completely done.
			for running := true; running; {
				select {
				case tx := <-sess.txOut:
					if tx != nil {
						err := via.SendTx(tx)
						tx.ReleaseRef()
						if err != nil {
							if amp.GetErrCode(err) != amp.ErrCode_NotConnected {
								ctx.Warnf("Transport.SendTx() err: %v", err)
							}
						}
					}
				case <-sessDone:
					ctx.Info(2, "session closed")
					via.Close()
					running = false
				}
			}
		},
	})

	from.StartChild(&task.Task{
		Label:     fmt.Sprint(via.Label(), " -> ", sessDesc),
		IdleClose: time.Nanosecond,
		OnRun: func(ctx task.Context) {
			sessDone := sess.Done()

			for running := true; running; {
				tx, err := via.RecvTx()
				if err != nil {
					if err == amp.ErrStreamClosed {
						ctx.Info(2, "transport closed")
					} else {
						ctx.Warnf("RecvTx error: %v", err)
					}
					sess.Context.Close()
					running = false
				} else if tx != nil {
					select {
					case sess.txIn <- tx:
					case <-sessDone:
						ctx.Info(2, "session done")
						running = false
					}
				}
			}
		},
	})

	return sess, nil
}

// hostSess wraps a session the parent host has with a client.
type hostSess struct {
	task.Context
	amp.Registry // forked registry for this session

	host      *host                // parent host
	txIn      chan *amp.TxMsg      // msgs inbound to this hostSess
	txOut     chan *amp.TxMsg      // msgs outbound from this hostSess
	openOps   map[tag.ID]*clientOp // ReqID maps to an open request.
	openOpsMu sync.Mutex           // protects openOps

	auth         amp.Login
	loginID      tag.ID // ReqID of the login, allowing session attrs back to the client.
	activeAppsMu sync.Mutex
	activeApps   map[tag.ID]*appContext
}

// *** implements amp.Requester ***
type clientOp struct {
	req     amp.Request     // non-nil if this is a pinning request
	outlet  chan *amp.TxMsg // send to this channel to transmit to the request originator
	sess    *hostSess       // parent session
	appCtx  *appContext     // app handling this request
	pin     amp.Pin         // request executing (as a Pin)
	parent  amp.Pin         // parent Pin, if any
	started chan struct{}   // closed when the op is on service
	cancel  chan struct{}   // closed when the request is closed
	closed  atomic.Int32    // 0 = open, 1 = closed
}

func (op *clientOp) Request() *amp.Request {
	return &op.req
}

func (op *clientOp) pinClosed() <-chan struct{} {
	if op.pin != nil {
		return op.pin.Context().Closing()
	}
	return nil
}

func (op *clientOp) WaitOnReady() error {
	<-op.started

	select {
	case <-op.cancel:
		return amp.ErrRequestClosed
	case <-op.pinClosed():
		return amp.ErrRequestClosed
	default:
		return nil
	}
}

func (op *clientOp) PushTx(tx *amp.TxMsg) error {
	if tx.Status == amp.OpStatus_Synced {
		if op.req.PinSync == amp.PinSync_CloseOnSync {
			tx.Status = amp.OpStatus_Closed
		}
	}

	// route this update to the originating request
	tx.SetRequestID(op.req.ID)

	// If the client backs up, this will back up too which is the desired effect.
	// Otherwise, something like reading from a db reading would quickly fill up the Msg outbox chan (and have no gain)
	// Note that we don't need to check on op.cell or op.sess since if either close, all subs will be closed.
	var err error
	select {
	case op.outlet <- tx:
	default:
		select {
		case op.outlet <- tx:
		case <-op.cancel:
			err = amp.ErrRequestClosed
		case <-op.pinClosed():
			err = amp.ErrRequestClosed
		}
	}

	if tx.Status == amp.OpStatus_Closed {
		op.OnComplete(nil)
	}

	return err
}

func (op *clientOp) FormLogLabel() string {
	var strBuf [128]byte
	str := fmt.Appendf(strBuf[:0], "[req_%s] ", op.req.ID.Base32Suffix())
	if target := op.req.PinTarget; target != nil {
		if target.URL != "" {
			str = fmt.Append(str, target.URL)
		}
	}
	return string(str)
}

func (op *clientOp) submitToApp() error {

	{
		pinTarget := op.req.PinTarget
		if pinTarget != nil && pinTarget.URL != "" {
			var err error
			if op.req.URL, err = url.Parse(pinTarget.URL); err != nil {
				err = amp.ErrCode_BadRequest.Errorf("error parsing URL: %v", err)
				return err
			}
			if op.req.Values, err = url.ParseQuery(op.req.URL.RawQuery); err != nil {
				err = amp.ErrCode_BadRequest.Errorf("error parsing URL query: %v", err)
				return err
			}
		}
	}

	// If originating context is available, choose an app based on the app invocation (appearing as the hostname in the URL)
	if op.appCtx == nil {
		var err error
		if op.appCtx, err = op.sess.appCtxForInvocation(op.req.URL, true); err != nil {
			return err
		}
	}
	op.appCtx.inbox <- op
	return nil
}

func (op *clientOp) WasClosed() bool {
	return op.closed.Load() != 0
}

func (op *clientOp) OnComplete(err error) {
	op.sess.closeOp(op.req.ID, err)
}

func (sess *hostSess) closeAllReqs() {
	sess.openOpsMu.Lock()
	toClose := make([]tag.ID, 0, len(sess.openOps))
	for reqID := range sess.openOps {
		toClose = append(toClose, reqID)
	}
	sess.openOpsMu.Unlock()

	for _, reqID := range toClose {
		sess.closeOp(reqID, amp.ErrShuttingDown)
	}
}

func (sess *hostSess) closeOp(reqID tag.ID, err error) {
	op := sess.detachOp(reqID)
	if op == nil {
		return
	}

	proceed := op.closed.CompareAndSwap(0, 1)
	if !proceed {
		return
	}
	sess.sendClose(reqID, err)
	close(op.cancel)

	// TODO: is there is a race condition where the ctx may not be set?
	if op.pin != nil {
		op.pin.Context().Close()
	}

	if op.req.CommitTx != nil {
		op.req.CommitTx.ReleaseRef()
		op.req.CommitTx = nil
	}
}

func (sess *hostSess) sendClose(reqID tag.ID, err error) {
	var errVal amp.ElemVal
	if err != nil {
		sess.Warnf("request failed %s: %v", reqID, err)
		errVal = amp.ErrorToValue(err)
	}
	amp.SendMetaAttr(sess, reqID, amp.OpStatus_Closed, errVal)
}

func (sess *hostSess) SendTx(tx *amp.TxMsg) error {

	// If we see a signal for a meta attr, send it to the client's session controller.
	if tx.RequestID().IsNil() {
		tx.SetRequestID(sess.loginID)
		tx.Status = amp.OpStatus_Synced
	}

	select {
	case sess.txOut <- tx:
		return nil
	case <-sess.Closing():
		return amp.ErrShuttingDown
	}
}

func (sess *hostSess) AssetPublisher() media.Publisher {
	return sess.host.Opts.AssetServer
}

func (sess *hostSess) handleLogin() error {
	if sess.auth.Checkpoint.Session != nil {
		return amp.ErrCode_LoginFailed.Error("already logged in")
	}

	timer := time.NewTimer(sess.host.Opts.LoginTimeout)
	nextTx := func() (*amp.TxMsg, amp.ElemVal, error) {
		select {
		case tx := <-sess.txIn:
			attr, err := tx.CheckMetaAttr(sess.Registry)
			if attr == nil && err == nil {
				err = amp.ErrCode_BadRequest.Error("missing meta attr")
			}
			return tx, attr, err
		case <-timer.C:
			return nil, nil, amp.ErrTimeout
		case <-sess.Closing():
			return nil, nil, amp.ErrShuttingDown
		}
	}

	// Login / LoginChallenge
	{
		tx, attr, err := nextTx()
		if err != nil {
			return err
		}
		defer tx.ReleaseRef()

		sess.loginID = tx.RequestID()
		if sess.loginID.IsNil() {
			return amp.ErrCode_LoginFailed.Error("missing login Tag")
		}
		loginReq, _ := attr.(*amp.Login)
		if loginReq == nil {
			return amp.ErrCode_LoginFailed.Error("failed to unmarshal Login")
		}

		sess.auth = amp.Login{
			Checkpoint: &amp.AuthCheckpoint{
				Session:  &amp.Tag{},
				Member:   &amp.Tag{},
				HomeFeed: &amp.Tag{
					// TODO
				},
			},
		}

		sess.auth.Checkpoint.Session.SetTagID(sess.loginID)
		sess.auth.Checkpoint.Member.SetTagID(tag.FromString(loginReq.UserUID))

		// Bootstrap the planet (db service) for the session user so we can get the user's home cell
		_, err = sess.appContextForAppID(planet.AppSpec.ID, true)
		if err != nil {
			return err
		}

		chall := &amp.LoginChallenge{
			Hash: []byte("TODO"), // TODO
		}
		if err = amp.SendMetaAttr(sess, sess.loginID, amp.OpStatus_Synced, chall); err != nil {
			return err
		}
	}

	// LoginResponse
	{
		tx, attr, err := nextTx()
		if err != nil {
			return err
		}
		defer tx.ReleaseRef()

		loginResp, _ := attr.(*amp.LoginResponse)
		if loginResp == nil {
			return amp.ErrCode_LoginFailed.Error("failed to unmarshal LoginResponse")
		}

		{
			if err = amp.SendMetaAttr(sess, sess.loginID, amp.OpStatus_Synced, &sess.auth); err != nil {
				return err
			}
		}
	}

	//sess.closeOp(sess.loginID, kSmartClose, err)  not needed since login is not a
	return nil
}

func (sess *hostSess) consumeInbox() {
	for running := true; running; {
		select {

		case tx := <-sess.txIn:
			if tx.Status != amp.OpStatus_Closed {
				sess.handleIncomingOp(tx)
			}

		case <-sess.Closing():
			sess.closeAllReqs()
			running = false
		}
	}
}

// Note: assumes ownership of the given tx
func (sess *hostSess) handleIncomingOp(tx *amp.TxMsg) {
	newReqID := tx.GenesisID()

	metaAttr, err := tx.CheckMetaAttr(sess)
	pinReq, _ := metaAttr.(*amp.PinRequest)
	var op *clientOp

	if err != nil {
		// handle err below
	} else if metaAttr == nil {
		op, err = sess.addClientOp(newReqID, tx.RequestID())
		if op != nil {
			tx.AddRef()
			op.req.CommitTx = tx
		}
	} else if pinReq != nil {
		op, err = sess.addClientOp(newReqID, tx.RequestID())
		if op != nil {
			op.req.PinRequest = *pinReq
		}
	} else {
		err = amp.ErrCode_BadRequest.Error("unsupported meta attr")
	}

	if err == nil && op != nil {
		err = op.submitToApp()
	}
	if err != nil || op == nil {
		sess.closeOp(newReqID, err)
	}

	tx.ReleaseRef()
}

func (sess *hostSess) addClientOp(genesisID, parentID tag.ID) (*clientOp, error) {

	op := &clientOp{
		req: amp.Request{
			ID: genesisID,
		},
		cancel:  make(chan struct{}),
		started: make(chan struct{}),
		sess:    sess,
	}

	sess.openOpsMu.Lock()
	defer sess.openOpsMu.Unlock()

	// If this is an internal request, make a memory outlet to read from
	if op.req.ID.IsNil() {
		op.outlet = make(chan *amp.TxMsg)
	} else {
		if _, exists := sess.openOps[op.req.ID]; exists {
			return nil, amp.ErrCode_BadRequest.Error("request ID already in use")
		}
		op.outlet = sess.txOut
		sess.openOps[op.req.ID] = op
	}

	if parentID.IsSet() {
		parentReq := sess.openOps[parentID]
		if parentReq == nil {
			return nil, amp.ErrCode_BadRequest.Error("invalid parent / context ID")
		}
		if err := parentReq.WaitOnReady(); err != nil {
			return nil, amp.ErrCode_BadRequest.Errorf("parent request not ready: %v", err)
		}
		op.appCtx = parentReq.appCtx
		op.parent = parentReq.pin
	}

	return op, nil
}

// onReq performs the given pinVerb on given reqID and returns its clientOp
func (sess *hostSess) detachOp(reqID tag.ID) *clientOp {
	if reqID.IsNil() {
		return nil
	}
	sess.openOpsMu.Lock()
	op := sess.openOps[reqID]
	if op != nil {
		delete(sess.openOps, reqID)
	}
	sess.openOpsMu.Unlock()
	return op
}

func (sess *hostSess) Auth() amp.Login {
	return sess.auth
}

func (sess *hostSess) onAppClosing(appCtx *appContext) {
	sess.activeAppsMu.Lock()
	delete(sess.activeApps, appCtx.module.AppSpec.ID)
	sess.activeAppsMu.Unlock()
}

func (sess *hostSess) GetAppInstance(appID tag.ID, autoCreate bool) (amp.AppInstance, error) {
	if appCtx, err := sess.appContextForAppID(appID, autoCreate); err == nil {
		return appCtx.inst, nil
	} else {
		return nil, err
	}
}

func (sess *hostSess) appContextForAppID(appID tag.ID, autoCreate bool) (*appContext, error) {
	sess.activeAppsMu.Lock()
	defer sess.activeAppsMu.Unlock()

	appCtx := sess.activeApps[appID]
	if appCtx == nil && autoCreate {
		appModule, err := sess.GetAppByTag(appID)
		if err != nil {
			return nil, err
		}

		appCtx = &appContext{
			sess:   sess,
			module: appModule,
			instID: tag.New(),
			inbox:  make(chan *clientOp, 1),
		}

		appCtx.Context, err = sess.StartChild(&task.Task{
			Label:     appModule.AppSpec.Canonic,
			ID:        appCtx.instID,
			IdleClose: sess.host.Opts.AppIdleClose,
			OnStart: func(ctx task.Context) error {
				var createErr error
				appCtx.inst, createErr = appModule.NewAppInstance(appCtx)
				return createErr
			},
			OnRun: func(ctx task.Context) {
				for running := true; running; {
					select {
					case op := <-appCtx.inbox:
						if reqErr := appCtx.startOp(op); reqErr != nil {
							op.OnComplete(reqErr)
						} else {
							close(op.started) // let others know when the op is on service
						}
					case <-ctx.Closing():
						running = false
					}
				}
			},
			OnClosing: func() {
				appCtx.inst.OnClosing()
				sess.onAppClosing(appCtx)
			},
		})
		if err != nil {
			return nil, err
		}

		sess.activeApps[appID] = appCtx
	}

	return appCtx, nil
}

func (sess *hostSess) appCtxForInvocation(url *url.URL, autoCreate bool) (*appContext, error) {
	if url == nil {
		return nil, amp.ErrCode_BadRequest.Error("missing URL")
	}
	appModule, err := sess.GetAppForInvocation(url.Host)
	if err != nil {
		return nil, err
	}
	appCtx, err := sess.appContextForAppID(appModule.AppSpec.ID, autoCreate)
	if err != nil {
		return nil, err
	}
	return appCtx, nil
}

// appContext Implements amp.AppContext
type appContext struct {
	task.Context
	inst   amp.AppInstance
	instID tag.ID
	sess   *hostSess
	module *amp.App
	inbox  chan *clientOp // incoming client ops for the app
}

func (appCtx *appContext) startOp(op *clientOp) error {
	err := appCtx.inst.MakeReady(op)
	if err != nil {
		return err
	}

	// If no parent Pin given, the app is responsible for resolving the request to a new Pin.
	pinner := amp.Pinner(op.parent)
	if pinner == nil {
		pinner = appCtx.inst
	} else {
		op.parent = nil // no need to store the parent pin (which may prevent GC)
	}

	op.pin, err = pinner.ServeRequest(op)
	return err
}

func (appCtx *appContext) Session() amp.HostSession {
	return appCtx.sess
}

func (appCtx *appContext) LocalDataPath() string {
	return path.Join(appCtx.sess.host.Opts.StatePath, appCtx.module.AppSpec.Canonic)
}

func (appCtx *appContext) PublishAsset(asset media.Asset, opts media.PublishOpts) (URL string, err error) {
	return appCtx.sess.AssetPublisher().PublishAsset(asset, opts)
}

var appCellTag = tag.FromToken("AppCell")

func (appCtx *appContext) pinAppAttr(appAttr tag.ID, sync amp.PinSync) (*clientOp, error) {

	// Make a "local" pin that sends to a channel rather than the session client.
	op, err := appCtx.sess.addClientOp(tag.Nil, tag.Nil)
	if err != nil {
		return nil, err
	}

	// Scope the target attr to the requesting app -- virtualized privacy
	appCell := appCtx.module.AppSpec.ID.With(appCellTag)

	op.req.PinRequest = amp.PinRequest{
		PinSync: sync,
		PinTarget: &amp.Tag{
			URL:     "amp://planet/~",
			TagID_0: int64(appCell[0]),
			TagID_1: appCell[1],
			TagID_2: appCell[2],
		},
		PinAttrs: []*amp.Tag{
			{
				TagID_0: int64(appAttr[0]),
				TagID_1: appAttr[1],
				TagID_2: appAttr[2],
			},
		},
	}

	if err = op.submitToApp(); err != nil {
		return nil, err
	}

	if err := op.WaitOnReady(); err != nil {
		return nil, err
	}

	return op, nil
}

func (appCtx *appContext) GetAppAttr(appAttr tag.ID, dst amp.ElemVal) error {

	// When we pin through the runtime, there is no ReqID since this is host-side request -- TODO: uniify with normal pinning
	// Use CloseOnSync and/or pinCtx.Close() when we're done.
	op, err := appCtx.pinAppAttr(appAttr, amp.PinSync_CloseOnSync)
	if err != nil {
		return err
	}

	for {
		select {
		case tx := <-op.outlet:
			for i, txOp := range tx.Ops {
				if txOp.AttrID == appAttr {
					err = tx.UnmarshalOpValue(i, dst)
					op.pin.Context().Close()
					tx.ReleaseRef()
					return err
				}
			}
			tx.ReleaseRef()
		case <-op.pin.Context().Closing():
			return amp.ErrCellNotFound
		}
	}
}

func (appCtx *appContext) PutAppAttr(appAttr tag.ID, src amp.ElemVal) error {
	tx, err := amp.MarshalMetaAttr(appAttr, src)
	if err != nil {
		return err
	}

	op, err := appCtx.pinAppAttr(appAttr, amp.PinSync_None)
	if err != nil {
		return err
	}

	op.req.CommitTx = tx

	// TODO: make this better / handle err
	op.pin.ServeRequest(op)
	// if err != nil {
	// 	return err
	// }

	op.pin.Context().Close()
	return nil
}
