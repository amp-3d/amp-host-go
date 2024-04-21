package host

import (
	"fmt"
	"net/url"
	"path"
	"sync"
	"sync/atomic"
	"time"

	app_planet "github.com/amp-3d/amp-host-go/amp/apps/amp-app-planet/planet"

	"github.com/amp-3d/amp-sdk-go/amp"
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
		Registry: RootRegistry(),
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
		host:     host,
		txIn:     make(chan *amp.TxMsg),
		txOut:    make(chan *amp.TxMsg, 4),
		openReqs: make(map[amp.TagID]*appReq),
		openApps: make(map[amp.TagID]*appContext),
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

	host       *host                 // parent host
	txIn       chan *amp.TxMsg       // msgs inbound to this hostSess
	txOut      chan *amp.TxMsg       // msgs outbound from this hostSess
	openReqs   map[amp.TagID]*appReq // ReqID maps to an open request.
	openReqsMu sync.Mutex            // protects openReqs

	login      amp.Login
	loginID    amp.TagID // ReqID of the login, allowing us to send session attrs back to the client.
	openAppsMu sync.Mutex
	openApps   map[amp.TagID]*appContext
}

// PinOp implements PinReq
type pinOp struct {
	amp.PinRequest
	url *url.URL
	//LogLabel string      // info string for logging and debugging
	outlet chan *amp.TxMsg // send to this channel to transmit to the request originator
}

func (op *pinOp) RawRequest() amp.PinRequest {
	return op.PinRequest
}

func (op *pinOp) URL() *url.URL {
	return op.url
}

func (op *pinOp) ContextID() amp.TagID {
	return amp.TagIDFromInts(op.ContextID_0, op.ContextID_1)
}

// *** implements PinContext ***
// IDEA: rather than make this a task.Context, make a sub structure like before?
//   - raw go routines, etc
//   - cleaner code, less hacky for planet app to push tx to subs
//   - CellBase helps implement PinnedCell and contains support code
type appReq struct {
	task.Context
	op      pinOp
	sess    *hostSess
	appCtx  *appContext
	pinned  amp.PinnedCell
	parent  amp.PinnedCell
	cancel  chan struct{}
	pinning chan struct{} // closed once the cell is pinned
	closed  atomic.Int32
}

func (req *appReq) App() amp.AppContext {
	return req.appCtx
}

func (req *appReq) Op() amp.PinOp {
	return &req.op
}

func (req *appReq) MarshalTxOp(dst *amp.TxMsg, op amp.TxOp, val amp.ElemVal) {
	if err := op.Validate(); err != nil {
		req.Warnf("MarshalTxOp: %v", err)
		return
	}
	if err := dst.MarshalOpValue(&op, val); err != nil {
		req.Error(err)
		return
	}
}

func (req *appReq) PushTx(tx *amp.TxMsg) error {
	if tx.Status == amp.ReqStatus_Synced {
		if (req.op.Flags & amp.PinFlags_CloseOnSync) != 0 {
			tx.Status = amp.ReqStatus_Closed
		}
	}

	// route this update to the originating request
	tx.RouteTo_0, tx.RouteTo_1 = req.op.ContextID().ToInts()

	// If the client backs up, this will back up too which is the desired effect.
	// Otherwise, something like reading from a db reading would quickly fill up the Msg outbox chan (and have no gain)
	// Note that we don't need to check on req.cell or req.sess since if either close, all subs will be closed.
	var err error
	select {
	case req.op.outlet <- tx:
	default:
		var closing <-chan struct{}
		if req.Context != nil {
			closing = req.Context.Closing()
		} else {
			closing = req.cancel
		}
		select {
		case req.op.outlet <- tx:
		case <-closing:
			err = amp.ErrShuttingDown
		}
	}

	if tx.Status == amp.ReqStatus_Closed {
		req.sess.autoSendCloseReq(req.op.ContextID(), kSmartClose, nil)
	}

	return err
}

func (sess *hostSess) autoSendCloseReq(routeTo amp.TagID, opts closeMode, err error) {
	if routeTo.IsNil() {
		return
	}
	req, _ := sess.getReq(routeTo, kClosingReq)
	if req != nil {
		doClose := req.closed.CompareAndSwap(0, 1)
		if !doClose {
			return
		}
	} else if opts == kSmartClose {
		return
	}

	if opts == kSmartClose && err != nil {
		val := amp.ErrorToValue(err)
		amp.SendMetaAttr(sess, routeTo, amp.ReqStatus_Closed, val)
	}

	if req != nil {
		// first, remove this req as a sub if applicable
		// if cell := req.cell; cell != nil {
		// 	cell.pl.cancelSub(req)
		// }

		// finally, close the cancel chan now that the close msg has been pushed
		close(req.cancel)

		// TODO: is there is a race condition where the ctx may not be set?
		if req.Context != nil {
			req.Context.Close()
		}
	}

}

