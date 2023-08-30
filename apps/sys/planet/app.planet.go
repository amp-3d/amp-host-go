package planet

import (
	"encoding/binary"
	"fmt"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/symbol"
	"github.com/arcspace/go-arc-sdk/stdlib/task"
	"github.com/arcspace/go-archost/apps/sys"
	"github.com/arcspace/go-archost/arc/badger/symbol_table"
	"github.com/dgraph-io/badger/v4"
)

// King Jesus, our Lord's Christ, is a servant, our advocate!
const (
	AppID           = "planet" + sys.AppFamilyDomain
	HomePlanetAlias = "~" // hard-wired alias for the the current user's home planet
)

var AppUID = arc.FormUID(0xda1949fbe7a642de, 0xaa4dba7a3f939f27)

func RegisterApp(reg arc.Registry) {
	reg.RegisterApp(&arc.AppModule{
		AppID:   AppID,
		UID:     AppUID,
		Desc:    "cell storage & sync",
		Version: "v1.2023.2",
		NewAppInstance: func() arc.AppInstance {
			return &appCtx{
				plSess: make(map[string]*plSess),
			}
		},
	})
}

type appCtx struct {
	arc.AppContext
	home   *plSess            // current user's home planet
	plSess map[string]*plSess // mounted planets
	plMu   sync.Mutex         // plSess mutex

	// AppBase is off-limits for this app b/c it bootstraps the system.
	// the system, in order to boot, requires cell storage to load and store.
	// Or in other words, the genesis bootstrap comes from the genesis planet
}

func (app *appCtx) OnNew(ctx arc.AppContext) error {
	app.AppContext = ctx
	return app.mountHomePlanet()
}

func (app *appCtx) HandleURL(*url.URL) error {
	return arc.ErrUnimplemented
}

// func (app *appCtx) WillPinCell(res arc.CellResolver, req arc.PinReq) error {
// 	// do we need to close all mounted planets?
// }

func (app *appCtx) OnClosing() {
	// do we need to close all mounted planets?
}

func (app *appCtx) mountHomePlanet() error {
	if app.home != nil {
		return nil
	}

	login := app.Session().LoginInfo()
	if len(login.UserUID) == 0 {
		return arc.ErrCode_PlanetFailure.Errorf("no user logged in")
	}

	var err error
	app.home, err = app.mountPlanet(login.UserUID)
	if err != nil {
		return err
	}

	// TODO: this should be done by the host somehow?
	app.Session().InitSessionRegistry(app.home.symTable)
	return nil
}

func (app *appCtx) resolvePlanet(alias string) (*plSess, error) {
	if len(alias) == 0 {
		return nil, arc.ErrCode_PlanetFailure.Errorf("missing planet alias")
	}

	if alias == HomePlanetAlias {
		return app.home, nil
	}

	panic("TODO: planet db name is string vers of arc.UID")
	//planetID := app.home.GetSymbolID([]byte(alias), false)
}

func (app *appCtx) PinCell(parent arc.PinnedCell, req arc.PinReq) (arc.PinnedCell, error) {
	var pl *plSess

	// arc://planet/<planet-alias>/<cell-scope>/<cell-alias>
	// url part       ----0----     ----1----    ----2----
	// e.g. "~/settings/main-profile.AssetRef"
	urlParts := req.URLPath()
	if len(urlParts) != 3 {
		return nil, arc.ErrCode_CellNotFound.Error("invalid planet URI")
	}

	for {
		var err error
		pl, err = app.resolvePlanet(urlParts[0])
		if err != nil {
			return nil, err
		}
		if pl.Context.PreventIdleClose(time.Second) {
			break
		}
	}

	// TODO: security risk to allow symbols to be made
	scopeID := pl.GetSymbolID([]byte(urlParts[1]), true)
	cellID := pl.GetSymbolID([]byte(urlParts[2]), true)

	cell, err := pl.getCell(scopeID, cellID)
	if err != nil {
		return nil, err
	}

	return cell, nil
}

