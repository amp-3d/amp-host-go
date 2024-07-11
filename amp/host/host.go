package host

import (
	"fmt"
	"net/url"
	"path"
	"sync"
	"sync/atomic"
	"time"

	space "github.com/amp-3d/amp-host-go/amp/apps/amp.app.space"

	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/registry"
	"github.com/amp-3d/amp-sdk-go/amp/std"
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
		Info: task.Info{
			Label:     host.Opts.Desc,
			IdleClose: time.Nanosecond,
		},
		OnClosed: func() {
			host.Log().Infof(1, "amp.Host shutdown complete")
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

func (host *host) StartNewSession(from amp.HostService, via amp.Transport) (amp.Session, error) {
	sess := &session{
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
		Info: task.Info{
			Label:     "amp.Session",
			IdleClose: time.Nanosecond,
		},
		OnRun: func(ctx task.Context) {
			if err := sess.handleLogin(); err != nil {
				ctx.Log().Warnf("login failed: %v", err)
				ctx.Close()
				return
			}
			sess.consumeInbox()
		},
	})
	if err != nil {
		return nil, err
	}

	sessDesc := fmt.Sprintf("%s(%d)", sess.Log().GetLogLabel(), sess.Info().TID)

	// Start a child contexts for send & recv that drives the inbox & outbox.
	// We start them as children of the HostService, not the Session since we want to keep the stream running until closing completes.
	//
	// Possible paths:
	//   - If the amp.Transport errors out, initiate session.Close()
	//   - If sess.Close() is called elsewhere, and when once complete, <-sessDone will signal and close the amp.Transport.
	from.StartChild(&task.Task{
		Info: task.Info{
			Label:     fmt.Sprint(via.Label(), " <- ", sessDesc),
			IdleClose: time.Nanosecond,
		},
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
								ctx.Log().Warnf("Transport.SendTx() err: %v", err)
							}
						}
					}
				case <-sessDone:
					ctx.Log().Infof(2, "session closed")
					via.Close()
					running = false
				}
			}
		},
	})

	from.StartChild(&task.Task{
		Info: task.Info{
			Label:     fmt.Sprint(via.Label(), " -> ", sessDesc),
			IdleClose: time.Nanosecond,
		},
		OnRun: func(ctx task.Context) {
			sessDone := sess.Done()

			for running := true; running; {
				tx, err := via.RecvTx()
				if err != nil {
					if err == amp.ErrStreamClosed {
						ctx.Log().Info(2, "transport closed")
					} else {
						ctx.Log().Warnf("RecvTx error: %v", err)
					}
					sess.Context.Close()
					running = false
				} else if tx != nil {
					select {
					case sess.txIn <- tx:
					case <-sessDone:
						ctx.Log().Info(2, "session done")
						running = false
					}
				}
			}
		},
	})

	return sess, nil
}

type LoginInfo struct {
	Login   amp.Login // login info
	LoginID tag.ID    // ReqID of the login, allowing session attrs back to the client.
}

// Implements amp.Session and wraps a client / host session.
type session struct {
	task.Context
	amp.Registry // forked registry for this session

	user      LoginInfo            // session user info
	host      *host                // parent host
	txIn      chan *amp.TxMsg      // msgs inbound to this session
	txOut     chan *amp.TxMsg      // msgs outbound from this session
	openOps   map[tag.ID]*clientOp // ReqID maps to an open request.
	openOpsMu sync.Mutex           // protects openOps

	activeAppsMu sync.Mutex
	activeApps   map[tag.ID]*appContext
}

// *** implements amp.Requester ***
type clientOp struct {
	req     amp.Request     // non-nil if this is a pinning request
	outlet  chan *amp.TxMsg // send to this channel to transmit to the request originator
	sess    *session        // parent session
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
		if op.req.StateSync == amp.StateSync_CloseOnSync {
			tx.Status = amp.OpStatus_Closed
		}
	}

	// route this update to the originating request
	tx.SetContextID(op.req.ID)

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

