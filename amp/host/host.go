package host

import (
	"fmt"
	"net/url"
	"path"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amp-space/amp-host-go/amp/host/registry"
	"github.com/amp-space/amp-host-go/apps/sys/planet"
	"github.com/amp-space/amp-sdk-go/amp"
	"github.com/amp-space/amp-sdk-go/stdlib/symbol"
	"github.com/amp-space/amp-sdk-go/stdlib/task"
	"github.com/amp-space/amp-sdk-go/stdlib/utils"
)

type host struct {
	task.Context
	Opts Opts
}

func startNewHost(opts Opts) (amp.Host, error) {
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
		Opts: opts,
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

func (host *host) Registry() amp.Registry {
	return host.Opts.Registry
}

func (host *host) StartNewSession(from amp.HostService, via amp.Transport) (amp.HostSession, error) {
	sess := &hostSess{
		host:     host,
		msgsIn:   make(chan *amp.Msg),
		msgsOut:  make(chan *amp.Msg, 4),
		openReqs: make(map[uint64]*appReq),
		openApps: make(map[amp.UID]*appContext),
	}

	var err error
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
				case msg := <-sess.msgsOut:
					if msg != nil {
						err := via.SendMsg(msg)
						msg.Reclaim()
						if err != nil {
							if amp.GetErrCode(err) != amp.ErrCode_NotConnected {
								ctx.Warnf("Transport.SendMsg() err: %v", err)
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
				msg, err := via.RecvMsg()
				if err != nil {
					if err == amp.ErrStreamClosed {
						ctx.Info(2, "transport closed")
					} else {
						ctx.Warnf("RecvMsg error: %v", err)
					}
					sess.Context.Close()
					running = false
				} else if msg != nil {
					select {
					case sess.msgsIn <- msg:
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
	amp.SessionRegistry

	nextID     atomic.Uint64      // next CellID to be issued
	host       *host              // parent host
	msgsIn     chan *amp.Msg      // msgs inbound to this hostSess
	msgsOut    chan *amp.Msg      // msgs outbound from this hostSess
	openReqs   map[uint64]*appReq // ReqID maps to an open request.
	openReqsMu sync.Mutex         // protects openReqs

	login      amp.Login
	loginReqID uint64 // ReqID of the login, allowing us to send session attrs back to the client.
	openAppsMu sync.Mutex
	openApps   map[amp.UID]*appContext
}

// *** implements PinContext ***
// IDEA: rather than make this a task.Context, make a sub structure like before?
//   - raw go routines, etc
//   - cleaner code, less hacky for planet app to push tx to subs
//   - CellBase helps implement PinnedCell and contains support code
type appReq struct {
	task.Context
	amp.PinReqParams
	sess      *hostSess
	appCtx    *appContext
	pinned    amp.PinnedCell
	parent    amp.PinnedCell
	cancel    chan struct{}
	pinning   chan struct{} // closed once the cell is pinned
	closed    atomic.Int32
	attrCache map[string]uint32
}

func (req *appReq) App() amp.AppContext {
	return req.appCtx
}

func (req *appReq) GetLogLabel() string {
	var strBuf [128]byte
	str := fmt.Appendf(strBuf[:0], "[req %d] ", req.ReqID)
	if req.URL != nil {
		str = fmt.Append(str, " ", req.URL.String())
	}
	return string(str)
}

func (req *appReq) GetAttrID(attrSpec string) uint32 {
	if attrID, resolved := req.attrCache[attrSpec]; resolved {
		return attrID
	}

	spec, err := req.sess.ResolveAttrSpec(attrSpec, false)
	attrID := spec.DefID
	if err != nil {
		attrID = 0
	}
	req.attrCache[attrSpec] = attrID
	return attrID
}

func (req *appReq) PushUpdate(msg *amp.Msg) error {
	status := msg.Status
	if status == amp.ReqStatus_Synced {
		if (req.PinReq.Flags & amp.PinFlags_CloseOnSync) != 0 {
			status = amp.ReqStatus_Closed
		}
	}
	msg.Status = status

	// route this update to the originating request
	msg.ReqID = req.ReqID

	// If the client backs up, this will back up too which is the desired effect.
	// Otherwise, something like reading from a db reading would quickly fill up the Msg outbox chan (and have no gain)
	// Note that we don't need to check on req.cell or req.sess since if either close, all subs will be closed.
	var err error
	select {
	case req.Outlet <- msg:
	default:
		var closing <-chan struct{}
		if req.Context != nil {
			closing = req.Context.Closing()
		} else {
			closing = req.cancel
		}
		select {
		case req.Outlet <- msg:
		case <-closing:
			err = amp.ErrShuttingDown
		}
	}

	if msg.Status == amp.ReqStatus_Closed {
		req.sess.closeReq(req.ReqID, false, nil)
	}

	return err
}

func (sess *hostSess) closeReq(reqID uint64, sendClose bool, err error) {
	req, _ := sess.getReq(reqID, removeReq)
	if req != nil {
		doClose := req.closed.CompareAndSwap(0, 1)
		if !doClose {
			return
		}
	}

	if sendClose {
		msg := amp.NewMsg()
		msg.ReqID = reqID
		msg.Status = amp.ReqStatus_Closed

		if err != nil {
			elem := &amp.AttrElemPb{
				AttrID: uint64(amp.ConstSymbol_Err.Ord()),
			}
			amp.ErrorToValue(err).MarshalToBuf(&elem.ValBuf)
			msg.CellTxs = append(msg.CellTxs, &amp.CellTxPb{
				Op:    amp.CellTxOp_MetaAttr,
				Elems: []*amp.AttrElemPb{elem},
			})
		}
		sess.SendMsg(msg)
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

func (sess *hostSess) SendMsg(msg *amp.Msg) error {

	// If we see a signal for a meta attr, send it to the client's session controller.
	if msg.ReqID == 0 {
		msg.ReqID = sess.loginReqID
		msg.Status = amp.ReqStatus_Synced
	}

	select {
	case sess.msgsOut <- msg:
		return nil
	case <-sess.Closing():
		return amp.ErrShuttingDown
	}
}

func (sess *hostSess) AssetPublisher() amp.AssetPublisher {
	return sess.host.Opts.AssetServer
}

func (sess *hostSess) handleLogin() error {
	if sess.login.UserUID != "" {
		return amp.ErrCode_LoginFailed.Error("already logged in")
	}

	timer := time.NewTimer(sess.host.Opts.LoginTimeout)
	nextMsg := func() (*amp.Msg, *amp.AttrElemPb, error) {
		select {
		case msg := <-sess.msgsIn:
			attr, err := msg.GetMetaAttr()
			return msg, attr, err
		case <-timer.C:
			return nil, nil, amp.ErrTimeout
		case <-sess.Closing():
			return nil, nil, amp.ErrShuttingDown
		}
	}

	// Wait for login msg
	msg, attr, err := nextMsg()
	if err != nil {
		return err
	}
	sess.loginReqID = msg.ReqID
	if sess.loginReqID == 0 {
		return amp.ErrCode_LoginFailed.Error("missing Req ID")
	}
	if err = sess.login.Unmarshal(attr.ValBuf); err != nil {
		return amp.ErrCode_LoginFailed.Error("failed to unmarshal Login")
	}

	// Importantly, this boots the planet app to mount and start the home planet, which calls InitSessionRegistry()
	_, err = sess.GetAppInstance(planet.AppUID, true)
	if err != nil {
		return err
	}

	{
		chall := &amp.LoginChallenge{
			// TODO
		}
		if err = amp.SendClientMetaAttr(sess, msg.ReqID, chall); err != nil {
			return err
		}
	}

	if msg, attr, err = nextMsg(); err != nil {
		return err
	}
	var loginResp amp.LoginResponse
	if err = loginResp.Unmarshal(attr.ValBuf); err != nil {
		return amp.ErrCode_LoginFailed.Error("failed to unmarshal LoginResponse")
	}

	// TODO: Check challenge response
	{
	}

	sess.closeReq(msg.ReqID, true, err)
	return nil
}

func (sess *hostSess) consumeInbox() {
	for running := true; running; {
		select {

		case msg := <-sess.msgsIn:
			keepOpen := false
			if msg.Status == amp.ReqStatus_Closed {
				sess.closeReq(msg.ReqID, false, nil)
			} else {
				var err error

				metaAttr, _ := msg.GetMetaAttr()
				if metaAttr != nil {
					keepOpen, err = sess.handleMetaAttr(msg.ReqID, metaAttr)
				} else {
					// tx := &amp.MultiTx{}
					// err = tx.UnmarshalFrom(msg, sess.SessionRegistry, false)
					err = amp.ErrUnimplemented // TODO: handle cell update tx
				}
				if err != nil || !keepOpen {
					sess.closeReq(msg.ReqID, true, err)
				}
			}
			msg.Reclaim()

		case <-sess.Closing():
			sess.closeAllReqs()
			running = false
		}
	}
}

func (sess *hostSess) InitSessionRegistry(symTable symbol.Table) {
	if sess.SessionRegistry != nil {
		panic("InitSessionRegistry() already called")
	}
	sess.SessionRegistry = registry.NewSessionRegistry(symTable)
	if err := sess.registry().ExportTo(sess.SessionRegistry); err != nil {
		panic(err)
	}
}

func (sess *hostSess) handleMetaAttr(reqID uint64, attr *amp.AttrElemPb) (keepOpen bool, err error) {

	switch amp.ConstSymbol(attr.AttrID) {
	case amp.ConstSymbol_PinRequest:
		{
			var req *appReq
			req, err = sess.getReq(reqID, clientReq)
			if err != nil {
				return
			}
			if err = req.PinReq.Unmarshal(attr.ValBuf); err != nil {
				return
			}
			keepOpen = true
			err = sess.pinCell(req)
			return
		}
	case amp.ConstSymbol_RegisterDefs:
		{
			var defs amp.RegisterDefs
			if err = defs.Unmarshal(attr.ValBuf); err != nil {
				return
			}
			err = sess.SessionRegistry.RegisterDefs(&defs)
			return
		}
	case amp.ConstSymbol_HandleURI:
		{
			var req amp.HandleURI
			if err = req.Unmarshal(attr.ValBuf); err != nil {
				return
			}
			var uri *url.URL
			uri, err = url.Parse(req.URI)
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
	clientReq pinVerb = iota
	removeReq
	getReq
)

func (sess *hostSess) closeAllReqs() {
	sess.openReqsMu.Lock()
	toClose := make([]uint64, 0, len(sess.openReqs))
	for reqID := range sess.openReqs {
		toClose = append(toClose, reqID)
	}
	sess.openReqsMu.Unlock()

	for _, reqID := range toClose {
		sess.closeReq(reqID, true, amp.ErrShuttingDown)
	}
}

func (sess *hostSess) newAppReq(reqID uint64) *appReq {
	req := &appReq{
		cancel:    make(chan struct{}),
		pinning:   make(chan struct{}),
		attrCache: make(map[string]uint32),
		sess:      sess,
	}
	req.ReqID = reqID
	return req
}

// onReq performs the given pinVerb on given reqID and returns its appReq
func (sess *hostSess) getReq(reqID uint64, verb pinVerb) (req *appReq, err error) {

	sess.openReqsMu.Lock()
	{
		req = sess.openReqs[reqID]
		if req != nil {
			switch verb {
			case removeReq:
				delete(sess.openReqs, reqID)
			case clientReq:
				err = amp.ErrCode_InvalidReq.Error("ReqID already in use")
			}
		} else {
			switch verb {
			case clientReq:
				req = sess.newAppReq(reqID)
				req.Outlet = sess.msgsOut
				sess.openReqs[reqID] = req
			}
		}
	}
	sess.openReqsMu.Unlock()

	return
}

func (sess *hostSess) PinCell(pinReq amp.PinReq) (amp.PinContext, error) {
	req := sess.newAppReq(0)
	req.PinReqParams = *pinReq.Params()

	if err := sess.pinCell(req); err != nil {
		return nil, err
	}

	<-req.pinning
	return req, nil
}

func (sess *hostSess) pinCell(req *appReq) error {
	var err error

	pinReq := &req.PinReq
	req.PinCell = pinReq.CellID()

	// Parse and process the pin request (PinReq)
	if len(pinReq.PinURL) > 0 {
		req.URL, err = url.Parse(pinReq.PinURL)
		if err != nil {
			err = amp.ErrCode_InvalidReq.Errorf("failed to parse PinURL: %v", err)
			return err
		}
	}

	if pinReq.ParentReqID != 0 {
		parentReq, _ := sess.getReq(pinReq.ParentReqID, getReq)
		if parentReq == nil {
			err = amp.ErrCode_InvalidReq.Error("invalid ParentReqID")
			return err
		}
		req.appCtx = parentReq.appCtx
		req.parent = parentReq.pinned
	}

	// If no app context is available, choose an app based on the app invocation (appearing as the hostname in the URL)
	if req.appCtx == nil && req.URL != nil {
		req.appCtx, err = sess.appCtxForInvocation(req.URL.Host, true)
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

func (sess *hostSess) registry() amp.Registry {
	return sess.host.Opts.Registry
}

func (sess *hostSess) LoginInfo() amp.Login {
	return sess.login
}

// func (sess *hostSess) PushMetaAttr(val amp.ElemVal, reqID uint64) error {
// 	attrSpec, err := sess.SessionRegistry.ResolveAttrSpec(val.TypeName(), false)
// 	if err != nil {
// 		sess.Warnf("ResolveAttrSpec() error: %v", err)
// 		return err
// 	}

// 	attrElem := amp.AttrElem{
// 		AttrID: attrSpec.DefID,
// 		Val:    val,
// 	}

// 	if reqID == 0 {
// 		reqID = sess.loginReqID
// 	}
// 	msg, err := attrElem.MarshalToMsg(amp.CellID(reqID))
// 	if err != nil {
// 		return err
// 	}

// 	msg.ReqID = sess.loginReqID
// 	msg.Op = amp.MsgOp_MetaAttr
// 	req, err := sess.getReq(msg.ReqID, getReq)
// 	if req != nil && err == nil {
// 		if !req.PushUpdate(msg) {
// 			err = amp.ErrNotConnected
// 		}
// 	} else {
// 		sess.PushUpdate(msg)
// 	}
// 	return err
// }

func (sess *hostSess) onAppClosing(appCtx *appContext) {
	sess.openAppsMu.Lock()
	delete(sess.openApps, appCtx.module.UID)
	sess.openAppsMu.Unlock()
}

func (sess *hostSess) GetAppInstance(appID amp.UID, autoCreate bool) (amp.AppInstance, error) {
	if ctx, err := sess.appContextForUID(appID, autoCreate); err == nil {
		return ctx.appInst, nil
	} else {
		return nil, err
	}
}

func (sess *hostSess) appContextForUID(appID amp.UID, autoCreate bool) (*appContext, error) {
	sess.openAppsMu.Lock()
	defer sess.openAppsMu.Unlock()

	app := sess.openApps[appID]
	if app == nil && autoCreate {
		appModule, err := sess.registry().GetAppByUID(appID)
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
							req.sess.closeReq(req.ReqID, true, reqErr)
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

		sess.openApps[appModule.UID] = app
	}

	return app, nil
}

func (sess *hostSess) appCtxForInvocation(invocation string, autoCreate bool) (*appContext, error) {
	appModule, err := sess.registry().GetAppForInvocation(invocation)
	if err != nil {
		return nil, err
	}

	ctx, err := sess.appContextForUID(appModule.UID, autoCreate)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

// appContext Implements amp.AppContext
type appContext struct {
	task.Context

	//rng     *rand.Rand
	appInst amp.AppInstance
	sess    *hostSess
	module  *amp.App
	appReqs chan *appReq // incoming requests for the app
}

func (ctx *appContext) IssueCellID() (id amp.CellID) {
	id.AssignFromU64(ctx.sess.nextID.Add(1)+100, 0)
	return
	///return amp.IssueCellID(ctx.rng)
}

/*
func (reg *sessRegistry) IssueTimeID() amp.TimeID {
	now := int64(amp.ConvertToUTC16(time.Now()))
	if !reg.timeMu.CompareAndSwap(0, 1) { // spin lock
		runtime.Gosched()
	}
	issued := reg.timeID
	if issued < now {
		issued = now
	} else {
		issued += 1
	}
	reg.timeID = issued
	reg.timeMu.Store(0) // spin unlock

	return amp.TimeID(issued)
}
*/

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
	// planetApp, err := ctx.sess.GetAppInstance(planet.AppUID, true)
	// if err != nil {
	// 	return nil, err
	// }

	req := &amp.PinReqParams{
		Outlet: make(chan *amp.Msg, 1),
	}
	req.PinReq.Flags = amp.PinFlags_UseNativeSymbols | flags
	req.PinReq.PinURL = fmt.Sprintf("amp://planet/~/%s/.AppAttrs", ctx.module.AppID)

	pinCtx, err := ctx.sess.PinCell(req)
	if err != nil {
		return nil, err
	}
	return pinCtx.(*appReq), nil
}

func (ctx *appContext) GetAppCellAttr(attrSpec string, dst amp.ElemVal) error {
	match, err := ctx.sess.SessionRegistry.ResolveAttrSpec(attrSpec, true)
	if err != nil {
		return err
	}

	// When we pin through the runtime, there is no ReqID since this is host-side.
	// So we use CloseOnSync and/or pinCtx.Close() when we're done.
	pinCtx, err := ctx.PinAppCell(amp.PinFlags_CloseOnSync)
	if err != nil {
		return err
	}

	outlet := pinCtx.Params().Outlet
	for {
		select {
		case msg := <-outlet:
			if len(msg.CellTxs) > 0 && len(msg.CellTxs[0].Elems) > 0 {
				first := msg.CellTxs[0].Elems[0]
				if uint32(first.AttrID) == match.DefID {
					err := dst.Unmarshal(first.ValBuf)
					pinCtx.Close()
					return err
				}
			}
			msg.Reclaim()
		case <-pinCtx.Closing():
			return amp.ErrCellNotFound
		}
	}
}

func (ctx *appContext) PutAppCellAttr(attrSpec string, src amp.ElemVal) error {
	spec, err := ctx.sess.ResolveAttrSpec(attrSpec, true)
	if err != nil {
		return err
	}

	msg, err := amp.FormMetaAttrTx(spec, src)
	if err != nil {
		return err
	}

	pinCtx, err := ctx.PinAppCell(amp.PinFlags_NoSync)
	if err != nil {
		return err
	}

	// TODO: make this better / handle err
	pinCtx.pinned.MergeUpdate(msg)
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
	req.pinned, err = req.appCtx.appInst.PinCell(req.parent, req)
	if err != nil {
		return err
	}

	// Now that we have the target cell pinned, no need to retain a reference to the parent cell
	req.parent = nil

	label := req.LogLabel
	if label == "" {
		label = req.GetLogLabel()
	}

	// FUTURE: switch to ref counting rather than task.Context close detection?
	// Once the cell is resolved, serve state in child context of the PinnedCell
	// Even if the client closes the req before StartChild() (causing it to error), the err result will no op.
	req.Context, err = req.pinned.Context().StartChild(&task.Task{
		TaskRef:   amp.PinContext(req),
		Label:     label,
		IdleClose: time.Microsecond,
		OnRun: func(pinCtx task.Context) {
			err := req.pinned.ServeState(req)
			if err != nil && err != amp.ErrShuttingDown {
				pinCtx.Errorf("ServeState() error: %v", err)
			}
			if (req.PinReq.Flags & amp.PinFlags_CloseOnSync) != 0 {
				pinCtx.Close()
			} else {
				<-pinCtx.Closing()

			}

			// If this is a client req, send a close if needed (no-op if already closed)
			if req.ReqID != 0 {
				req.sess.closeReq(req.ReqID, true, nil)
			}
		},
	})

	// release those waiting on this appReq to be on service
	close(req.pinning)

	return err
}
