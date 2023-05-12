package host

import (
	"fmt"
	"os"
	"path"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arcspace/go-arc-sdk/stdlib/bufs"
	"github.com/arcspace/go-arc-sdk/stdlib/process"
	"github.com/arcspace/go-arc-sdk/stdlib/utils"
	"github.com/arcspace/go-arcspace/arc"
	"github.com/arcspace/go-arcspace/arc/assets"
	"github.com/arcspace/go-arcspace/symbol"
	"github.com/dgraph-io/badger/v4"
)

type host struct {
	process.Context
	opts HostOpts

	homePlanetID uint64
	home         *planetSess // Home planet of this host
	appsByURI    map[string]arc.App
	appsByModel  map[string]arc.App
	plSess       map[uint64]*planetSess
	plMu         sync.RWMutex
}

const (
	hackHostPlanetID = 66
)

func startNewHost(opts HostOpts) (arc.Host, error) {
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
		opts:        opts,
		appsByURI:   make(map[string]arc.App),
		appsByModel: make(map[string]arc.App),
		plSess:      make(map[uint64]*planetSess),
	}

	// err = host.loadSeat()
	// if err != nil {
	// 	host.Process.OnClosed()
	// 	return nil, err
	// }

	// // This is a hack for now
	// if len(host.seat.RootPlanets) == 0 {
	// 	host.mountPlanet()
	// }

	host.Context, err = process.Start(&process.Task{
		Label:     host.opts.Label,
		IdleClose: time.Nanosecond,
		OnClosed: func() {
			host.Info(1, "arc.Host shutdown complete")
		},
	})
	if err != nil {
		return nil, err
	}

	err = host.opts.AssetServer.StartService(host.Context)
	if err != nil {
		host.Close()
		return nil, err
	}

	err = host.mountHomePlanet()
	if err != nil {
		host.Close()
		return nil, err
	}

	return host, nil
}

func (host *host) mountHomePlanet() error {
	var err error

	if host.homePlanetID == 0 {
		host.homePlanetID = hackHostPlanetID

		_, err = host.getPlanet(host.homePlanetID)

		//pl, err = host.mountPlanet(0, &arc.PlanetEpoch{
		// 	EpochTID:   utils.RandomBytes(16),
		// 	CommonName: "HomePlanet",
		// })
	}

	return err
	// // Add a new home/root planent if none exists
	// if host.seat.HomePlanetID == 0 {
	// 	pl, err = host.mountPlanet(0, &arc.PlanetEpoch{
	// 		EpochTID:   utils.RandomBytes(16),
	// 		CommonName: "HomePlanet",
	// 	})
	// 	if err == nil {
	// 		host.seat.HomePlanetID = pl.planetID
	// 		host.commitSeatChanges()
	// 	}
	// } else {

	// }

}

/*
func (host *host) loadSeat() error {
	err := host.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(gSeatKey)
		if err == nil {
			err = item.Value(func(val []byte) error {
				host.seat = arc.HostSeat{}
				return host.seat.Unmarshal(val)
			})
		}
		return err
	})

	switch err {

	case badger.ErrKeyNotFound:
		host.seat = arc.HostSeat{
			MajorVers: 2022,
			MinorVers: 1,
		}
		err = host.commitSeatChanges()

	case nil:
		if host.seat.MajorVers != 2022 {
			err = errors.New("Catalog version is incompatible")
		}

	}

	return err
}

func (host *host) commitSeatChanges() error {
	err := host.db.Update(func(txn *badger.Txn) error {
		stateBuf, err := host.seat.Marshal()
		if err != nil {
			return err
		}
		err = txn.Set(gSeatKey, stateBuf)
		if err != nil {
			return err
		}
		return err
	})
	return err
}
*/

func (host *host) getPlanet(planetID uint64) (*planetSess, error) {
	if planetID == 0 {
		return nil, arc.ErrCode_PlanetFailure.Error("no planet ID given")
	}

	host.plMu.RLock()
	pl := host.plSess[planetID]
	host.plMu.RUnlock()

	if pl != nil {
		return pl, nil
	}

	return host.mountPlanet(planetID, nil)
}

