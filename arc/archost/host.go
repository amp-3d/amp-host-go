package archost

import (
	"fmt"
	"net/url"
	"path"
	"strings"
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
	Opts
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

	err = host.AssetServer.StartService(host.Context)
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
		msgsOut:  make(chan *arc.Msg, 8),
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
						var err error
						if flags := msg.Flags; flags&arc.MsgFlags_ValBufShared != 0 {
							msg.Flags = flags &^ arc.MsgFlags_ValBufShared
							err = via.SendMsg(msg)
							msg.Flags = flags
						} else {
							err = via.SendMsg(msg)
						}
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
	loginReqID uint64
	openAppsMu sync.Mutex
	openApps   map[arc.UID]*appContext
}

type cellReq struct {
	reqID  uint64
	pinReq arc.PinReq // Raw request from client
	pinURL *url.URL   // https://pkg.go.dev/net/url#URL
}

func (req *cellReq) PinID() arc.CellID {
	return arc.CellID(req.pinReq.PinCell)
}

func (req *cellReq) URL() *url.URL {
	return req.pinURL
}

func (req *cellReq) URLPath() []string {
	if req.pinURL == nil || req.pinURL.Path == "" {
		return nil
	}
	return strings.Split(req.pinURL.Path, "/")
}

func (req *cellReq) String() string {
	var strBuf [128]byte

	str := fmt.Appendf(strBuf[:0], "Req:%04d  ", req.reqID)
	if pinID := req.PinID(); pinID != 0 {
		str = fmt.Appendf(str, "Cell:%04d  ", pinID)
	}
	if req.pinURL != nil {
		fmt.Append(str, "PinURL: ", req.pinURL.String())
	}
	return string(str)
}

// clientReq
// hostReq?
// *** implements PinContext ***
type reqContext struct {
	task.Context
	cellReq
	appCtx *appContext
	pinned arc.PinnedCell
	cancel chan struct{}
	closed atomic.Int32

	// idle        uint32           // set when the pinnedRange reaches the target range
	// targetRange arc.AttrRange // specifies the range(s) to be mapped
	//backlog    []*arc.Msg // backlog of update msgs if this sub falls behind.  TODO: remplace with chunked "infinite" queue class
}

func (req *reqContext) App() arc.AppContext {
	return req.appCtx.AppInstance
}

func (req *reqContext) MaintainSync() bool {
	return req.cellReq.pinReq.MaintainSync
}

func (req *reqContext) PushMsg(msg *arc.Msg) bool {
	msg.ReqID = req.cellReq.reqID

	// If the client backs up, this will back up too which is the desired effect.
	// Otherwise, something like reading from a db reading would quickly fill up the Msg outbox chan (and have no gain)
	// Note that we don't need to check on req.cell or req.sess since if either close, all subs will be closed.
	select {

	case req.appCtx.sess.msgsOut <- msg:
		return true

	default:
		var closing <-chan struct{}
		if req.Context != nil {
			closing = req.Context.Closing()
		} else {
			closing = req.cancel
		}
		select {
		case req.appCtx.sess.msgsOut <- msg:
			return true
		case <-closing:
			return false
		}
	}
}

func (req *reqContext) closeReq(pushClose bool, err error) {
	if req == nil {
		return
	}

	doClose := req.closed.CompareAndSwap(0, 1)
	if doClose {

		// first, remove this req as a sub if applicable
		// if cell := req.cell; cell != nil {
		// 	cell.pl.cancelSub(req)
		// }

		// next, send a close msg to the client
		if pushClose {
			msg := arc.NewMsg()
			msg.Op = arc.MsgOp_Close
			if err != nil {
				msg.AttrID = arc.ConstSymbol_Err.Ord()
				arc.ErrorToValue(err).MarshalToBuf(&msg.ValBuf)
			}
			req.PushMsg(msg)
		}

		// finally, close the cancel chan now that the close msg has been pushed
		close(req.cancel)

		// TODO: there is a race condition where the ctx could be set
		if req.Context != nil {
			req.Context.Close()
		}
	}
}