// mountPlanet mounts the given planet by ID, or creates a new one if genesis is non-nil.
func (app *appCtx) mountPlanet(
	dbName string,
) (*plSess, error) {

	// if planetID == 0 {
	// 	if genesis == nil {
	// 		return nil, arc.ErrCode_PlanetFailure.Error("missing PlanetID and PlanetEpoch TID")
	// 	}
	// 	planetID = app.plHome.GetSymbolID(genesis.EpochTID, false)
	// }

	app.plMu.Lock()
	defer app.plMu.Unlock()

	// Check if already mounted
	pl := app.plSess[dbName]
	if pl != nil {
		return pl, nil
	}

	// var fsName string
	// if planetID == host.homePlanetID {
	// 	fsName = "HostHomePlanet"
	// } else if planetID != 0 {
	// 	fsName = string(host.home.GetSymbol(planetID, nil))
	// 	if fsName == "" && genesis == nil {
	// 		return nil, arc.ErrCode_PlanetFailure.Errorf("planet ID=%v failed to resolve", planetID)
	// 	}
	// } else {

	// 	asciiTID := bufs.Base32Encoding.EncodeToString(genesis.EpochTID)
	// 	fsName = utils.MakeFSFriendly(genesis.CommonName, nil) + " " + asciiTID[:6]
	// 	planetID = host.home.GetSymbolID([]byte(fsName), true)

	// 	// Create new planet ID entries that all map to the same ID value
	// 	host.home.SetSymbolID([]byte(asciiTID), planetID)
	// 	host.home.SetSymbolID(genesis.EpochTID, planetID)
	// }

	pl = &plSess{
		app:    app,
		dbName: dbName,
		cells:  make(map[arc.CellID]*plCell),
	}

	dbPath := path.Join(app.LocalDataPath(), dbName)

	// The db should already exist if opening and vice versa
	// _, err := os.Stat(dbPath)
	// if genesis != nil && err == nil {
	// 	return nil, arc.ErrCode_PlanetFailure.Error("planet db already exists")
	// }

	// Limit ValueLogFileSize to ~134mb since badger does a mmap size test on init, causing iOS 13 to error out.
	// Also, massive value file sizes aren't appropriate for mobile.  TODO: make configurable.
	var err error
	dbOpts := badger.DefaultOptions(dbPath)
	dbOpts.Logger = nil
	dbOpts.ValueLogFileSize = 1 << 27
	pl.db, err = badger.Open(dbOpts)
	if err != nil {
		return nil, err
	}

	opts := symbol_table.DefaultOpts()
	opts.Db = pl.db
	opts.IssuerInitsAt = symbol.ID(arc.ConstSymbol_IssuerInitsAt)
	pl.symTable, err = opts.CreateTable()
	if err != nil {
		pl.db.Close()
		pl.db = nil
		return nil, err
	}

	pl.Context, err = app.StartChild(&task.Task{
		// TODO: home planet should not close -- solution:
		// IdleClose: 120 * time.Second,
		Label: fmt.Sprint("planet: ", dbName),
		OnChildClosing: func(child task.Context) {
			pl.cellsMu.Lock()
			delete(pl.cells, child.TaskRef().(arc.CellID))
			pl.cellsMu.Unlock()
		},
		OnClosed: func() {
			pl.app.onPlanetClosed(pl)
			if pl.symTable != nil {
				pl.symTable.Close()
			}
			if pl.db != nil {
				pl.db.Close()
			}
		},
	})
	if err != nil {
		return nil, err
	}
	pl.Context.Info(1, "mounted '", dbPath, "'")

	app.plSess[pl.dbName] = pl
	return pl, nil
}

func (app *appCtx) onPlanetClosed(pl *plSess) {
	app.plMu.Lock()
	delete(app.plSess, pl.dbName)
	app.plMu.Unlock()
}

// Db key format:
//
//	AppSID + NodeSID + AttrNamedTypeSID [+ SI] => ElemVal
const (
	kScopeOfs = 0 // byte offset of AppID
	kNodeOfs  = 4 // byte offset of NodeID
	kAttrOfs  = 8 // byte offset of AttrID
)

// plSess represents a "mounted" planet (a Cell database), allowing it to be accessed, served, and updated.
type plSess struct {
	task.Context

	app      *appCtx
	symTable symbol.Table           // each planet has separate symbol tables
	dbName   string                 // local pathname to db
	db       *badger.DB             // db access
	cells    map[arc.CellID]*plCell // working set of mounted cells (pinned or idle)
	cellsMu  sync.Mutex             // cells mutex
	//planetID uint32                 // symbol ID (as known by the host's symbol table)
}