func (sess *session) closeAllReqs() {
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

func (sess *session) closeOp(reqID tag.ID, err error) {
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

func (sess *session) sendClose(reqID tag.ID, err error) {
	var errVal tag.Value
	if err != nil {
		sess.Log().Warnf("closing request %s: %v", reqID.Base32Suffix(), err)
		errVal = amp.ErrorToValue(err)
	}
	amp.SendMetaAttr(sess, reqID, amp.OpStatus_Closed, tag.Nil, errVal)
}

func (sess *session) SendTx(tx *amp.TxMsg) error {

	// If we see a signal for a meta attr, send it to the client's session controller.
	if tx.ContextID().IsNil() {
		tx.SetContextID(sess.user.LoginID)
		tx.Status = amp.OpStatus_Synced
	}

	select {
	case sess.txOut <- tx:
		return nil
	case <-sess.Closing():
		return amp.ErrShuttingDown
	}
}

func (sess *session) AssetPublisher() media.Publisher {
	return sess.host.Opts.AssetServer
}

// var ErrLoginExpected = amp.ErrCode_BadRequest.Error("expected " + sessionLoginSpec.Canonic)
var errBadLogin = amp.ErrCode_LoginFailed.Error("login failed")

func (sess *session) handleLogin() error {
	timer := time.NewTimer(sess.host.Opts.LoginTimeout)

	nextTxValue := func(expectedAttrID tag.ID, out tag.Value) (tag.ID, error) {
		select {
		case tx := <-sess.txIn:
			err := tx.Load(amp.MetaNodeID, expectedAttrID, tag.Nil, out)
			txID := tx.GenesisID()
			tx.ReleaseRef()
			return txID, err
		case <-timer.C:
			return tag.ID{}, amp.ErrTimeout
		case <-sess.Closing():
			return tag.ID{}, amp.ErrShuttingDown
		}
	}

	{
		// --- Login / LoginChallenge
		login := amp.Login{
			UserUID:    &amp.Tag{},
			Checkpoint: &amp.LoginCheckpoint{},
		}
		loginID, err := nextTxValue(std.LoginSpec, &login)
		if err != nil {
			return err
		}

		userID := login.UserUID.TagID()
		if userID.IsNil() {
			userID = tag.FromToken(login.UserUID.Text)
			login.UserUID.SetTagID(userID)
		}
		if userID.IsNil() {
			return errBadLogin
		}

		sess.user.LoginID = loginID
		sess.user.Login = login

		//sess.user.Login.MemberID.SetTagID(tag.FromString(sess.user.Login.UserUID))

		// Bootstrap the space (db service) for the session user so we can get the user's home cell
		_, err = sess.appContextForAppID(space.AppSpec.ID, true)
		if err != nil {
			return err
		}

		challenge := &amp.LoginChallenge{
			Hash: []byte("TODO"), // TODO
		}
		if err = amp.SendMetaAttr(sess, sess.user.LoginID, amp.OpStatus_Synced, std.LoginChallengeSpec, challenge); err != nil {
			return err
		}

		// --- LoginResponse

		var loginResp amp.LoginResponse
		_, err = nextTxValue(std.LoginResponseSpec, &loginResp)
		if err != nil {
			return err
		}

		login.Checkpoint.AuthToken = tag.Now().String()
		if err = amp.SendMetaAttr(sess, sess.user.LoginID, amp.OpStatus_Synced, std.LoginCheckpointSpec, login.Checkpoint); err != nil {
			return err
		}
	}

	return nil
}

func (sess *session) consumeInbox() {
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

/*
// FUTURE -- move the session request handling etc into an amp.App
func (sess *session) handleIncomingOp2(tx *amp.TxMsg) {
	newReqID := tx.GenesisID()

	op, err := sess.addClientOp(newReqID, tx.ContextID())
	if op != nil {
		tx.AddRef()
		op.req.CommitTx = tx
	}

	if err == nil && op != nil {
		err = op.submitToApp()
	}
	if err != nil || op == nil {
		sess.closeOp(newReqID, err)
	}

	tx.ReleaseRef()
}
*/

// Note: assumes ownership of the given tx
func (sess *session) handleIncomingOp(tx *amp.TxMsg) {
	newReqID := tx.GenesisID()

	metaAttr, err := tx.CheckMetaAttr(sess)
	pinReq, _ := metaAttr.(*amp.PinRequest)
	var op *clientOp

	if err != nil {
		// handle err below
	} else if metaAttr == nil {
		op, err = sess.addClientOp(newReqID, tx.ContextID())
		if op != nil {
			tx.AddRef()
			op.req.CommitTx = tx
		}
	} else if pinReq != nil {
		op, err = sess.addClientOp(newReqID, tx.ContextID())
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

func (sess *session) addClientOp(genesisID, parentReqID tag.ID) (*clientOp, error) {

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

	// If this is an internal request (no genesis ID), make a memory outlet to read from
	if op.req.ID.IsNil() {
		op.outlet = make(chan *amp.TxMsg)
	} else {
		if _, exists := sess.openOps[op.req.ID]; exists {
			return nil, amp.ErrCode_BadRequest.Error("request ID already in use")
		}
		op.outlet = sess.txOut
		sess.openOps[op.req.ID] = op
	}

	if parentReqID.IsSet() {
		parentReq := sess.openOps[parentReqID]
		if parentReq == nil {
			return nil, amp.ErrCode_BadRequest.Error("invalid parent context ID")
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
func (sess *session) detachOp(reqID tag.ID) *clientOp {
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

func (sess *session) LoginInfo() amp.Login {
	return sess.user.Login
}

func (sess *session) onAppClosing(appCtx *appContext) {
	sess.activeAppsMu.Lock()
	delete(sess.activeApps, appCtx.module.AppSpec.ID)
	sess.activeAppsMu.Unlock()
}

func (sess *session) GetAppInstance(appID tag.ID, autoCreate bool) (amp.AppInstance, error) {
	if appCtx, err := sess.appContextForAppID(appID, autoCreate); err == nil {
		return appCtx.inst, nil
	} else {
		return nil, err
	}
}

func (sess *session) appContextForAppID(appID tag.ID, autoCreate bool) (*appContext, error) {
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
			instID: tag.Now(),
			inbox:  make(chan *clientOp, 1),
		}

		appCtx.Context, err = sess.StartChild(&task.Task{
			Info: task.Info{
				Label:     appModule.AppSpec.Canonic,
				ContextID: appCtx.instID,
				IdleClose: sess.host.Opts.AppIdleClose,
				DebugMode: sess.host.Opts.DebugMode,
			},
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

func (sess *session) appCtxForInvocation(url *url.URL, autoCreate bool) (*appContext, error) {
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
	sess   *session
	module *amp.App
	inbox  chan *clientOp // incoming client ops for the app
}

func (appCtx *appContext) startOp(op *clientOp) error {

	// If no parent Pin given, the app is responsible for resolving the request to a new Pin.
	pinner := amp.Pinner(op.parent)
	if pinner == nil {
		pinner = appCtx.inst
	} else {
		op.parent = nil // no need to store the parent pin (which may prevent GC)
	}

	var err error
	op.pin, err = pinner.ServeRequest(op)
	return err
}

func (appCtx *appContext) Session() amp.Session {
	return appCtx.sess
}

func (appCtx *appContext) LocalDataPath() string {
	return path.Join(appCtx.sess.host.Opts.StatePath, appCtx.module.AppSpec.Canonic)
}

func (appCtx *appContext) PublishAsset(asset media.Asset, opts media.PublishOpts) (URL string, err error) {
	return appCtx.sess.AssetPublisher().PublishAsset(asset, opts)
}

func (appCtx *appContext) pinAppAttr(appAttrID tag.ID, sync amp.StateSync) (*clientOp, error) {

	// Scope the target attr to the requesting app -- virtualized privacy
	appCell := appCtx.module.AppSpec.ID.With(appAttrID)

	// Make a "local" pin that sends to a channel rather than the session client.
	op, err := appCtx.sess.addClientOp(tag.Nil, tag.Nil)
	if err != nil {
		return nil, err
	}

	op.req.PinRequest = amp.PinRequest{
		StateSync: sync,
		PinTarget: &amp.Tag{
			URL:     "amp://space/~",
			TagID_0: int64(appCell[0]),
			TagID_1: appCell[1],
			TagID_2: appCell[2],
		},
		PinAttrs: []*amp.Tag{
			{
				TagID_0: int64(appAttrID[0]),
				TagID_1: appAttrID[1],
				TagID_2: appAttrID[2],
			},
		},
	}

	return op, nil
}

func (appCtx *appContext) GetAppAttr(appAttrID tag.ID, dst tag.Value) error {

	// When we pin through the runtime, there is no ReqID since this is host-side request -- TODO: uniify with normal pinning
	// Use CloseOnSync and/or pinCtx.Close() when we're done.
	op, err := appCtx.pinAppAttr(appAttrID, amp.StateSync_CloseOnSync)
	if err != nil {
		return err
	}

	if err = op.submitToApp(); err != nil {
		return err
	}
	if err := op.WaitOnReady(); err != nil {
		return err
	}

	for {
		select {
		case tx := <-op.outlet:
			err = tx.LoadFirst(appAttrID, dst)
			tx.ReleaseRef()
			op.pin.Context().Close()
			return err
		case <-op.pin.Context().Closing():
			return amp.ErrCellNotFound
		}
	}
}

func (appCtx *appContext) PutAppAttr(appAttrID tag.ID, src tag.Value) error {

	op, err := appCtx.pinAppAttr(appAttrID, amp.StateSync_None)
	if err != nil {
		return err
	}

	op.req.CommitTx, err = amp.MarshalAttr(op.req.TargetID(), appAttrID, src)
	if err != nil {
		return err
	}

	if err = op.submitToApp(); err != nil {
		return err
	}
	if err := op.WaitOnReady(); err != nil {
		return err
	}

	_, err = op.pin.ServeRequest(op)
	if err != nil {
		return err
	}

	op.pin.Context().Close()
	return nil
}