func (sess *hostSess) SendTx(tx *amp.TxMsg) error {

	// If we see a signal for a meta attr, send it to the client's session controller.
	if tx.RouteTo_0 == 0 && tx.RouteTo_1 == 0 {
		tx.RouteTo_0, tx.RouteTo_1 = sess.loginID.ToInts()
		tx.Status = amp.ReqStatus_Synced
	}

	select {
	case sess.txOut <- tx:
		return nil
	case <-sess.Closing():
		return amp.ErrShuttingDown
	}
}

func (sess *hostSess) AssetPublisher() amp.AssetPublisher {
	return sess.host.Opts.AssetServer
}

func (sess *hostSess) handleLogin() error {
	if sess.login.UserTagID != "" {
		return amp.ErrCode_LoginFailed.Error("already logged in")
	}

	timer := time.NewTimer(sess.host.Opts.LoginTimeout)
	nextTx := func() (*amp.TxMsg, amp.ElemVal, error) {
		select {
		case msg := <-sess.txIn:
			attr, err := msg.ExtractMetaAttr(sess.Registry)
			return msg, attr, err
		case <-timer.C:
			return nil, nil, amp.ErrTimeout
		case <-sess.Closing():
			return nil, nil, amp.ErrShuttingDown
		}
	}

	// Wait for login msg
	tx, attr, err := nextTx()
	if err != nil {
		return err
	}
	sess.loginID = tx.GenesisID()
	if sess.loginID.IsNil() {
		return amp.ErrCode_LoginFailed.Error("missing genesis ID")
	}
	login, _ := attr.(*amp.Login)
	if login == nil {
		return amp.ErrCode_LoginFailed.Error("failed to unmarshal Login")
	}

	sess.login = *login
	_, err = sess.GetAppInstance(app_planet.AppTagID, true)
	if err != nil {
		return err
	}

	{
		chall := &amp.LoginChallenge{
			// TODO
		}
		if err = amp.SendMetaAttr(sess, tx.RouteTo(), amp.ReqStatus_Closed, chall); err != nil {
			return err
		}
	}

	if tx, attr, err = nextTx(); err != nil {
		return err
	}

	loginResp, _ := attr.(*amp.LoginResponse)
	if loginResp == nil {
		return amp.ErrCode_LoginFailed.Error("failed to unmarshal LoginResponse")
	}

	// TODO: Check challenge response
	{
	}

	sess.autoSendCloseReq(tx.RouteTo(), kSmartClose, err)
	return nil
}

func (sess *hostSess) consumeInbox() {
	for running := true; running; {
		select {

		case tx := <-sess.txIn:
			if tx.Status != amp.ReqStatus_Closed {
				keepOpen, err := sess.handleMetaOp(tx)
				if err != nil || !keepOpen {
					sess.autoSendCloseReq(tx.RouteTo(), kSmartClose, err)
				}
			}
			tx.ReleaseRef()

		case <-sess.Closing():
			sess.closeAllReqs()
			running = false
		}
	}
}

func (sess *hostSess) handleMetaOp(tx *amp.TxMsg) (keepOpen bool, err error) {
	metaAttr, err := tx.ExtractMetaAttr(sess)
	if err != nil {
		return false, err
	}

	switch v := metaAttr.(type) {
	case *amp.PinRequest:
		{
			var req *appReq
			req, err = sess.getReq(tx.RouteTo(), kOpenNewReq)
			if err != nil {
				return
			}
			req.op.PinRequest = *v
			keepOpen = true
			err = sess.pinCell(req)
			return
		}
	case *amp.RegisterDefs:
		{
			err = sess.RegisterDefs(v)
			return
		}
	case *amp.HandleURI:
		{
			var uri *url.URL
			uri, err = url.Parse(v.URI)
			if err != nil {
				return
			}
			var appCtx *appContext
			appCtx, err = sess.appCtxForInvocation(uri.Host, true)
			if err != nil {
				return
			}
			err = appCtx.appInst.HandleURL(uri)
			return
		}
	default:
		err = amp.ErrCode_InvalidReq.Error("unsupported meta attr")
		return
	}

}

type pinVerb int32

const (
	kOpenNewReq pinVerb = iota
	kClosingReq
	kExistingReq
)

type closeMode int32

const (
	kSmartClose closeMode = iota
	kBlindClose
)

func (sess *hostSess) closeAllReqs() {
	sess.openReqsMu.Lock()
	toClose := make([]amp.TagID, 0, len(sess.openReqs))
	for openReq := range sess.openReqs {
		toClose = append(toClose, openReq)
	}
	sess.openReqsMu.Unlock()

	for _, openReqID := range toClose {
		sess.autoSendCloseReq(openReqID, kBlindClose, amp.ErrShuttingDown)
	}
}