// plCell is a "mounted" cell serving requests for a specific cell (typically 0-2 open plReq).
// This can be thought of as the controller for one or more active cell pins.
// *** implements arc.PinnedCell ***
type plCell struct {
	arc.CellID
	pl      *plSess       // parent planet
	ctx     task.Context  // arc.PinnedCell ctx
	newTxns chan *arc.Msg // txns to merge
	dbKey   [kAttrOfs]byte
	// newSubs  chan *plReq        // new requests waiting for state
	// subsHead *plReq             // single linked list of open reqs on this cell
	// subsMu   sync.Mutex         // mutex for subs
	// newReqs  chan *appReq      // new requests waiting for state
	// idleSecs int32              // ticks up as time passes when there are no subs
}

// plContext?
// pinContext?
// plSub?
// plCellSub?
// plCellReq?
// *** implements arc.PinContext ***
// Instantiated by a client pinning a cell.
// type plReq struct {
// 	client arc.PinContext // given by the runtime via CellResolver.ResolveCell()
// 	parent *plCell        // parent cell controller
// 	next   *plReq         // single linked list of same-cell reqs
// }
/*



    // ValType_AttrSet            = 4; // .ValInt is a AttrSet CellID
    // ValType_NameSet            = 5; // CellID+AttrID+NameID     => Msg.(Type)          Values only 
    // ValType_CellSet            = 6; // CellID+AttrID+CellID     => Cell_NID            AttrSet NIDs only 
    // ValType_Series             = 8; // CellID+AttrID+TSI+FromID => Msg.(Type)          Values only
    // ValType_CellRef            = 20; // .FromID and .SI together identify a cell
    // ValType_CellSetID          = 21; // .ValInt is a CellSet ID (used for SetValType_CellSet)


func (pl *plSess) ReadAttr(ctx arc.AppContext, nodeKey, attrKey string, decoder func(valData []byte) error) error {
	appSID := pl.GetSymbolID([]byte(ctx.StateScope()), false)
	nodeSID := pl.GetSymbolID([]byte(nodeKey), false)
	attrSID := pl.GetSymbolID([]byte(attrKey), false)

	var keyBuf [64]byte
	key := symbol.AppendID(symbol.AppendID(symbol.AppendID(keyBuf[:0], appSID), nodeSID), attrSID)

	dbTx := pl.db.NewTransaction(false)
	defer dbTx.Discard()

	item, err := dbTx.Get(key)
	if err == badger.ErrKeyNotFound {
		return arc.ErrCellNotFound
	}

	err = item.Value(decoder)
	if err != nil {
		return arc.ErrCode_CellNotFound.Errorf("failed to read cell: %v", err)
	}

	return nil
}

func (pl *plSess) WriteAttr(ctx arc.AppContext, nodeKey, attrKey string, valData []byte) error {
	appSID := pl.GetSymbolID([]byte(ctx.StateScope()), true)
	nodeSID := pl.GetSymbolID([]byte(nodeKey), true)
	attrSID := pl.GetSymbolID([]byte(attrKey), true)

	var keyBuf [64]byte
	key := symbol.AppendID(symbol.AppendID(symbol.AppendID(keyBuf[:0], appSID), nodeSID), attrSID)

	if appSID == 0 || nodeSID == 0 || attrSID == 0 {
		return arc.ErrCellNotFound
	}

	dbTx := pl.db.NewTransaction(true)
	defer dbTx.Discard()

	dbTx.Set(key, valData)
	err := dbTx.Commit()
	if err != nil {
		return arc.ErrCode_DataFailure.Errorf("failed to write cell: %v", err)
	}

	return nil
}
*/

func (pl *plSess) GetSymbolID(value []byte, autoIssue bool) uint32 {
	return uint32(pl.symTable.GetSymbolID(value, autoIssue))
}

func (pl *plSess) GetSymbol(ID uint32, io []byte) []byte {
	return pl.symTable.GetSymbol(symbol.ID(ID), io)
}

func (pl *plSess) SetSymbolID(value []byte, ID uint32) uint32 {
	return uint32(pl.symTable.SetSymbolID(value, symbol.ID(ID)))
}