func (sess *hostSess) closeReq(reqID uint64, err error, pushClose bool) {
	req, _ := sess.getReq(reqID, removeReq)
	if req != nil {
		req.closeReq(pushClose, err)
	} else if pushClose {
		msg := arc.NewMsg()
		msg.ReqID = reqID
		msg.Op = arc.MsgOp_Close
		if err != nil {
			msg.AttrID = arc.ConstSymbol_Err.Ord()
			arc.ErrorToValue(err).MarshalToBuf(&msg.ValBuf)
		}
		sess.pushMsg(msg)
	}
}

func (sess *hostSess) pushMsg(msg *arc.Msg) {
	select {
	case sess.msgsOut <- msg:
	case <-sess.Closing():
	}
}

func (sess *hostSess) AssetPublisher() arc.AssetPublisher {
	return sess.host.Opts.AssetServer
}

func (sess *hostSess) handleLogin() error {
	if sess.login.UserUID != "" {
		return arc.ErrCode_LoginFailed.Error("already logged in")
	}

	timer := time.NewTimer(time.Duration(3 * time.Second))

	// Wait for login msg
	var msg *arc.Msg
	{
		select {
		case msg = <-sess.msgsIn:
			if err := msg.UnmarshalValue(&sess.login); err != nil {
				return arc.ErrCode_LoginFailed.Error("failed to unmarshal Login")
			}
			sess.loginReqID = msg.ReqID
		case <-timer.C:
			return arc.ErrTimeout
		case <-sess.Closing():
			return arc.ErrShuttingDown
		}
	}

	// Importantly, this boots the planet app to mount and start the home planet, which calls InitSessionRegistry()
	_, err := sess.GetAppContext(planet.AppUID, true)
	if err != nil {
		return err
	}

	// Send challenge (TODO)
	{
		reply := &arc.LoginChallenge{}
		msg.MarshalValue(reply)
		sess.pushMsg(msg)
		msg = nil
	}

	var loginResp arc.LoginResponse
	select {
	case msg = <-sess.msgsIn:
		if err := msg.UnmarshalValue(&loginResp); err != nil {
			return arc.ErrCode_LoginFailed.Error("failed to unmarshal LoginResponse")
		}
	case <-timer.C:
		return arc.ErrTimeout
	case <-sess.Closing():
		return arc.ErrShuttingDown
	}

	// Await challenge response (TODO)
	return nil
}

