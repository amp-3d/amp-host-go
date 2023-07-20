package archost

import (
	"fmt"
	"net/url"
	"path"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/symbol"
	"github.com/arcspace/go-arc-sdk/stdlib/task"
	"github.com/arcspace/go-arc-sdk/stdlib/utils"
	"github.com/arcspace/go-archost/arc/apps/std_family/planet"
	"github.com/arcspace/go-archost/arc/archost/registry"
)

type host struct {
	task.Context
	Opts Opts
}

func startNewHost(opts Opts) (arc.Host, error) {
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
			host.Info(1, "arc.Host shutdown complete")
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

func (host *host) Registry() arc.Registry {
	return host.Opts.Registry
}

func (host *host) StartNewSession(from arc.HostService, via arc.Transport) (arc.HostSession, error) {
	sess := &hostSess{
		host:     host,
		msgsIn:   make(chan *arc.Msg),
		msgsOut:  make(chan *arc.Msg, 4),
		openReqs: make(map[uint64]*reqContext),
		openApps: make(map[arc.UID]*appContext),
	}

	var err error
	sess.Context, err = host.StartChild(&task.Task{
		Label:     "HostSession",
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
	//   - If the arc.Transport errors out, initiate hostSess.Close()
	//   - If sess.Close() is called elsewhere, and when once complete, <-sessDone will signal and close the arc.Transport.
	from.StartChild(&task.Task{
		Label:     fmt.Sprint(via.Desc(), " <- ", sessDesc),
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
						if err != nil /*&& err != TransportClosed */ {
							ctx.Warnf("Transport.SendMsg() err: %v", err)
						}
					}
				case <-sessDone:
					ctx.Info(2, "<-hostDone")
					via.Close()
					running = false
				}
			}
		},
	})

	from.StartChild(&task.Task{
		Label:     fmt.Sprint(via.Desc(), " -> ", sessDesc),
		IdleClose: time.Nanosecond,
		OnRun: func(ctx task.Context) {
			sessDone := sess.Done()

			for running := true; running; {
				msg, err := via.RecvMsg()
				if err != nil {
					if err == arc.ErrStreamClosed {
						ctx.Info(2, "Transport closed")
					} else {
						ctx.Warnf("RecvMsg() error: %v", err)
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
	arc.SessionRegistry

	nextID     atomic.Uint64          // next CellID to be issued
	host       *host                  // parent host
	msgsIn     chan *arc.Msg          // msgs inbound to this hostSess
	msgsOut    chan *arc.Msg          // msgs outbound from this hostSess
	openReqs   map[uint64]*reqContext // ReqID maps to an open request.
	openReqsMu sync.Mutex             // protects openReqs

	login      arc.Login
	loginReqID uint64 // ReqID of the login, allowing us to send session attrs back to the client.
	openAppsMu sync.Mutex
	openApps   map[arc.UID]*appContext
}

// *** implements PinContext ***
type reqContext struct {
	task.Context
	arc.PinReqParams
	sess    *hostSess
	appCtx  *appContext
	pinned  arc.PinnedCell
	cancel  chan struct{}
	started chan struct{} // closed once the cell is on service
	closed  atomic.Int32
}

func (req *reqContext) App() arc.AppContext {
	return req.appCtx
}

func (req *reqContext) GetLogLabel() string {
	var strBuf [128]byte

	str := fmt.Appendf(strBuf[:0], "req %d", req.ReqID)
	if req.URL != nil {
		str = fmt.Append(str, ": ", req.URL.String())
	}
	return string(str)
}

func (req *reqContext) PushUpdate(msg *arc.Msg) error {
	status := msg.Status
	if status == arc.ReqStatus_Synced {
		if (req.PinReq.Flags & arc.PinFlags_CloseOnSync) != 0 {
			status = arc.ReqStatus_Closed
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
			err = arc.ErrShuttingDown
		}
	}

	if msg.Status == arc.ReqStatus_Closed {
		req.sess.closeReq(req.ReqID, false, nil)
	}

	return err
}

func (req *reqContext) closeReq(sendClose bool, err error) {
	if req == nil {
		return
	}
	req.sess.closeReq(req.ReqID, sendClose, err)
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
		msg := arc.NewMsg()
		msg.ReqID = reqID
		msg.Status = arc.ReqStatus_Closed

		if err != nil {
			elem := &arc.AttrElemPb{
				AttrID: uint64(arc.ConstSymbol_Err.Ord()),
			}
			arc.ErrorToValue(err).MarshalToBuf(&elem.ValBuf)
			msg.CellTxs = append(msg.CellTxs, &arc.CellTxPb{
				Op:    arc.CellTxOp_MetaAttr,
				Elems: []*arc.AttrElemPb{elem},
			})
		}
		sess.SendMsg(msg)
	}

	if req != nil {
		// first, remove this req as a sub if applicable
		// if cell := req.cell; cell != nil {
		// 	cell.pl.cancelSub(req)
		// }

		req.sess.closeReq(req.ReqID, sendClose, err)

		// finally, close the cancel chan now that the close msg has been pushed
		close(req.cancel)

		// TODO: there is a race condition where the ctx could be set
		if req.Context != nil {
			req.Context.Close()
		}
	}

}

func (sess *hostSess) SendMsg(msg *arc.Msg) error {

	// If we see a signal for a meta attr, send it to the client's session controller.
	if msg.ReqID == 0 {
		msg.ReqID = sess.loginReqID
		msg.Status = arc.ReqStatus_Synced
	}

	select {
	case sess.msgsOut <- msg:
		return nil
	case <-sess.Closing():
		return arc.ErrShuttingDown
	}
}

func (sess *hostSess) AssetPublisher() arc.AssetPublisher {
	return sess.host.Opts.AssetServer
}

func (sess *hostSess) handleLogin() error {
	if sess.login.UserUID != "" {
		return arc.ErrCode_LoginFailed.Error("already logged in")
	}

	timer := time.NewTimer(sess.host.Opts.LoginTimeout)
	nextMsg := func() (*arc.Msg, *arc.AttrElemPb, error) {
		select {
		case msg := <-sess.msgsIn:
			attr, err := msg.GetMetaAttr()
			return msg, attr, err
		case <-timer.C:
			return nil, nil, arc.ErrTimeout
		case <-sess.Closing():
			return nil, nil, arc.ErrShuttingDown
		}
	}

	// Wait for login msg
	msg, attr, err := nextMsg()
	if err != nil {
		return err
	}
	sess.loginReqID = msg.ReqID
	if sess.loginReqID == 0 {
		return arc.ErrCode_LoginFailed.Error("missing Req ID")
	}
	if err = sess.login.Unmarshal(attr.ValBuf); err != nil {
		return arc.ErrCode_LoginFailed.Error("failed to unmarshal Login")
	}

	// Importantly, this boots the planet app to mount and start the home planet, which calls InitSessionRegistry()
	_, err = sess.GetAppInstance(planet.AppUID, true)
	if err != nil {
		return err
	}

	{
		chall := &arc.LoginChallenge{
			// TODO
		}
		if err = arc.SendClientMetaAttr(sess, msg.ReqID, chall); err != nil {
			return err
		}
	}

	if msg, attr, err = nextMsg(); err != nil {
		return err
	}
	var loginResp arc.LoginResponse
	if err = loginResp.Unmarshal(attr.ValBuf); err != nil {
		return arc.ErrCode_LoginFailed.Error("failed to unmarshal LoginResponse")
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
			if msg.Status == arc.ReqStatus_Closed {
				sess.closeReq(msg.ReqID, false, nil)
			} else {
				var err error

				metaAttr, _ := msg.GetMetaAttr()
				if metaAttr != nil {
					keepOpen, err = sess.handleMetaAttr(msg.ReqID, metaAttr)
				} else {
					// tx := &arc.MultiTx{}
					// err = tx.UnmarshalFrom(msg, sess.SessionRegistry, false)
					err = arc.ErrUnimplemented // TODO: handle cell update tx
				}
				if err != nil || !keepOpen {
					sess.closeReq(msg.ReqID, true, err)
				}
			}
			msg.Reclaim()

		case <-sess.Closing():
			sess.cancelAll()
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

func (sess *hostSess) handleMetaAttr(reqID uint64, attr *arc.AttrElemPb) (keepOpen bool, err error) {

	switch arc.ConstSymbol(attr.AttrID) {
	case arc.ConstSymbol_PinRequest:
		{
			var req *reqContext
			req, err = sess.getReq(reqID, insertReq)
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
	case arc.ConstSymbol_RegisterDefs:
		{
			var defs arc.RegisterDefs
			if err = defs.Unmarshal(attr.ValBuf); err != nil {
				return
			}
			err = sess.SessionRegistry.RegisterDefs(&defs)
			return
		}
	case arc.ConstSymbol_HandleURI:
		{
			var req arc.HandleURI
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
		err = arc.ErrCode_InvalidReq.Error("unsupported meta attr")
		return
	}

}

type pinVerb int32

const (
	insertReq pinVerb = iota
	removeReq
	getReq
)

func (sess *hostSess) cancelAll() {
	sess.openReqsMu.Lock()
	defer sess.openReqsMu.Unlock()

	for reqID := range sess.openReqs {
		sess.closeReq(reqID, true, arc.ErrShuttingDown)
		delete(sess.openReqs, reqID)
	}
}

// onReq performs the given pinVerb on given reqID and returns its reqContext
func (sess *hostSess) getReq(reqID uint64, verb pinVerb) (req *reqContext, err error) {

	sess.openReqsMu.Lock()
	{
		req = sess.openReqs[reqID]
		if req != nil {
			switch verb {
			case removeReq:
				delete(sess.openReqs, reqID)
			case insertReq:
				err = arc.ErrCode_InvalidReq.Error("ReqID already in use")
			}
		} else {
			switch verb {
			case insertReq:
				req = &reqContext{
					cancel:  make(chan struct{}),
					started: make(chan struct{}),
					sess:    sess,
				}
				req.ReqID = reqID
				req.Outlet = sess.msgsOut
				sess.openReqs[reqID] = req
			}
		}
	}
	sess.openReqsMu.Unlock()

	return
}

func (sess *hostSess) PinCell(pinReq arc.PinReq) (arc.PinContext, error) {
	req := &reqContext{
		cancel:  make(chan struct{}),
		started: make(chan struct{}),
		sess:    sess,
	}
	req.PinReqParams = *pinReq.Params()

	if err := sess.pinCell(req); err != nil {
		return nil, err
	}

	<-req.started
	return req, nil
}

func (sess *hostSess) pinCell(req *reqContext) error {
	var err error

	req.Target = arc.CellID(req.PinReq.PinCellID)

	pinReq := &req.PinReq

	// Parse and process the pin request (PinReq)
	if len(pinReq.PinURL) > 0 {
		req.URL, err = url.Parse(pinReq.PinURL)
		if err != nil {
			err = arc.ErrCode_InvalidReq.Errorf("failed to parse PinURI: %v", err)
			return err
		}
	}

	var parent arc.PinnedCell
	if pinReq.ParentReqID != 0 {
		parentReq, _ := sess.getReq(pinReq.ParentReqID, getReq)
		if parentReq == nil {
			err = arc.ErrCode_InvalidReq.Error("invalid ParentReqID")
			return err
		}
		req.appCtx = parentReq.appCtx
		parent = parentReq.pinned
	}

	// If no app context is available, choose an app based on the app invocation (appearing as the hostname in the URL)
	if req.appCtx == nil && req.URL != nil {
		req.appCtx, err = sess.appCtxForInvocation(req.URL.Host, true)
		if err != nil {
			return err
		}
	}

	if req.appCtx == nil {
		err = arc.ErrCode_InvalidReq.Error("unable to resolve cell pinner")
		return err
	}

	req.appCtx.appReqs <- appReq{
		reqContext: req,
		parent:     parent,
	}

	return nil
}

func (sess *hostSess) registry() arc.Registry {
	return sess.host.Opts.Registry
}

func (sess *hostSess) LoginInfo() arc.Login {
	return sess.login
}

// func (sess *hostSess) PushMetaAttr(val arc.ElemVal, reqID uint64) error {
// 	attrSpec, err := sess.SessionRegistry.ResolveAttrSpec(val.TypeName(), false)
// 	if err != nil {
// 		sess.Warnf("ResolveAttrSpec() error: %v", err)
// 		return err
// 	}

// 	attrElem := arc.AttrElem{
// 		AttrID: attrSpec.DefID,
// 		Val:    val,
// 	}

// 	if reqID == 0 {
// 		reqID = sess.loginReqID
// 	}
// 	msg, err := attrElem.MarshalToMsg(arc.CellID(reqID))
// 	if err != nil {
// 		return err
// 	}

// 	msg.ReqID = sess.loginReqID
// 	msg.Op = arc.MsgOp_MetaAttr
// 	req, err := sess.getReq(msg.ReqID, getReq)
// 	if req != nil && err == nil {
// 		if !req.PushUpdate(msg) {
// 			err = arc.ErrNotConnected
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

func (sess *hostSess) GetAppInstance(appID arc.UID, autoCreate bool) (arc.AppInstance, error) {
	if ctx, err := sess.appContextForUID(appID, autoCreate); err == nil {
		return ctx.appInst, nil
	} else {
		return nil, err
	}
}

func (sess *hostSess) appContextForUID(appID arc.UID, autoCreate bool) (*appContext, error) {
	sess.openAppsMu.Lock()
	defer sess.openAppsMu.Unlock()

	app := sess.openApps[appID]
	if app == nil && autoCreate {
		appModule, err := sess.registry().GetAppByUID(appID)
		if err != nil {
			return nil, err
		}

		app = &appContext{
			appInst: appModule.NewAppInstance(),
			sess:    sess,
			module:  appModule,
			appReqs: make(chan appReq, 1),
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
					case appReq := <-app.appReqs:
						if reqErr := app.handleAppReq(appReq); reqErr != nil {
							appReq.reqContext.closeReq(true, reqErr)
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

type appReq struct {
	*reqContext
	parent arc.PinnedCell
}

// appContext Implements arc.AppContext
type appContext struct {
	task.Context

	appInst arc.AppInstance
	sess    *hostSess
	module  *arc.AppModule
	appReqs chan appReq // incoming requests for the app
}

func (ctx *appContext) IssueCellID() arc.CellID {
	return arc.CellID(ctx.sess.nextID.Add(1) + 100)
}

func (ctx *appContext) Session() arc.HostSession {
	return ctx.sess
}

func (ctx *appContext) LocalDataPath() string {
	return path.Join(ctx.sess.host.Opts.StatePath, ctx.module.AppID)
}

func (ctx *appContext) PublishAsset(asset arc.MediaAsset, opts arc.PublishOpts) (URL string, err error) {
	return ctx.sess.AssetPublisher().PublishAsset(asset, opts)
}

func (ctx *appContext) PinAppCell(flags arc.PinFlags) (*reqContext, error) {
	// planetApp, err := ctx.sess.GetAppInstance(planet.AppUID, true)
	// if err != nil {
	// 	return nil, err
	// }

	req := &arc.PinReqParams{
		Outlet: make(chan *arc.Msg, 1),
	}
	req.PinReq.Flags = arc.PinFlags_UseNativeSymbols | flags
	req.PinReq.PinURL = fmt.Sprintf("arc://planet/~/%s/.AppAttrs", ctx.module.AppID)

	pinCtx, err := ctx.sess.PinCell(req)
	if err != nil {
		return nil, err
	}
	return pinCtx.(*reqContext), nil
}

func (ctx *appContext) GetAppCellAttr(attrSpec string, dst arc.ElemVal) error {
	match, err := ctx.sess.SessionRegistry.ResolveAttrSpec(attrSpec, true)
	if err != nil {
		return err
	}

	// When we pin through the runtime, there is no ReqID since this is host-side.
	// So we use CloseOnSync and/or pinCtx.Close() when we're done.
	pinCtx, err := ctx.PinAppCell(arc.PinFlags_CloseOnSync)
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
			return arc.ErrCellNotFound
		}
	}
}

func (ctx *appContext) PutAppCellAttr(attrSpec string, src arc.ElemVal) error {
	spec, err := ctx.sess.ResolveAttrSpec(attrSpec, true)
	if err != nil {
		return err
	}

	msg, err := arc.FormMetaAttrTx(spec, src)
	if err != nil {
		return err
	}

	pinCtx, err := ctx.PinAppCell(arc.PinFlags_NoSync)
	if err != nil {
		return err
	}

	//cellTx.CellID = pinCtx.pinned.Info().CellID

	// TODO: make this better
	pinCtx.pinned.MergeUpdate(msg)
	// err = pinCtx.PushUpdate(msg)
	// if err != nil {
	// 	return err
	// }

	pinCtx.Close()
	return nil
}

func (app *appContext) handleAppReq(appReq appReq) error {

	// TODO: resolve to a generic Cell and *then* pin it?
	// Also, use pinned cell ref counting to know when it it safe to idle close?
	var err error
	req := appReq.reqContext
	req.pinned, err = req.appCtx.appInst.PinCell(appReq.parent, req)
	if err != nil {
		return err
	}

	label := req.LogLabel
	if label == "" {
		label = req.GetLogLabel()
	}

	// FUTURE: switch to ref counting rather than task.Context close detection?
	// Once the cell is resolved, serve state in child context of the PinnedCell
	req.Context, err = req.pinned.Context().StartChild(&task.Task{
		TaskRef:   arc.PinContext(req),
		Label:     label,
		IdleClose: time.Microsecond,
		OnRun: func(pinCtx task.Context) {
			err := req.pinned.ServeState(req)
			if err != nil && err != arc.ErrShuttingDown {
				pinCtx.Errorf("ServeState() error: %v", err)
			}
			if (req.PinReq.Flags & arc.PinFlags_CloseOnSync) != 0 {
				pinCtx.Close()
			} else {
				<-pinCtx.Closing()
			}
		},
		OnClosing: func() {
			appReq.reqContext.closeReq(true, nil)
		},
	})
	// Check race condition where the req was closed before the pinned cell is resolved
	close(req.started)
	if err == nil && appReq.reqContext.closed.Load() != 0 {

	}

	return err
}

/*
// implements arc.PinContext in order to read a planet cell as a "one-shot" read
type fauxClient struct {
	task.Context // stays nil since this never spins up as a real PinContext
	arc.SessionRegistry
	invoker arc.AppContext
	match   arc.AttrSpec
	val     arc.ElemVal
	err     error
}

func (ctx *fauxClient) App() arc.AppContext {
	return ctx.invoker
}

func (ctx *fauxClient) MaintainSync() bool {
	return false
}

func (ctx *fauxClient) UsingNativeSymbols() bool {
	return true
}

func (ctx *fauxClient) PushUpdate(msg *arc.Msg) bool {
	var err error
	if msg.AttrID == ctx.match.DefID {
		err = ctx.val.Unmarshal(msg.ValBuf)
		ctx.err = err
		return false
	}
	msg.Reclaim()
	return true
}
*/
/*
func (ctx *appContext) GetSchemaForType(typ reflect.Type) (*arc.AttrSchema, error) {

	// TODO: skip if already registered
	{
	}

	schema, err := arc.MakeSchemaForType(typ)
	if err != nil {
		return nil, err
	}

	schema.SchemaID = ctx.nextSchemaID.Add(-1) // negative IDs reserved for host-side schemas
	defs := arc.Defs{
		Schemas: []*arc.AttrSchema{schema},
	}
	err = ctx.Session().RegisterDefs(&defs)
	if err != nil {
		return nil, err
	}

	return schema, nil
}

func (ctx *appContext) getValStoreSchema() (schema *arc.AttrSchema, err error) {
	if ctx.valStoreSchemaID == 0 {
		schema, err = ctx.GetSchemaForType(reflect.TypeOf(valStore{}))
		if err != nil {
			return
		}
		ctx.valStoreSchemaID = schema.SchemaID
	} else {
		schema, err = ctx.Session().GetSchemaByID(ctx.valStoreSchemaID)
	}
	return
}
*/