func (pl *plSess) getCell(scopeID, nodeID uint32) (cell *plCell, err error) {
	if nodeID == 0 {
		return nil, arc.ErrCellNotFound
	}

	pl.cellsMu.Lock()
	defer pl.cellsMu.Unlock()

	cellID := arc.CellID(uint64(scopeID)<<32 | uint64(nodeID))

	// If the cell is already open, make sure it doesn't auto-close while we are handling it.
	cell = pl.cells[cellID]
	if cell != nil && cell.ctx.PreventIdleClose(time.Second) {
		return cell, nil
	}

	cell = &plCell{
		pl:      pl,
		newTxns: make(chan *arc.Msg),
	}
	cell.CellID = cellID

	binary.BigEndian.PutUint32(cell.dbKey[kScopeOfs:], scopeID)
	binary.BigEndian.PutUint32(cell.dbKey[kNodeOfs:], nodeID)

	// Start the cell context as a child of the planet db it belongs to
	cell.ctx, err = pl.Context.StartChild(&task.Task{
		Label:   cell.GetLogLabel(),
		TaskRef: cellID,
		OnRun: func(ctx task.Context) {

			// Manage incoming subs, push state to subs, and then maintain state for each sub
			for running := true; running; {
				select {
				case tx := <-cell.newTxns:
					err := cell.mergeUpdate(tx)
					if err != nil {
						ctx.Error(err)
					}

				case <-ctx.Closing():
					running = false
				}
			}
		},
	})
	if err != nil {
		return
	}

	pl.cells[cellID] = cell
	return
}

/*

func (ctx *appContext) startCell(req *appReq) error {
	if req.cell != nil {
		panic("already has sub")
	}

	cell, err := ctx.bindAndStart(req)
	if err != nil {
		return err
	}
	req.cell = cell

	// Add incoming req to the cell IDs list of subs
	cell.subsMu.Lock()
	{
		prev := &cell.subsHead
		for *prev != nil {
			prev = &((*prev).next)
		}
		*prev = req
		req.next = nil

		cell.newReqs <- req
	}
	cell.idleSecs = 0
	cell.subsMu.Unlock()

	return nil
}



func (sess *hostSess) ServeState(req *appReq) error {

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
	spec, err := sess.SessionRegistry.GetResolvedNodeSpec(mapAs)
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

	nSess.pushState(req)

	return nil
}


func (pl *planetSess) onRun(task.Context) {
	const period = 60
	timer := time.NewTicker(period * time.Second)
	for running := true; running; {
		select {
		case <-timer.C:
			pl.closeIdleCells(period)
		case <-pl.Closing():
			running = false
		}
	}
	timer.Stop()
}


func (pl *plSess) addSub(req arc.PinReq) error {

	// Add incoming req to the cell IDs list of subs
	cell.subsMu.Lock()
	{
		prev := &cell.subsHead
		for *prev != nil {
			prev = &((*prev).next)
		}
		*prev = req
		req.next = nil
		cell.newSubs <- req
	}
	//cell.idleSecs = 0
	cell.subsMu.Unlock()

	return nil
}

func (cell *plCell) removeSub(req *plReq) {
	// cell := req.cell
	// if cell == nil { // || req.closed != 0 {
	// 	return
	// }
	if req.parent != cell {
		return
	}

	cell.subsMu.Lock()
	{
		req.parent = nil
		prev := &cell.subsHead
		for *prev != req && *prev != nil {
			prev = &((*prev).next)
		}
		if *prev == req {
			*prev = req.next
			req.next = nil
		} else {
			panic("failed to find sub")
		}
	}
	cell.subsMu.Unlock()

	// N := len(csess.subs)
	// for i := 0; i < N; i++ {
	// 	if csess.subs[i] == remove {
	// 		N--
	// 		csess.subs[i] = csess.subs[N]
	// 		csess.subs[N] = nil
	// 		csess.subs = csess.subs[:N]
	// 		break
	// 	}
	// }
}
*/

// zig-zag encoding
func encodeSI(val int64) uint64 {
	return uint64(val<<1) ^ uint64(val>>63)
}

func decodeSI(raw uint64) int64 {
	return int64((raw >> 1) ^ (-(raw & 1)))
}