// mountPlanet mounts the given planet by ID, or creates a new one if genesis is non-nil.
func (host *host) mountPlanet(
	planetID uint64,
	genesis *arc.PlanetEpoch,
) (*planetSess, error) {

	if planetID == 0 {
		if genesis == nil {
			return nil, arc.ErrCode_PlanetFailure.Error("missing PlanetID and PlanetEpoch TID")
		}
		planetID = host.home.GetSymbolID(genesis.EpochTID, false)
	}

	host.plMu.Lock()
	defer host.plMu.Unlock()

	// Check if already mounted
	pl := host.plSess[planetID]
	if pl != nil {
		return pl, nil
	}

	var fsName string
	if planetID == host.homePlanetID {
		fsName = "HostHomePlanet"
	} else if planetID != 0 {
		fsName = string(host.home.LookupID(planetID))
		if len(fsName) == 0 && genesis == nil {
			return nil, arc.ErrCode_PlanetFailure.Errorf("planet ID=%v failed to resolve", planetID)
		}
	} else {

		asciiTID := bufs.Base32Encoding.EncodeToString(genesis.EpochTID)
		fsName = utils.MakeFSFriendly(genesis.CommonName, nil) + " " + asciiTID[:6]
		planetID = host.home.GetSymbolID([]byte(fsName), true)

		// Create new planet ID entries that all map to the same ID value
		host.home.SetSymbolID([]byte(asciiTID), planetID)
		host.home.SetSymbolID(genesis.EpochTID, planetID)
	}

	pl = &planetSess{
		planetID: planetID,
		dbPath:   path.Join(host.opts.StatePath, string(fsName)),
		cells:    make(map[arc.CellID]*cellInst),
		//newReqs:  make(chan *openReq, 1),
	}

	// The db should already exist if opening and vice versa
	_, err := os.Stat(pl.dbPath)
	if genesis != nil && err == nil {
		return nil, arc.ErrCode_PlanetFailure.Error("planet db already exists")
	}

	task := &process.Task{
		Label: fsName,
		OnStart: func(process.Context) error {

			// The host's home planet is ID issuer of all other planets
			opts := symbol.DefaultTableOpts
			if host.home.symTable != nil {
				opts.Issuer = host.home.symTable.Issuer()
			}
			return pl.onStart(opts)
		},
		OnRun:    pl.onRun,
		OnClosed: pl.onClosed,
	}

	// Make sure host.home closes last, so make all mounted planets subs
	if host.home == nil {
		host.home = pl
		pl.Context, err = host.StartChild(task)
	} else {
		task.IdleClose = 120 * time.Second
		pl.Context, err = host.home.StartChild(task)
	}
	if err != nil {
		return nil, err
	}

	// if genesis != nil {
	// 	pl.replayGenesis(seed)
	// }
	//

	host.plSess[planetID] = pl
	return pl, nil
}

func (host *host) RegisterApp(app arc.App) error {
	appURI := app.AppURI()
	if appURI == "" {
		return arc.ErrCode_InvalidURI.Error("invalid app URI")
	}
	host.appsByURI[appURI] = app

	for _, modelURI := range app.SupportedDataModels() {
		if modelURI != "" {
			host.appsByModel[modelURI] = app
		}
	}
	return nil
}

func (host *host) SelectAppForSchema(schema *arc.AttrSchema) (arc.App, error) {
	if schema == nil {
		return nil, arc.ErrCode_AppNotFound.Errorf("missing schema")
	}

	if schema.AppURI != arc.DefaultAppForDataModel {
		app := host.appsByURI[schema.AppURI]
		if app != nil {
			return app, nil
		}
	}

	app := host.appsByModel[schema.CellDataModel]
	if app == nil {
		return nil, arc.ErrCode_AppNotFound.Errorf("App not found for schema: %s", schema.SchemaDesc())
	}

	return app, nil
}

func (host *host) HostPlanet() arc.Planet {
	return host.home
}