func (sess *hostSess) consumeInbox() {
	for running := true; running; {
		select {

		case msg := <-sess.msgsIn:
			if msg != nil && msg.Op != arc.MsgOp_NoOp {
				closeReq := true
				var err error

				switch msg.Op {
				case arc.MsgOp_PinCell:
					err = sess.pinCell(msg)
					closeReq = err != nil
				case arc.MsgOp_MetaAttr:
					err = sess.handleMetaAttr(msg)
				case arc.MsgOp_Close:
				default:
					err = arc.ErrCode_UnsupportedOp.Errorf("unknown MsgOp: %v", msg.Op)
				}

				if closeReq {
					sess.closeReq(msg.ReqID, err, true)
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

func (sess *hostSess) handleMetaAttr(msg *arc.Msg) error {
	elem, err := msg.UnmarshalAttrElem(sess.SessionRegistry)
	if err != nil {
		return err
	}

	// Some meta attrs are echoed to all open apps for handling -- others are intercepted
	switch v := elem.Val.(type) {
	case *arc.RegisterDefs:
		return sess.SessionRegistry.RegisterDefs(v)
	case *arc.HandleURI:
		// post to to apps
	default:
		return arc.ErrCode_InvalidReq.Error("unsupported meta attr")
	}

	// TODO: deadlock if appCtx tries to open a new app
	sess.openAppsMu.Lock()
	defer sess.openAppsMu.Unlock()

	for _, appCtx := range sess.openApps {
		appCtx.HandleMetaAttr(elem)
	}
	return nil
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

	for reqID, req := range sess.openReqs {
		req.closeReq(false, nil)
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
					cancel: make(chan struct{}),
					cellReq: cellReq{
						reqID: reqID,
					},
				}
				sess.openReqs[reqID] = req
			}
		}
	}
	sess.openReqsMu.Unlock()

	return
}

// TODO: expose this via arc.HostSession to allow apps to abstractly pin cells
func (sess *hostSess) pinCell(msg *arc.Msg) error {

	req, err := sess.getReq(msg.ReqID, insertReq)
	if err != nil {
		return err
	}

	pinReq := &req.cellReq.pinReq
	if err = msg.UnmarshalValue(pinReq); err != nil {
		return err
	}

	// Parse and process the pin request (PinReq)
	if len(pinReq.PinURI) > 0 {
		req.pinURL, err = url.Parse(pinReq.PinURI)
		if err != nil {
			err = arc.ErrCode_InvalidReq.Errorf("failed to parse PinURI: %v", err)
			return err
		}
	}

	var res arc.CellResolver
	if pinReq.ParentReqID != 0 {
		parentReq, _ := sess.getReq(pinReq.ParentReqID, getReq)
		if parentReq == nil {
			err = arc.ErrCode_InvalidReq.Error("invalid ParentReqID")
			return err
		}
		req.appCtx = parentReq.appCtx
		res = parentReq.pinned
	}

	// If no app context is available, choose an app based on the app invocation (appearing as the hostname in the URL)
	if res == nil && req.pinURL != nil {
		req.appCtx, err = sess.getAppForInvocation(req.pinURL.Host, true)
		if err != nil {
			return err
		}
		res = req.appCtx
	}

	if res == nil {
		err = arc.ErrCode_InvalidReq.Error("unable to resolve cell")
		return err
	}

	req.appCtx.newReqs <- appReq{
		reqContext: req,
		resolver:   res,
	}

	return nil
}

func (sess *hostSess) registry() arc.Registry {
	return sess.host.Opts.Registry
}

func (sess *hostSess) LoginInfo() arc.Login {
	return sess.login
}

func (sess *hostSess) PushMetaAttr(val arc.ElemVal) error {
	attrDef, err := sess.SessionRegistry.ResolveAttrSpec(val.AttrSpec())
	if err != nil {
		sess.Warnf("FormAttr() error: %v", err)
		return err
	}

	attrElem := arc.AttrElem{
		AttrID: attrDef.Client.DefID,
		Val:    val,
	}
	msg, err := attrElem.MarshalToMsg()
	if err != nil {
		return err
	}

	msg.ReqID = sess.loginReqID
	req, err := sess.getReq(msg.ReqID, getReq)
	if req != nil && err == nil {
		if !req.PushMsg(msg) {
			err = arc.ErrNotConnected
		}
	} else {
		sess.pushMsg(msg)
	}
	return err
}

func (sess *hostSess) onAppClosing(appCtx *appContext) {
	sess.openAppsMu.Lock()
	delete(sess.openApps, appCtx.module.UID)
	sess.openAppsMu.Unlock()
}

func (sess *hostSess) GetAppContext(appID arc.UID, autoCreate bool) (arc.AppContext, error) {
	if ctx, err := sess.getAppForUID(appID, autoCreate); err == nil {
		return ctx.AppInstance, nil
	} else {
		return nil, err
	}
}

func (sess *hostSess) getAppForUID(appID arc.UID, autoCreate bool) (*appContext, error) {
	sess.openAppsMu.Lock()
	defer sess.openAppsMu.Unlock()

	app := sess.openApps[appID]
	if app == nil && autoCreate {
		appModule, err := sess.registry().GetAppByUID(appID)
		if err != nil {
			return nil, err
		}

		app = &appContext{
			sess:            sess,
			SessionRegistry: sess.SessionRegistry,
			module:          appModule,
			newReqs:         make(chan appReq, 6),
		}
		app.AppInstance = appModule.NewAppInstance()
		if err = app.AppInstance.OnNew(app.AppInstance); err != nil {
			return nil, err
		}
		app.Context, err = sess.StartChild(&task.Task{
			Label:     appModule.AppID,
			IdleClose: 1 * time.Minute, // 10 mins?
			OnRun: func(ctx task.Context) {
				for running := true; running; {
					select {
					case appReq := <-app.newReqs:
						{
							// TODO: resolve to a generic Cell and *then* pin it
							req := appReq.reqContext
							pinned, err := appReq.resolver.ResolveCell(&req.cellReq)
							if err != nil {
								// TODO
								panic(err)
							}
							// FUTURE: switch to ref counting rather than task.Context close detection?
							// Once the cell is resolved, serve state in child context of the PinnedCell
							req.Context, err = pinned.Context().StartChild(&task.Task{
								TaskRef:   arc.PinContext(req),
								Label:     req.String(),
								IdleClose: time.Microsecond,
								OnRun: func(ctx task.Context) {
									err := pinned.PushState(req)
									if err != nil {
										ctx.Errorf("PushState() error: %v", err)
									}
									// PinContext shouldn't close until the req is closing
									if req.MaintainSync() {
										<-ctx.Closing()
									}
								},
								OnClosing: func() {
									appReq.reqContext.closeReq(true, nil)
								},
							})
							// Check race condition where the req was closed before the pinned cell is resolved
							if err == nil && appReq.reqContext.closed.Load() != 0 {

							}
							if err != nil {
								// TODO
								panic(err)
							}
						}
					case <-ctx.Closing():
						running = false
					}
				}
			},
			OnClosing: func() {
				sess.onAppClosing(app)
				app.OnClosing()
			},
		})
		if err != nil {
			return nil, err
		}

		sess.openApps[appModule.UID] = app
	}

	return app, nil
}

func (sess *hostSess) getAppForInvocation(invocation string, autoCreate bool) (*appContext, error) {
	appModule, err := sess.registry().GetAppForInvocation(invocation)
	if err != nil {
		return nil, err
	}

	ctx, err := sess.getAppForUID(appModule.UID, autoCreate)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

func (sess *hostSess) GetAppForInvocation(invocation string, autoCreate bool) (arc.AppContext, error) {
	if appCtx, err := sess.getAppForInvocation(invocation, autoCreate); err == nil {
		return appCtx.AppInstance, nil
	} else {
		return nil, err
	}

}

type appReq struct {
	*reqContext
	resolver arc.CellResolver
}

type appContext struct {
	task.Context
	arc.AppInstance
	arc.SessionRegistry

	sess    *hostSess
	module  *arc.AppModule
	newReqs chan appReq // incoming requests for the app
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

func (ctx *appContext) GetAppCellAttr(attrSpec string, dst arc.ElemVal) error {
	reader := fauxClient{
		SessionRegistry: ctx.sess.SessionRegistry,
		invoker:         ctx.AppInstance,
		val:             dst,
	}

	attrDef, err := ctx.sess.SessionRegistry.ResolveAttrSpec(attrSpec)
	if err != nil {
		return err
	}
	reader.match = attrDef.Client

	planetApp, err := ctx.sess.GetAppContext(planet.AppUID, true)
	if err != nil {
		return err
	}

	req := cellReq{
		pinReq: arc.PinReq{
			MaintainSync: false,
		},
	}
	cell, err := planetApp.ResolveCell(&req)
	if err != nil {
		return err
	}

	reader.err = arc.ErrCellNotFound
	err = cell.PushState(&reader)
	if err != nil {
		return err
	}

	return reader.err
}

func (ctx *appContext) PutAppCellAttr(attrSpec string, src arc.ElemVal) error {
	return arc.ErrCellNotFound
}

// implements arc.PinContext in order to read a planet cell as a "one-shot" read
type fauxClient struct {
	task.Context // stays nil since this never spins up as a real PinContext
	invoker      arc.AppContext
	arc.SessionRegistry
	match arc.AttrSpec
	val   arc.ElemVal
	err   error
}

func (ctx *fauxClient) App() arc.AppContext {
	return ctx.invoker
}

func (ctx *fauxClient) MaintainSync() bool {
	return false
}

func (ctx *fauxClient) PushMsg(msg *arc.Msg) bool {
	var err error
	if msg.AttrID == ctx.match.DefID {
		err = ctx.val.Unmarshal(msg.ValBuf)
		ctx.err = err
		return false
	}
	msg.Reclaim()
	return true
}

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