func (cell *plCell) Info() arc.CellID {
	return cell.CellID
}

func (cell *plCell) PinCell(req arc.PinReq) (arc.PinnedCell, error) {
	// Hmmm, what does it even mean for this or child cells to be pinned by a client?
	// Is the idea that child cells are either implicit links to cells in the same planet or explicit links to cells in other planets?
	return nil, arc.ErrCode_Unimplemented.Error("TODO: implement cell pinning")
}

func (cell *plCell) Context() task.Context {
	return cell.ctx
}

func (cell *plCell) GetLogLabel() string {
	return fmt.Sprintf("cell: %0x", cell.CellID)
}

func (cell *plCell) ServeState(ctx arc.PinContext) error {
	dbTx := cell.pl.db.NewTransaction(false)
	defer dbTx.Discard()

	opts := badger.IteratorOptions{
		PrefetchValues: true,
		PrefetchSize:   100,
		Prefix:         cell.dbKey[:],
	}

	itr := dbTx.NewIterator(opts)
	defer itr.Close()

	sess := cell.pl.app.Session()
	params := ctx.Params()
	useClientIDs := (params.PinReq.Flags & arc.PinFlags_UseNativeSymbols) == 0

	valsBuf := make([]byte, 0, 1024)

	var cellTx *arc.CellTxPb

	// cellTxs := make([]*arc.CellTxPb, 0, 4)
	// cellTxs = append(cellTxs, cellTx)

	// Read the Cell from the db and push enabled (current all) attrs it to the client
	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()

		// TODO: read child vs parent cell attrs

		send := true

		attrID := binary.BigEndian.Uint32(item.Key()[kAttrOfs:])
		if useClientIDs {
			attrID, send = sess.NativeToClientID(attrID)
		}

		//
		// TODO -- read SI
		SI_raw := uint64(0)

		if !send {
			continue
		}

		{
			elem := &arc.AttrElemPb{
				AttrID: uint64(attrID),
				SI:     decodeSI(SI_raw),
			}

			err := item.Value(func(valBuf []byte) error {
				pos := len(valsBuf)
				valsBuf = append(valsBuf, valBuf...)
				elem.ValBuf = valsBuf[pos : len(valsBuf)-pos]
				return nil
			})
			if err != nil {
				cell.ctx.Warnf("failed to read attr %d: %v", attrID, err)
				continue
			}

			if cellTx == nil {
				cellTx = &arc.CellTxPb{
					Op:         arc.CellTxOp_UpsertCell,
					TargetCell: int64(cell.CellID),
					Elems:      make([]*arc.AttrElemPb, 0, 4),
				}
			}
			cellTx.Elems = append(cellTx.Elems, elem)
		}
	}

	msg := arc.NewMsg()
	msg.Status = arc.ReqStatus_Synced
	if cellTx != nil {
		msg.CellTxs = append(msg.CellTxs, cellTx)
	}
	return ctx.PushUpdate(msg)
}

func (cell *plCell) MergeUpdate(tx *arc.Msg) error {
	select {
	case cell.newTxns <- tx:
		return nil // IDEA: maybe we just block until the txn is merged?  this lets us report errors
	case <-cell.ctx.Closing():
		return arc.ErrShuttingDown
	}
}

// TODO: handle SIs and children!
func (cell *plCell) mergeUpdate(tx *arc.Msg) error {
	dbTx := cell.pl.db.NewTransaction(true)
	defer dbTx.Discard()

	// Keep a key buf to store keys until we call Commit()
	var keyBuf [512]byte
	key := keyBuf[:0]

	for _, cellTx := range tx.CellTxs {

		// TODO: handle child cells
		if cellTx.TargetCell != 0 && arc.CellID(cellTx.TargetCell) != cell.CellID {
			cell.ctx.Warnf("unsupported child cell merge %d", cellTx.TargetCell)
			continue
		}

		// Hacky -- auto-detect if we need to serialize
		// if len(cellTx.ElemsPb) == 0 {
		// 	if err := cellTx.MarshalAttrs(); err != nil {
		// 		return nil
		// 	}
		// }

		R := 0
		for _, elem := range cellTx.Elems {
			cellKey := append(key[R:R], cell.dbKey[:]...)
			key = binary.BigEndian.AppendUint32(cellKey, uint32(elem.AttrID))
			R = len(key)
			err := dbTx.Set(key, elem.ValBuf)
			if err != nil {
				return err
			}
		}
	}

	err := dbTx.Commit() // TODO: handle conflict
	if err != nil {
		return err
	}

	cell.pushToSubs(tx)
	return nil
}