func (host *host) StartNewSession(from arc.HostService, via arc.ServerStream) (arc.HostSession, error) {
	sess := &hostSess{
		host:         host,
		TypeRegistry: arc.NewTypeRegistry(host.home.symTable),
		msgsIn:       make(chan *arc.Msg),
		msgsOut:      make(chan *arc.Msg, 8),
		openReqs:     make(map[uint64]*openReq),
	}

	var err error
	sess.Context, err = host.home.StartChild(&process.Task{
		Label:     "HostSession",
		IdleClose: time.Nanosecond,
		OnRun: func(ctx process.Context) {
			sess.consumeInbox()
		},
	})
	if err != nil {
		return nil, err
	}

	hostSessDesc := fmt.Sprintf("%s(%d)", sess.Label(), sess.ContextID())

	// Start a child contexts for send & recv that drives hostSess the inbox & outbox.
	// We start them as children of the HostService, not the HostSession since we want to keep the stream running until hostSess completes closing.
	//
	// Possible paths:
	//   - If stream returns ServerStreamClosed (or errors out), initiate hostSess.Close()
	//   - If hostSess.Close() is called externally, when close is complete, <-sessDone (below) will close the steam.
	from.StartChild(&process.Task{
		Label:     fmt.Sprint(via.Desc(), " <- ", hostSessDesc),
		IdleClose: time.Nanosecond,
		OnRun: func(ctx process.Context) {
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
						msg = nil
						if err != nil /*&& err != ServerStreamClosed */ {
							ctx.Warnf("ServerStream Send() err: %v", err)
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

	from.StartChild(&process.Task{
		Label:     fmt.Sprint(via.Desc(), " -> ", hostSessDesc),
		IdleClose: time.Nanosecond,
		OnRun: func(ctx process.Context) {
			sessDone := sess.Done()

			for running := true; running; {
				msg, err := via.RecvMsg()
				if err != nil {
					if err == arc.ErrStreamClosed {
						ctx.Info(2, "ServerStream closed")
					} else {
						ctx.Warnf("RecvMsg() error: %v", err)
					}
					sess.Context.Close()
					running = false
				} else if msg != nil {
					select {
					case sess.msgsIn <- msg:
					case <-sessDone:
						ctx.Info(2, "hostSession done")
						running = false
					}
				}
			}
		},
	})

	return sess, nil
}

// hostSess wraps a host session the parent host has with a client.
type hostSess struct {
	process.Context
	arc.TypeRegistry

	nextID     atomic.Uint64       // next CellID to be issued
	user       arc.User            // current user
	host       *host               // parent host
	msgsIn     chan *arc.Msg       // msgs inbound to this hostSess
	msgsOut    chan *arc.Msg       // msgs outbound from this hostSess
	openReqs   map[uint64]*openReq // ReqID maps to an open request.
	openReqsMu sync.Mutex          // protects openReqs
}

// planetSess represents a "mounted" planet (a Cell database), allowing it to be accessed, served, and updated.
type planetSess struct {
	process.Context

	symTable symbol.Table             // each planet has separate symbol tables
	planetID uint64                   // symbol ID (as known by the host's symbol table)
	dbPath   string                   // local pathname to db
	db       *badger.DB               // db access
	cells    map[arc.CellID]*cellInst // cells that recently have one or more active cells (subscriptions)
	cellsMu  sync.Mutex               // cells mutex
}

type openReq struct {
	arc.CellReq

	sess   *hostSess
	cell   *cellInst
	cancel chan struct{}
	closed uint32
	next   *openReq // single linked list of same-cell reqs

	//echo   arc.CellSub
	// err    error
	// attr        *arc.AttrSpec // if set, describes this attr (read-only).  if nil, all SeriesType_0 values are to be loaded.
	// idle        uint32           // set when the pinnedRange reaches the target range
	// pinnedRange Range            // the range currently mapped
	// targetRange arc.AttrRange // specifies the range(s) to be mapped
	//backlog    []*arc.Msg // backlog of update msgs if this sub falls behind.  TODO: remplace with chunked "infinite" queue class
}

func (req *openReq) Req() *arc.CellReq {
	return &req.CellReq
}

func (req *openReq) PushUpdate(batch *arc.MsgBatch) error {
	if atomic.LoadUint32(&req.closed) != 0 {
		return nil
	}

	// TODO / FUTURE
	// Instead of every req running its own goroutine, just have one that round robbins
	// based on a 'wakeup' channel saying which req sub is actively pushing msgs.
	//
	// This also make app sb life easier since msgs are just pushed as they're made rather than building batches
	// and then sending them all to this for one big PushUpdate.
	for _, src := range batch.Msgs {
		msg := arc.CopyMsg(src)
		err := req.PushMsg(msg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (req *openReq) PushMsg(msg *arc.Msg) error {
	var err error

	{
		msg.ReqID = req.ReqID

		// If the client backs up, this will back up too which is the desired effect.
		// Otherwise, something like reading from a db reading would quickly fill up the Msg outbox chan (and have no gain)
		// Note that we don't need to check on req.cell or req.sess since if either close, all subs will be closed.
		select {
		case req.sess.msgsOut <- msg:
		case <-req.cancel:
			err = arc.ErrCode_ShuttingDown.Error("request closing")
		}
	}

	return err
}

func (req *openReq) closeReq(pushClose bool, msgVal interface{}) {
	if req == nil {
		return
	}

	doClose := atomic.CompareAndSwapUint32(&req.closed, 0, 1)
	if doClose {

		// first, remove this req as a sub if applicable
		if cell := req.cell; cell != nil {
			cell.pl.cancelSub(req)
		}

		// next, send a close msg to the client
		if pushClose {
			msg := arc.NewMsg()
			msg.Op = arc.MsgOp_CloseReq
			if msgVal != nil {
				msg.SetVal(msgVal)
			}
			req.PushMsg(msg)
		}

		// finally, close the cancel chan now that the close msg has been pushed
		close(req.cancel)
	}
}

func (sess *hostSess) closeReq(reqID uint64, pushClose bool, msgVal interface{}) {
	req, _ := sess.getReq(reqID, removeReq)
	if req != nil {
		req.closeReq(pushClose, msgVal)
	} else if pushClose {
		sess.pushMsg(reqID, arc.MsgOp_CloseReq, msgVal)
	}
}

func (sess *hostSess) pushMsg(reqID uint64, msgOp arc.MsgOp, msgVal interface{}) {
	msg := arc.NewMsg()
	msg.ReqID = reqID
	msg.Op = msgOp

	if msgVal != nil {
		msg.SetVal(msgVal)
	}
	select {
	case sess.msgsOut <- msg:
	case <-sess.Closing():
	}
}

func (sess *hostSess) LoggedIn() arc.User {
	return sess.user
}

func (sess *hostSess) AssetServer() assets.AssetServer {
	return sess.host.opts.AssetServer
}

func (sess *hostSess) IssueCellID() arc.CellID {
	return arc.CellID(sess.nextID.Add(1) + 2701)
}

func (sess *hostSess) consumeInbox() {
	for running := true; running; {
		select {

		case msg := <-sess.msgsIn:
			if msg != nil && msg.Op != arc.MsgOp_NoOp {
				closeReq := true

				var err error
				switch msg.Op {
				// case arc.MsgOp_PinAttrRange:
				// 	err = sess.pinAttrRange(msg))
				// case arc.MsgOp_CreateCell:
				// 	err = sess.createCell(msg)
				// 	closeReq = err != nil
				case arc.MsgOp_PinCell:
					err = sess.pinCell(msg)
					closeReq = err != nil
				case arc.MsgOp_ResolveAndRegister:
					err = sess.resolveAndRegister(msg)
				case arc.MsgOp_Login:
					err = sess.login(msg)
				case arc.MsgOp_CloseReq:
				default:
					err = arc.ErrCode_UnsupportedOp.Errorf("unknown MsgOp: %v", msg.Op)
				}

				if closeReq {
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

func (sess *hostSess) login(msg *arc.Msg) error {
	if sess.user != nil {
		return arc.ErrCode_InvalidLogin.Error("already logged in")
	}

	u := &user{
		sess: sess,
	}
	if err := msg.LoadVal(&u.LoginReq); err != nil {
		return err
	}

	//
	// FUTURE: a "user home" app would start here and is bound to the userUID on the host's home arc.
	//
	seat, err := sess.host.home.getUser(u.LoginReq, true)
	if err != nil {
		return err
	}

	u.home, err = sess.host.getPlanet(seat.HomePlanetID)
	if err != nil {
		return err
	}

	sess.user = u
	return nil
}

func (sess *hostSess) resolveAndRegister(msg *arc.Msg) error {
	var defs arc.Defs
	if err := msg.LoadVal(&defs); err != nil {
		return err
	}

	if err := sess.TypeRegistry.ResolveAndRegister(&defs); err != nil {
		return err
	}

	sess.pushMsg(msg.ReqID, arc.MsgOp_CloseReq, nil)
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

// onReq performs the given pinVerb on given reqID and returns its openReq
func (sess *hostSess) getReq(reqID uint64, verb pinVerb) (req *openReq, err error) {

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
				req = &openReq{
					sess:   sess,
					cancel: make(chan struct{}),
				}
				req.ReqID = reqID
				req.CellSub = req
				sess.openReqs[reqID] = req
			}
		}
	}
	sess.openReqsMu.Unlock()

	return
}

func (sess *hostSess) pinCell(msg *arc.Msg) error {

	req, err := sess.getReq(msg.ReqID, insertReq)
	if err != nil {
		return err
	}

	var pinReq arc.PinReq
	if err = msg.LoadVal(&pinReq); err != nil {
		return err
	}

	if pinReq.ParentReqID != 0 {
		parentReq, _ := sess.getReq(pinReq.ParentReqID, getReq)
		if parentReq == nil {
			err = arc.ErrCode_InvalidReq.Error("invalid ParentReqID")
			return err
		}
		req.ParentReq = &parentReq.CellReq
	}

	// Recover the referenced cell model the client wants to pin
	req.ContentSchema, err = sess.TypeRegistry.GetSchemaByID(pinReq.ContentSchemaID)
	if err != nil {
		return err
	}

	// Select the app that can handle the requested schema / model.
	req.ParentApp, err = sess.host.SelectAppForSchema(req.ContentSchema)
	if err != nil {
		return err
	}

	req.CellID = arc.CellID(pinReq.PinCell)
	req.User = sess.user
	req.Args = pinReq.Args
	req.ChildSchemas = make([]*arc.AttrSchema, len(pinReq.ChildSchemas))
	for i, child := range pinReq.ChildSchemas {
		req.ChildSchemas[i], err = sess.TypeRegistry.GetSchemaByID(child)
		if err != nil {
			return err
		}
	}

	err = req.ParentApp.PinCell(&req.CellReq)
	if err != nil {
		return err
	}
	if req.Cell == nil {
		return arc.ErrCode_Unimplemented.Errorf("app %s failed to pin cell", req.ParentApp.AppURI())
	}

	pl, err := sess.host.getPlanet(sess.user.HomePlanet().PlanetID())
	if err != nil {
		return err
	}

	err = pl.queueReq(nil, req)
	if err != nil {
		return err
	}

	return nil
}

type user struct {
	arc.LoginReq
	home         arc.Planet
	sess         *hostSess
	nextSchemaID atomic.Int32
}

func (usr *user) Session() arc.HostSession {
	return usr.sess
}

func (usr *user) LoginInfo() arc.LoginReq {
	return usr.LoginReq
}

func (usr *user) HomePlanet() arc.Planet {
	return usr.home
}

func (usr *user) MakeSchemaForStruct(app arc.App, structPtr any) (*arc.AttrSchema, error) {

	// Build and AttrSchema for the struct (use Elem() to deref the pointer)
	val := reflect.Indirect(reflect.ValueOf(structPtr))
	switch val.Kind() {
	case reflect.Pointer:
		val = val.Elem()
	case reflect.Struct:
	default:
		return nil, arc.ErrCode_ExportErr.Errorf("expected pointer to struct or struct, got %v", val.Kind())
	}
	valTyp := val.Type()

	numFields := val.NumField()

	schema := &arc.AttrSchema{
		AppURI:        app.AppURI(),
		CellDataModel: valTyp.Name(),
		SchemaName:    "on-demand-reflect",
		Attrs:         make([]*arc.AttrSpec, numFields),
		SchemaID:      usr.nextSchemaID.Add(-1), // negative IDs reserved for host-side schemas
	}

	for i := range schema.Attrs {
		field := val.Field(i)
		attr := &arc.AttrSpec{
			AttrURI: valTyp.Field(i).Name,
			AttrID:  int32(i + 1),
		}
		schema.Attrs[i] = attr

		switch field.Type().Kind() {
		case reflect.Int32:
		case reflect.Uint32:
		case reflect.Int64:
		case reflect.Uint64:
			attr.ValTypeID = arc.ValType_int
		case reflect.String:
			attr.ValTypeID = arc.ValType_string
		default:
			return nil, arc.ErrCode_ExportErr.Errorf("unsupported type '%s.%s: %v", schema.CellDataModel, attr.AttrURI, field.Type().Kind())
		}
	}

	defs := arc.Defs{
		Schemas: []*arc.AttrSchema{schema},
	}
	err := usr.Session().ResolveAndRegister(&defs)
	if err != nil {
		return nil, err
	}

	return schema, nil
}

func (usr *user) WriteCell(app arc.App, cellURI string, src any) error {

	schema, err := usr.MakeSchemaForStruct(app, src)
	if err != nil {
		return err
	}

	{
		tx := arc.NewMsgBatch()
		msg := tx.AddMsg()
		msg.Op = arc.MsgOp_UpsertCell
		msg.ValType = arc.ValType_SchemaID
		msg.ValInt = int64(schema.SchemaID)
		msg.ValBuf = append(append(msg.ValBuf[:0], []byte(app.AppURI())...), []byte(cellURI)...)

		val := reflect.ValueOf(src)
		numFields := val.NumField()

		valTyp := val.Type()

		for _, attr := range schema.Attrs {
			msg := tx.AddMsg()
			msg.Op = arc.MsgOp_PushAttr
			msg.AttrID = attr.AttrID
			for i := 0; i < numFields; i++ {
				if valTyp.Field(i).Name == attr.AttrURI {
					msg.SetVal(val.Field(i).Interface())
					break
				}
			}
			if msg.ValType == arc.ValType_nil {
				panic("missing field")
			}
		}

		msg = tx.AddMsg()
		msg.Op = arc.MsgOp_Commit

		if err := usr.HomePlanet().PushTx(tx); err != nil {
			return err
		}
	}

	return nil
}

func (usr *user) ReadCell(app arc.App, cellURI string, dst any) error {

	schema, err := usr.MakeSchemaForStruct(app, dst)
	if err != nil {
		return err
	}

	val := reflect.ValueOf(dst)

	var keyBuf [256]byte
	cellKey := append(append(keyBuf[:0], []byte(app.AppURI())...), []byte(cellURI)...)

	err = usr.HomePlanet().ReadCell(cellKey, schema, func(msg *arc.Msg) {
		switch msg.Op {
		case arc.MsgOp_PushAttr:
			for ai, attr := range schema.Attrs {
				if attr.AttrID == msg.AttrID {
					msg.LoadVal(val.Field(ai).Interface())
				}
			}
		}
	})

	return err
	// {
	// 	tx := arc.NewMsgBatch()

	// 	val := reflect.ValueOf(dst)

	// 	for i := 0; i < numFields; i++ {
	// 		msg := tx.AddMsg()
	// 		msg.Op = arc.MsgOp_PushAttr
	// 		switch
	// 		msg.SetVal(

	//       fieldValue := field.Interface()
	//       fmt.Println(fieldName, " -> ", fieldValue)
	// }

}

/*


func (sess *hostSess) serveState(req *openReq) error {

	pl, err := sess.host.getPlanet(req.PlanetID)
	if err != nil {
		return err
	}

	csess, err := pl.getCellSess(req.target.NodeID, true)
	if err != nil {
		return err
	}

	// The client specifies how to map the node's attrs.
	// This is inherently safe since attrIDs won't match up otherwise, etc.
	mapAs := req.target.NodeTypeID
	spec, err := sess.TypeRegistry.GetResolvedNodeSpec(mapAs)
	if err != nil {
		return err
	}

	// Go through all the attr for this NodeType and for any series types, queue them for loading.
	var head, prev *cellSub
	for _, attr := range spec.Attrs {
		if attr.SeriesType != arc.SeriesType_0 && attr.AutoPin != arc.AutoPin_0 {
			sub := &cellSub{
				sess: sess,
				attr: attr,
			}
			if head == nil {
				head = sub
			} else {
				prev.next = sub
			}

			switch attr.AutoPin {
			case arc.AutoPin_All_Ascending:
				sub.targetRange.SI_SeekTo = 0
				sub.targetRange.SI_StopAt = uint64(arc.SI_DistantFuture)

			case arc.AutoPin_All_Descending:
				sub.targetRange.SI_SeekTo = uint64(arc.SI_DistantFuture)
				sub.targetRange.SI_StopAt = 0
			}

			prev = sub
		}
	}

	nSess.serveState(req)

	return nil
}

*/
/*

func (sess *hostSess) pinAttrRange(msg *arc.Msg) error {
	attrRange := arc.AttrRange{}

	if err := msg.LoadValue(&attrRange); err != nil {
		return err
	}

	// pin, err := sess.onCellReq(msg.ReqID, getPin)
	// if err != nil {
	// 	return err
	// }

	// err = pin.pinAttrRange(msg.AttrID, attrRange)
	// if err != nil {
	// 	return err
	// }

	return nil
}


	// Get (or make) a new req job using the given ReqID
	sess.pinsMu.Lock()
	pin := sess.pins[msg.ReqID]
	if pin != nil {
		if cancel {
			pin.Close()
			sess.pins[msg.ReqID] = nil
		} else {
			err = arc.ErrCode_InvalidReq.ErrWithMsg("ReqID already in use")
		}
	} else {
		if cancel {
			err = arc.ErrCode_ReqNotFound.Err()
		} else {
			pin = &nodeReq{
				reqCancel: make(chan struct{}),
			}
			sess.pins[msg.ReqID] = pin
		}
	}
	sess.pinsMu.Unlock()

*/

// func (req *nodeReq) Close() {

// }

// func (req *nodeReq) Closing() <-chan struct{} {
// 	return pin.reqCancel
// }

// func (req *nodeReq) Done() <-chan struct{} {
// 	return pin.reqCancel
// }

/*



OLD archost era stuff...


func (host *host) GetPlanet(planetID arc.TID, fromID arc.TID) (arc.Planet, error) {

	if len(domainName) == 0 {
		return nil, arc.arc.ErrCode_InvalidURI.ErrWithMsg("no planet domain name given")
	}

	host.mu.RLock()
	domain := host.plSess[domainName]
	host.my.RUnlock()

	if domain != nil {
		return domain, nil
	}

	if autoMount == false {
		return nil, arc.ErrCode_DomainNotFound.ErrWithMsg(domainName)
	}

	return host.mountDomain(domainName)

	return getPlanet(seed)
}

// Start -- see interface Host
func (host *host) Start() error {
	err := host.Process.Start()
	if err != nil {
		return err
	}

	dbPathname := path.Join(host.params.BasePath, "host.db")
	host.Infof(1, "opening db %v", dbPathname)
	opts := badger.DefaultOptions(dbPathname)
	opts.Logger = nil
	host.db, err = badger.Open(opts)
	if err != nil {
		return err
	}

	// host.vaultMgr = newVaultMgr(host)
	// err = host.vaultMgr.Start()
	// if err != nil {
	// 	return err
	// }

	// // Making the vault ctx a child ctx of this domain means that it must Stop before the domain ctx will even start stopping
	// host.CtxAddChild(host.vaultMgr, nil)

	return err
}

func (host *host) OnClosed() {

	// Since domain are child contexts of this host, by the time we're here, they have all finished stopping.
	// All that's left is to close the dbs
	if host.db != nil {
		host.db.Close()
		host.db = nil
	}
}

// OpenChSub -- see interface Host
func (host *host) OpenChSub(req *reqJob) (*chSub, error) {
	domain, err := host.getDomain(req.chURI.DomainName, true)
	if err != nil {
		return nil, err
	}
	return domain.OpenChSub(req)
}


// SubmitTx -- see interface Host
func (host *host) SubmitTx(tx *Tx) error {

	if tx == nil || tx.TxOp == nil {
		return arc.ErrCode_NothingToCommit.ErrWithMsg("missing tx")
	}

	uri := tx.TxOp.ChStateURI

	if uri == nil || len(uri.DomainName) == 0 {
		return arc.ErrCode_InvalidURI.ErrWithMsg("no domain name given")
	}

	var err error
	{
		// Use the same time value each node we're commiting
        timestampFS := TimeNowFS()
		for _, entry := range tx.TxOp.Entries {
			entry.Keypath, err = NormalizeKeypath(entry.Keypath)
			if err != nil {
				return err
			}

			switch entry.Op {
			case NodeOp_NodeUpdate:
			case NodeOp_NodeRemove:
			case NodeOp_NodeRemoveAll:
			default:
				err = arc.ErrCode_CommitFailed.ErrWithMsg("unsupported NodeOp for entry")
			}

            if (entry.RevID == 0) {
                entry.RevID = int64(timestampFS)
            }
		}
	}

	domain, err := host.getDomain(uri.DomainName, true)
	if err != nil {
		return err
	}

	err = domain.SubmitTx(tx)
	if err != nil {
		return err
	}

	return nil
}

func (host *host) getDomain(domainName string, autoMount bool) (*domain, error) {
	if len(domainName) == 0 {
		return nil, arc.arc.ErrCode_InvalidURI.ErrWithMsg("no planet domain name given")
	}

	host.domainsMu.RLock()
	domain := host.domains[domainName]
	host.domainsMu.RUnlock()

	if domain != nil {
		return domain, nil
	}

	if autoMount == false {
		return nil, arc.ErrCode_DomainNotFound.ErrWithMsg(domainName)
	}

	return host.mountDomain(domainName)
}

func (host *host) mountDomain(domainName string) (*domain, error) {
	host.domainsMu.Lock()
	defer host.domainsMu.Unlock()

	domain := host.domains[domainName]
	if domain != nil {
		return domain, nil
	}

	domain = newDomain(domainName, host)
	host.domains[domainName] = domain

	err := domain.Start()
	if err != nil {
		return nil, err
	}

	return domain, nil
}


func (host *host) stopDomainIfIdle(d *domain) bool {
	host.domainsMu.Lock()
	defer host.domainsMu.Unlock()

	didStop := false

	domainName := d.DomainName()
	if host.domains[domainName] == d {
		dctx := d.Ctx()

		// With the domain's ch session mutex locked, we can reliably call CtxChildCount
		if dctx.CtxChildCount() == 0 {
			didStop = dctx.CtxStop("idle domain auto stop", nil)
			delete(host.domains, domainName)
		}
	}

	return didStop
}


func (sess *hostSess) Start() error {

	sess.Go("msgInbox", func(p process.Context) {
		for running := true; running; {
			select {
			case msg := <-sess.msgInbox:
				sess.dispatchMsg(msg)
			case <-sess.Done():
				// Cancel all jobs

			}
		}
	})

	return nil
}

func (sess *hostSess) OnClosing() {
	fix me
	sess.cancelAllJobs()
}

func (sess *hostSess) cancelAllJobs() {

	sess.Info(2, "canceling all jobs")
	jobsCanceled := 0
	sess.openReqsMu.Lock()
	for _, job := range sess.openReqs {
		if job.isCanceled() == false {
			jobsCanceled++
			job.cancelJob()
		}
	}
	sess.openReqsMu.Unlock()
	if jobsCanceled > 0 {
		sess.Infof(1, "canceled %v jobs", jobsCanceled)
	}
}

func (sess *hostSess) lookupJob(reqID uint32) *reqJob {
	sess.openReqsMu.Lock()
	job := sess.openReqs[reqID]
	sess.openReqsMu.Unlock()
	return job
}

func (sess *hostSess) removeJob(reqID uint32) {
	sess.openReqsMu.Lock()
	delete(sess.openReqs, reqID)
	sess.openReqsMu.Unlock()

	// Send an empty msg to wake up and check for shutdown
	sess.msgOutbox <- nil
}

func (sess *hostSess) numJobsOpen() int {
	sess.openReqsMu.Lock()
	N := len(sess.openReqs)
	sess.openReqsMu.Unlock()
	return N
}

func (sess *hostSess) dispatchMsg(msg *Msg) {
	if msg == nil {
		return
	}

	// Get (or make) a new req job using the given ReqID
	msgOp := msg.OpCode()
	sess.openReqsMu.Lock()
	job := sess.openReqs[msg.ReqID]
	if job == nil && msgOp != arc.MsgOp_ReqDiscard {
		job = sess.newJob(msg)
		sess.openReqs[msg.ReqID] = job
	}
	sess.openReqsMu.Unlock()

	var err error
	if msgOp == arc.MsgOp_ReqDiscard {
		if job != nil {
			job.cancelJob()
		} else {
			err = arc.ErrCode_ReqNotFound.Err()
		}
	} else {
		err = job.nextMsg(msg)
	}

	if err != nil {
		sess.msgOutbox <- msg.newReqDiscard(err)
	}
}


func (sess *hostSess) EncodeToTxAndSign(txOp *TxOp) (*Tx, error) {

	if txOp == nil {
		return nil, arc.ErrCode_NothingToCommit.ErrWithMsg("missing txOp")
	}

	if len(txOp.Entries) == 0 {
		return nil, arc.ErrCode_NothingToCommit.ErrWithMsg("no entries to commit")
	}

	if txOp.ChannelGenesis == false && len(txOp.ChStateURI.ChID_TID) < 16 {
		return nil, arc.ErrCode_NothingToCommit.ErrWithMsg("invalid ChID (missing TID)")
	}

	//
	// TODO
	//
	// placeholder until tx encoding and signing is
	var TID TIDBuf
	mrand.Read(TID[:])

	tx := &Tx{
		TID:  TID[:],
		TxOp: txOp,
	}

	if txOp.ChannelGenesis {
		// if len(uri.ChID) > 0 {
		// 	return arc.ErrCode_InvalidURI.ErrWithMsg("URI must be a domain name and not be a path")
		// }
		txOp.ChStateURI.ChID_TID = tx.TID
		txOp.ChStateURI.ChID = TID.Base32()
	}

	return tx, nil
}


func (sess *hostSess) newJob(newReq *Msg) *reqJob {
	job := &reqJob{
		sess: sess,
	}
	job.msgs = job.scrap[:0]
	return job
}







// rename txJob? chReq?
type reqJob struct {
	chOp     MsgOp
	chReq    MsgOp // non-zero if this is a query op
	sess     *hostSess  // Parent host session
	msgs     []*Msg  // msgs for this job
	chURI    ChStateURI // Set from chOp
	canceled bool       // Set if this job is to be discarded
	chSub    chSub      // If getOp is arc.MsgOp_Subscribe
	final    bool

	scrap [4]*Msg
}

func (job *reqJob) nextMsg(msg *Msg) error {

	if job.final {
		job.sess.Warnf("client sent ReqID already in use and closed (ReqID=%v)", msg.ReqID)
		return nil
	}

	addMsgToJob := false

	var err error
	opCode := msg.OpCode()
	switch opCode {
	case arc.MsgOp_ChOpen, arc.MsgOp_ChGenesis:
		if job.reqType != 0 {
			err = arc.ErrCode_UnsupportedOp.ErrWithMsg("multi-channel ops not supported")
			break
		}
		job.chOp = msg.Ops
		err = job.chURI.AssignFromURI(msg.Keypath)
	case arc.MsgOp_Get:
		if job.reqType == 0 {
			job.reqType = chQuery
		} else if job.reqType != chQuery {
			err = arc.ErrCode_UnsupportedOp.ErrWithMsg("multi-channel ops not supported")
			break
		}
		if job.getOp != 0 {
			err = arc.ErrCode_UnsupportedOp.ErrWithMsg("multi-get ops not supported")
			break
		}
		if job.chOp != 0 {
			err = arc.ErrCode_UnsupportedOp.ErrWithMsg("no channel URI specified for channel query")
			break
		}
		addMsgToJob = true
		job.getOp = msg.Ops
	case arc.MsgOp_PushAttr:

	case arc.MsgOp_AccessGrant:

		// case arc.MsgOp_Get, arc.MsgOp_Subscribe:
	// 	if job.chOp != nil {
	// 		err = arc.ErrCode_FailedToOpenChURI.ErrWithMsg("multiple get ops")
	// 		break
	// 	}
	// 	job.getOp = msg

	}

	if addMsgToJob {
		job.msgs = append(job.msgs, msg)
	}


	// switch msg.OpCode() {
	// case arc.MsgOp_ChOpen, arc.MsgOp_ChGenesis:
	// 	if len(job.chURI.Domain
	// 	if job.opCode != nil {
	// 		err = arc.ErrCode_FailedToOpenChURI.ErrWithMsg("multiple channel open ops")
	// 		break
	// 	}
	// 	job.chOp = msg
	// 	err = job.chURI.AssignFromURI(job.chOp.Keypath)
	// case arc.MsgOp_Get, arc.MsgOp_Subscribe:
	// 	if job.chOp != nil {
	// 		err = arc.ErrCode_FailedToOpenChURI.ErrWithMsg("multiple get ops")
	// 		break
	// 	}
	// 	job.getOp = msg

	// }

	// job.msgs = append(job.scrap[:], msg)

	// if (msg.Ops & arc.MsgOp_ReqComplete) != 0 {
	// 	job.final = true

	// 	go job.exeJob()
	// }

}

func (job *reqJob) OnMsg(msg *arc.Msg) error {

	for i := 0; i < 2; i++ {

		// Normally, the msg should be able to be buffer-queued in the session output (and we immediately return)
		select {
		case job.sess.msgOutbox <- msg:
			return nil
		default:
			// If we're here, the session outbox is somehow backed up.
			// We wait the smallest lil bit to see if that does it
			if i == 0 {
				runtime.Gosched()
			} else if i == 1 {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}

	return arc.arc.ErrCode_ClientNotResponding.Err()
}

// Debugf prints output to the output log
func (job *reqJob) Debugf(msgFormat string, msgArgs ...interface{}) {
	job.sess.Infof(2, msgFormat, msgArgs...)
}

// canceled returns true if this job should back out of all work.
func (job *reqJob) isCanceled() bool {
	return job.canceled
}

func (job *reqJob) cancelJob() {
	job.canceled = true
	if job.chSub != nil {
		job.chSub.Close()
	}
}

func (job *reqJob) exeGetOp() error {
	var err error
	job.chSub, err = job.sess.host.OpenChSub(job.req)
	if err != nil {
		return err
	}
	defer job.chSub.Close()

	// Block while the chSess works and outputs ch entries to send from the ch session.
	// If/when the chSess see the job ctx stopping, it will unwind and close the outbox
	{
		for msg := range job.chSub.Outbox() {
			job.sess.msgOutbox <- msg
		}
	}

	return nil
}

func (job *reqJob) exeTxOp() (*Node, error) {

	if job.req.TxOp.ChStateURI == nil {
		job.req.TxOp.ChStateURI = job.req.ChStateURI
	}

	tx, err := job.sess.hostSess.EncodeToTxAndSign(job.req.TxOp)
	if err != nil {
		return nil, err
	}

	// TODO: don't release this op until its merged or rejected (required tx broadcast)
	err = job.sess.srv.host.SubmitTx(tx)
	if err != nil {
		return nil, err
	}

	node := job.newResponse(NodeOp_ReqComplete)
	node.Attachment = append(node.Attachment[:0], tx.TID...)
	node.Str = path.Join(job.req.ChStateURI.DomainName, TID(tx.TID).Base32())

	return node, nil
}

func (job *reqJob) exeJob() {
	var err error
	var node *Node

	// Check to see if this req is canceled before beginning
	if err == nil {
		if job.isCanceled() {
			err = arc.ErrCode_ReqCanceled.Err()
		}
	}

	if err == nil {
		if job.req.ChStateURI == nil && len(job.req.ChURI) > 0 {
			job.req.ChStateURI = &ChStateURI{}
			err = job.req.ChStateURI.AssignFromURI(job.req.ChURI)
		}
	}

	if err == nil {
		switch job.req.ReqOp {

		case ChReqOp_Auto:
			switch {
			case job.req.GetOp != nil:
				err = job.exeGetOp()
			case job.req.TxOp != nil:
				node, err = job.exeTxOp()
			}

		default:
			err = arc.ErrCode_UnsupportedOp.Err()
		}
	}

	// Send completion msg
	{
		if err == nil && job.isCanceled() {
			err = arc.ErrCode_ReqCanceled.Err()
		}

		if err != nil {
			node = job.req.newResponse(NodeOp_ReqDiscarded, err)
		} else if node == nil {
			node = job.newResponse(NodeOp_ReqComplete)
		} else if node.Op != NodeOp_ReqComplete && node.Op != NodeOp_ReqDiscarded {
			panic("this should be msg completion")
		}

		job.sess.nodeOutbox <- node
	}

	job.sess.removeJob(job.req.ReqID)
}



func (req *Msg) newReqDiscard(err error) *Msg {
	msg := &Msg{
		Ops:   arc.MsgOp_ReqDiscard,
		ReqID: req.ReqID,
	}

	if err != nil {
		var reqErr *ReqErr
		if reqErr, _ = err.(*ReqErr); reqErr == nil {
			err = arc.ErrCode_UnnamedErr.Wrap(err)
			reqErr = err.(*ReqErr)
		}
		msg.Buf = bufs.SmartMarshal(reqErr, msg.Buf)
	}
	return msg
}

func (job *reqJob) newResponse(op NodeOp) *Node {
	return job.req.newResponse(op, nil)
}


*/