func (sess *hostSess) newAppReq() *appReq {
	req := &appReq{
		cancel:  make(chan struct{}),
		pinning: make(chan struct{}),
		sess:    sess,
	}
	return req
}

// onReq performs the given pinVerb on given reqID and returns its appReq
func (sess *hostSess) getReq(routeTo amp.TagID, verb pinVerb) (req *appReq, err error) {

	sess.openReqsMu.Lock()
	{
		req = sess.openReqs[routeTo]
		if req != nil {
			switch verb {
			case kClosingReq:
				delete(sess.openReqs, routeTo)
			case kOpenNewReq:
				err = amp.ErrCode_InvalidReq.Error("ReqID already in use")
			}
		} else {
			switch verb {
			case kOpenNewReq:
				req = sess.newAppReq()
				req.op.ContextID_0, req.op.ContextID_1 = routeTo.ToInts()
				req.op.outlet = sess.txOut
				sess.openReqs[routeTo] = req
			}
		}
	}
	sess.openReqsMu.Unlock()

	return
}

func (sess *hostSess) PinCell(pinReq amp.PinOp) (amp.PinContext, error) {
	req := sess.newAppReq()
	req.op.PinRequest = pinReq.RawRequest()

	if err := sess.pinCell(req); err != nil {
		return nil, err
	}

	<-req.pinning
	return req, nil
}

func (sess *hostSess) pinCell(req *appReq) error {
	var err error

	op := req.op

	// Parse and process the pin request (PinReq)
	if len(op.PinURL) > 0 {
		op.url, err = url.Parse(op.PinURL)
		if err != nil {
			err = amp.ErrCode_InvalidReq.Errorf("failed to parse PinURL: %v", err)
			return err
		}
	}

	contextID := op.ContextID()

	if !contextID.IsNil() {
		parentReq, _ := sess.getReq(contextID, kExistingReq)
		if parentReq == nil {
			err = amp.ErrCode_InvalidReq.Error("invalid ParentReqID")
			return err
		}
		req.appCtx = parentReq.appCtx
		req.parent = parentReq.pinned
	}

	// If no app context is available, choose an app based on the app invocation (appearing as the hostname in the URL)
	if req.appCtx == nil && op.PinURL != "" {
		req.appCtx, err = sess.appCtxForInvocation(op.PinURL, true)
		if err != nil {
			return err
		}
	}

	if req.appCtx == nil {
		err = amp.ErrCode_InvalidReq.Error("unable to resolve cell pinner")
		return err
	}

	req.appCtx.appReqs <- req
	return nil
}

func (sess *hostSess) LoginInfo() amp.Login {
	return sess.login
}

func (sess *hostSess) onAppClosing(appCtx *appContext) {
	sess.openAppsMu.Lock()
	delete(sess.openApps, appCtx.module.TagID)
	sess.openAppsMu.Unlock()
}

func (sess *hostSess) GetAppInstance(appID amp.TagID, autoCreate bool) (amp.AppInstance, error) {
	if ctx, err := sess.appContextForTagID(appID, autoCreate); err == nil {
		return ctx.appInst, nil
	} else {
		return nil, err
	}
}

func (sess *hostSess) appContextForTagID(appID amp.TagID, autoCreate bool) (*appContext, error) {
	sess.openAppsMu.Lock()
	defer sess.openAppsMu.Unlock()

	app := sess.openApps[appID]
	if app == nil && autoCreate {
		appModule, err := sess.GetAppByTagID(appID)
		if err != nil {
			return nil, err
		}

		app = &appContext{
			//rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
			appInst: appModule.NewAppInstance(),
			sess:    sess,
			module:  appModule,
			appReqs: make(chan *appReq, 1),
		}

		_, err = sess.StartChild(&task.Task{
			Label:     "app: " + appModule.AppID,
			IdleClose: sess.host.Opts.AppIdleClose,
			OnStart: func(ctx task.Context) error {
				app.Context = ctx
				return app.appInst.OnNew(app)
			},
			OnRun: func(ctx task.Context) {
				for running := true; running; {
					select {
					case req := <-app.appReqs:
						if reqErr := app.handleAppReq(req); reqErr != nil {
							req.sess.autoSendCloseReq(req.op.ContextID(), kSmartClose, reqErr)
						}
					case <-ctx.Closing():
						running = false
					}
				}
			},
			OnClosing: func() {
				sess.onAppClosing(app)
				app.appInst.OnClosing()
			},
		})
		if err != nil {
			return nil, err
		}

		sess.openApps[appModule.TagID] = app
	}

	return app, nil
}