func (cell *plCell) pushToSubs(src *arc.Msg) {
	var childBuf [8]task.Context
	subs := cell.ctx.GetChildren(childBuf[:0])

	// TODO: lock sub while it's being pushed to?
	//   Maybe each sub maintains a queue of Msg channels that push to it?
	//   - when the queue closes, it moves on to the next
	//   - this would ensure a sub doesn't get 2+ msg batches at once
	//   - see related comments for appReq
	for _, sub := range subs {
		if sub.PreventIdleClose(time.Second) {
			if subCtx, ok := sub.TaskRef().(arc.PinContext); ok {
				if subCtx.Params().PinReq.Flags&arc.PinFlags_NoSync != 0 {
					continue
				}

				msg := arc.NewMsg()
				msg.Status = src.Status
				if cap(msg.CellTxs) < len(src.CellTxs) {
					msg.CellTxs = make([]*arc.CellTxPb, len(src.CellTxs))
				} else {
					msg.CellTxs = msg.CellTxs[:len(src.CellTxs)]
				}

				for j, srcTx := range src.CellTxs {
					msg.CellTxs[j] = &arc.CellTxPb{
						Op:         srcTx.Op,
						TargetCell: int64(srcTx.TargetCell),
						Elems:      srcTx.Elems,
					}
				}
				err := subCtx.PushUpdate(msg)
				if err != nil {
					subCtx.Warn("failed to push update: ", err)
				}
			}
		}
	}
}