func (sess *hostSess) appCtxForInvocation(invocation string, autoCreate bool) (*appContext, error) {
	appModule, err := sess.GetAppForInvocation(invocation)
	if err != nil {
		return nil, err
	}

	ctx, err := sess.appContextForTagID(appModule.TagID, autoCreate)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

// appContext Implements amp.AppContext
type appContext struct {
	task.Context

	//rng     *rand.Rand
	seed    uint64
	appInst amp.AppInstance
	sess    *hostSess
	module  *amp.App
	appReqs chan *appReq // incoming requests for the app
}

func (ctx *appContext) IssueTagID() amp.TagID {
	timeID := amp.NewTagID()
	ctx.seed = timeID[1] ^ (ctx.seed * 977973373171)
	return amp.TagID{
		uint64(timeID[0]),
		timeID[1],
		ctx.seed,
	}
}

func (ctx *appContext) Session() amp.HostSession {
	return ctx.sess
}

func (ctx *appContext) LocalDataPath() string {
	return path.Join(ctx.sess.host.Opts.StatePath, ctx.module.AppID)
}

func (ctx *appContext) PublishAsset(asset amp.MediaAsset, opts amp.PublishOpts) (URL string, err error) {
	return ctx.sess.AssetPublisher().PublishAsset(asset, opts)
}

func (ctx *appContext) PinAppCell(flags amp.PinFlags) (*appReq, error) {
	// planetApp, err := ctx.sess.GetAppInstance(planet.AppTagID, true)
	// if err != nil {
	// 	return nil, err
	// }

	op := &pinOp{
		PinRequest: amp.PinRequest{
			Flags:  flags,
			PinURL: fmt.Sprintf("amp://planet/~/%s/.AppAttrs", ctx.module.AppID),
		},
		outlet: make(chan *amp.TxMsg, 1),
	}

	pinCtx, err := ctx.sess.PinCell(op)
	if err != nil {
		return nil, err
	}
	return pinCtx.(*appReq), nil
}

func (ctx *appContext) GetAppAttr(tagSpec string, dst amp.ElemVal) error {
	attrID, err := amp.FormTagSpecID(tagSpec)
	if err != nil {
		return err
	}

	// When we pin through the runtime, there is no ReqID since this is host-side.
	// So we use CloseOnSync and/or pinCtx.Close() when we're done.
	pinReq, err := ctx.PinAppCell(amp.PinFlags_CloseOnSync)
	if err != nil {
		return err
	}

	outlet := pinReq.op.outlet
	for {
		select {
		case tx := <-outlet:
			for i, op := range tx.Ops {
				if op.AttrID == attrID {
					err = tx.UnmarshalOpValue(i, dst)
					pinReq.Close()
					return err
				}
			}
			tx.ReleaseRef()
		case <-pinReq.Closing():
			return amp.ErrCellNotFound
		}
	}
}

func (ctx *appContext) PutAppAttr(attrSpec string, src amp.ElemVal) error {
	tx, err := amp.MarshalMetaAttr(attrSpec, src)
	if err != nil {
		return err
	}

	pinCtx, err := ctx.PinAppCell(amp.PinFlags_NoSync)
	if err != nil {
		return err
	}

	// TODO: make this better / handle err
	pinCtx.pinned.MergeTx(tx)
	// if err != nil {
	// 	return err
	// }

	pinCtx.Close()
	return nil
}

func (app *appContext) handleAppReq(req *appReq) error {

	// TODO: resolve to a generic Cell and *then* pin it?
	// Also, use pinned cell ref counting to know when it it safe to idle close?
	var err error
	req.pinned, err = req.appCtx.appInst.PinCell(req.parent, &req.op)
	if err != nil {
		return err
	}

	// Now that we have the target cell pinned, no need to retain a reference to the parent cell
	req.parent = nil

	// FUTURE: switch to ref counting rather than task.Context close detection?
	// Once the cell is resolved, serve state in child context of the PinnedCell
	// Even if the client closes the req before StartChild() (causing it to error), the err result will no op.
	req.Context, err = req.pinned.Context().StartChild(&task.Task{
		TaskRef:   req.op.ContextID(),
		Label:     req.op.FormLogLabel(),
		IdleClose: time.Microsecond,
		OnRun: func(pinCtx task.Context) {
			err := req.pinned.ServeState(req)
			if err != nil && err != amp.ErrShuttingDown {
				pinCtx.Errorf("ServeState() error: %v", err)
			}
			if (req.op.Flags & amp.PinFlags_CloseOnSync) != 0 {
				pinCtx.Close()
			} else {
				<-pinCtx.Closing()

			}

			// If this is a client req, send a close if needed (no-op if already closed)
			req.sess.autoSendCloseReq(req.op.ContextID(), kSmartClose, nil)
		},
	})

	// release those waiting on this appReq to be on service
	close(req.pinning)

	return err
}