/*

// WIP -- to be replaced by generic cell storage
const (
	kUserTable   = byte(0xF1)
	kCellStorage = byte(0xF2)
)

// This will be replaced in the future with generic use of GetCell() with a "user" App type.
// For now, just make a table with user IDs their respective user record.
func (pl *planetSess) getUser(req arc.Login, autoCreate bool) (seat arc.UserSeat, err error) {
	var buf [128]byte

	uid := append(buf[:0], "/UID/"...)
	uid = append(uid, req.UserUID...)
	userID := pl.symTable.GetSymbolID(uid, autoCreate)
	if userID == 0 {
		return arc.UserSeat{}, arc.ErrCode_LoginFailed.Error("unknown user")
	}

	dbTx := pl.db.NewTransaction(true)
	defer dbTx.Discard()

	// For now, just make a table with user IDs their respective user record.
	key := append(buf[:0], kUserTable) // User record table
	key = userID.WriteTo(key)
	item, err := dbTx.Get(key)
	if err == badger.ErrKeyNotFound && autoCreate {
		seat.UserID = uint64(userID)
		seat.HomePlanetID = pl.planetID /// TODO: do user planet genesis here!
		seatBytes, _ := seat.Marshal()
		dbTx.Set(key, seatBytes)
		err = dbTx.Commit()
	} else if err == nil {
		err = item.Value(func(val []byte) error {
			return seat.Unmarshal(val)
		})
	}

	if err != nil {
		panic(err)
	}

	return
}

// WIP -- placeholder hack until cell+attr support is added to the db
// Full replacement of all attrs is not how this will work in the future -- this is just a placeholder
func (pl *planetSess) ReadCell(cellKey []byte, schema *arc.AttrSchema, msgs func(msg *arc.Msg)) error {
	cellSymID := pl.symTable.GetSymbolID(cellKey, false)
	if cellSymID == 0 {
		return arc.ErrCode_CellNotFound.Error("cell not found")
	}

	var buf [128]byte
	key := append(buf[:0], kCellStorage)
	key = cellSymID.WriteTo(key)

	dbTx := pl.db.NewTransaction(false)
	defer dbTx.Discard()
	item, err := dbTx.Get(key)
	if err == badger.ErrKeyNotFound {
		return arc.ErrCode_CellNotFound.Error("cell not found")
	}

	var txn arc.Txn
	err = item.Value(func(val []byte) error {
		return txn.Unmarshal(val)
	})

	if err != nil {
		return arc.ErrCode_CellNotFound.Errorf("failed to read cell: %v", err)
	}

	for _, msg := range txn.Msgs {
		msgs(msg)
	}

	return err
}





func (csess *cellInst) ServeState(req *nodeReq) error {
	//target := req.req.TargetNode()
	csess.registerPin(sub)

	dbTx := csess.pl.db.NewTransaction(false)
	defer dbTx.Discard()

	// A node maps to current state where each db itr step corresponds to an attr ID:
	//    cellID+AttrID           => Msg
	// An attr declared as a series has keys of the form:
	//    cellID+AttrID+SI+FromID => Msg  (future: SI+FromID is replaced by SI+CollisionSortByte)
	//
	// This means:
	//    - future machinery can be added to perform multi-node locking txns (that update the state node and push to subs)
	//    - AttrIDs can be reserved to denote special internal state values -- e.g. revID, NodeType (NodeSpecID)
	//    - idea: edits to values name the SI, FromID, and RevID they are replacing in order to be accepted?



	// 2 possible approaches:
	//    node maps to state: a node ID maps to the "state" node, where each db itr step corresponds to an attr ID
	//       => a time series is its own node type (where each node entry key is SI rather than an attr ID)
	//       => machinery to perform multi-node locking txns would then update the node safely (and push to subs) -- but can be deferred (tech debt)
	//    node embeds all history: node key has AttrID+SI+From suffix (non-trivial impl)
	//       => reading node state means seeking the latest SI of each attr
	//       => merging a Tx only pushes state if its SI maps/replaces what is already mapped  (non-trivial impl)
	//       => multi-node locking ops get MESSY

	// Form key for target node to read, seek, and read and send each node entry / attr
	var keyBuf [64]byte
	baseKey := append(keyBuf[:0], csess.keyPrefix[:]...)

	// Announce the new node (send SetTypeID, ItemTypeID, map mode etc)
	msg := arc.NewMsg()
	msg.Op = arc.MsgOp_AnnounceNode
	msg.SetValue(&req.target)
	err := req.PushUpdate(msg)
	if err == nil {
		return err
	}


	for _, attr := range req.spec.Attrs {

		if req.isClosed() {
			break
		}

		attrKey := symbol.ID(attr.AttrID).WriteTo(baseKey)

		if attr.SeriesType == arc.SeriesType_0 {
			item, getErr := dbTx.Get(attrKey)
			if getErr == nil {
				err = req.unmarshalAndPush(item)
				if err != nil {
					csess.Error(err)
				}
			}
		} else if attr.AutoPin == arc.AutoPin_All {

			switch attr.SeriesType {



			}

		}

		// case arc.

		// case SeriesType_U16:

		// }


		if err != nil {
			return err
		}
		// autoMap := attr.AutoMap
		// if autoMap == arc.AutoMap_ForType {

		// 	switch arc.Type(attr.SetTypeID) {
		// 	case arc.Type_TimeSeries:

		// 	case arc.Type_AttrSet,
		// 		arc.Type_TimeSeries:
		// 		autoMap = arc.AutoMap_No
		// 	case arc.Type_NameSet:
		// 	}
		// }

		switch attr.AutoPin {
		case arc.AutoPin_All:

		}
	}

	// Send break when done
	msg = arc.NewMsg()
	msg.Op = arc.MsgOp_NodeUpdated
	err = req.PushUpdate(msg)
	if err != nil {
		return err
	}

	return nil
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
		fsName = string(host.home.GetSymbol(planetID, nil))
		if fsName == "" && genesis == nil {
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
		dbPath:   path.Join(host.Opts.StatePath, string(fsName)),
		cells:    make(map[arc.CellID]*cellInst),
		//newReqs:  make(chan *appReq, 1),
	}

	// The db should already exist if opening and vice versa
	_, err := os.Stat(pl.dbPath)
	if genesis != nil && err == nil {
		return nil, arc.ErrCode_PlanetFailure.Error("planet db already exists")
	}

	task := &task.Task{
		Label: fsName,
		OnStart: func(task.Context) error {

			// The host's home planet is ID issuer of all other planets
			opts := symbol_table.DefaultOpts()
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

func (host *host) HostPlanet() arc.Planet {
	return host.home
}



*/
