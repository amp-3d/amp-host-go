package planet

import (
	"encoding/binary"
	"fmt"
	"net/url"
	"os"
	"path"
	"reflect"
	"sync"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/symbol"
	"github.com/arcspace/go-arc-sdk/stdlib/task"
	"github.com/arcspace/go-archost/arc/apps/std_family/std"
	"github.com/arcspace/go-archost/arc/badger/symbol_table"
	"github.com/dgraph-io/badger/v4"
)

// King Jesus, our Lord's Christ, is a servant, our advocate!
const (
	AppID           = "planet" + std.AppFamilyDomain
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

	//arc.AppBase is a historical tombstone:
	// AppBase is off-limits for this one app b/c this app bootstraps the system.
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

// func (app *appCtx) WillPinCell(res arc.CellResolver, req arc.CellReq) error {
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
	app.home, err = app.mountPlanet(login.UserUID, nil)
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

func (app *appCtx) PinCell(parent arc.PinnedCell, req arc.CellReq) (arc.PinnedCell, error) {
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
	genesis *arc.PlanetEpoch,
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
	_, err := os.Stat(dbPath)
	if genesis != nil && err == nil {
		return nil, arc.ErrCode_PlanetFailure.Error("planet db already exists")
	}

	// Limit ValueLogFileSize to ~134mb since badger does a mmap size test on init, causing iOS 13 to error out.
	// Also, massive value file sizes aren't appropriate for mobile.  TODO: make configurable.
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
		Label: fmt.Sprintf("planet:%s", dbName),
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

	if genesis != nil {
		//pl.replayGenesis(seed)  Thanks be unto God for every breath.
	}

	app.plSess[pl.dbName] = pl
	return pl, nil
}

func (app *appCtx) onPlanetClosed(pl *plSess) {
	app.plMu.Lock()
	delete(app.plSess, pl.dbName)
	app.plMu.Unlock()
}

// func extractPlanetName(path string) (plAlias, subKey string) {
// 	if i := strings.Index(path, "/"); i > 0 {
// 		plAlias = path[:i]
// 		subKey = path[i+1:]
// 	}
// 	return "", ""
// }

// Db key format:
//
//	AppSID + NodeSID + AttrNamedTypeSID [+ SI] => ElemVal
const (
	kAppOfs  = 0 // byte offset of AppID
	kNodeOfs = 4 // byte offset of NodeID
	kAttrOfs = 8 // byte offset of AttrID
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
	ID      arc.CellID
	pl      *plSess            // parent planet
	ctx     task.Context       // arc.PinnedCell ctx
	newTxns chan *arc.MsgBatch // txns to be pushed to subs
	dbKey   [kAttrOfs]byte
	// newSubs  chan *plReq        // new requests waiting for state
	// subsHead *plReq             // single linked list of open reqs on this cell
	// subsMu   sync.Mutex         // mutex for subs
	// newReqs  chan *reqContext      // new requests waiting for state
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
		ID:      cellID,
		newTxns: make(chan *arc.MsgBatch),
	}

	binary.BigEndian.PutUint32(cell.dbKey[0:], scopeID)
	binary.BigEndian.PutUint32(cell.dbKey[4:], nodeID)

	// Start the cell context as a child of the planet db it belongs to
	cell.ctx, err = pl.Context.StartChild(&task.Task{
		Label:   fmt.Sprintf("Cell %d", nodeID),
		TaskRef: cellID,
		OnRun: func(ctx task.Context) {

			// Manage incoming subs, push state to subs, and then maintain state for each sub
			for running := true; running; {
				select {
				case tx := <-cell.newTxns:
					cell.pushToSubs(tx) // TODO: see comments in pushToSubs()

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

func (ctx *appContext) startCell(req *reqContext) error {
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

func (ctx *appContext) bindAndStart(req *reqContext) (cell *cellInst, err error) {
	ctx.cellsMu.Lock()
	defer ctx.cellsMu.Unlock()

	ID := req.pinned.ID()

	// If the cell is already open, we're done
	cell = ctx.cells[ID]
	if cell != nil {
		return
	}

	cell = &cellInst{
		CellID:  ID,
		newReqs: make(chan *reqContext),
		newTxns: make(chan *arc.MsgBatch),
	}

	cell.Context, err = ctx.Context.StartChild(&task.Task{
		//Label: req.pinned.Context.Label(),
		Label: fmt.Sprintf("CellID %d", ID),
		OnRun: func(ctx task.Context) {

			for running := true; running; {

				// Manage incoming subs, push state to subs, and then maintain state for each sub.
				select {
				case req := <-cell.newReqs:
					var err error
					{
						req.PushBeginPin(cell.CellID)

						// TODO: verify that a cell pushing state doesn't escape idle or close analysis
						err = req.pinned.PushCellState(&req.CellReq, arc.PushAsParent)
					}
					req.PushCheckpoint(err)

				case tx := <-cell.newTxns:
					cell.pushToSubs(tx)

				case <-cell.Context.Closing():
					running = false
				}

			}

		},
	})



func (sess *hostSess) pushState(req *reqContext) error {

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


func (pl *plSess) addSub(req arc.CellReq) error {


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

func (cell *plCell) PinCell(req arc.CellReq) (arc.PinnedCell, error) {
	// Hmmm, what does it even mean for this or child cells to be pinned by a client?
	// Is the idea that child cells are either implicit links to cells in the same planet or explicit links to cells in other planets?
	return nil, arc.ErrCode_Unimplemented.Error("TODO: implement cell pinning")
}

func (cell *plCell) Context() task.Context {
	return cell.ctx
}

func (cell *plCell) PushState(ctx arc.PinContext) error {
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
	usingNative := ctx.UsingNativeSymbols()

	// Read the Cell from the db and push enabled (current all) attrs it to the client
	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()

		attrNativeID := binary.BigEndian.Uint32(item.Key()[kAttrOfs:])
		val, err := sess.NewAttrElem(attrNativeID, usingNative)
		if err != nil {
			cell.ctx.Warnf("failed to create attr for native ID %d: %v", attrNativeID, err)
			continue
		}
		err = item.Value(func(valBuf []byte) error {
			return val.Unmarshal(valBuf)
		})
		if err != nil {
			cell.ctx.Warnf("failed to unmarshal attr %v: %v", reflect.ValueOf(val).Elem().Type().Name(), err)
			continue
		}

		//
		// TODO -- read SI
		//

		SI_raw := uint64(0)
		attrElem := arc.AttrElem{
			AttrID: attrNativeID,
			Val:    val,
			SI:     decodeSI(SI_raw),
		}

		push := true // TODO: select which attrs to push based on client's request

		// Don't send the attr if the client never registered it
		if !usingNative {
			attrElem.AttrID, push = sess.NativeToClientID(attrElem.AttrID)
		}

		if push {
			msg, err := attrElem.MarshalToMsg(cell.ID)
			if err == nil {
				if !ctx.PushMsg(msg) {
					break
				}
			}
		}
	}

	return nil
}

func (cell *plCell) MergeTx(tx arc.CellTx) error {

	// TODO: handle SIs
	dbTx := cell.pl.db.NewTransaction(true)
	defer dbTx.Discard()

	var keyBuf [32]byte
	cellKey := append(keyBuf[:0], cell.dbKey[:]...)

	for _, attr := range tx.Attrs {
		key := binary.BigEndian.AppendUint32(cellKey, attr.AttrID)

		var valBuf []byte
		err := attr.Val.MarshalToBuf(&valBuf)
		if err != nil {
			return err
		}
		err = dbTx.Set(key, valBuf)
		if err != nil {
			return err
		}
	}

	err := dbTx.Commit() // TODO: handle conflict / error
	if err != nil {
		return err
	}

	return nil
}

func (cell *plCell) pushToSubs(tx *arc.MsgBatch) {
	var childBuf [8]task.Context
	children := cell.ctx.GetChildren(childBuf[:0])

	// TODO: lock sub while it's being pushed to?
	//   Maybe each sub maintains a queue of Msg channels that push to it?
	//   - when the queue closes, it moves on to the next
	//   - this would ensure a sub doesn't get 2+ msg batches at once
	for _, child := range children {
		if child.PreventIdleClose(time.Second) {
			if pinCtx, ok := child.TaskRef().(arc.PinContext); ok {
				tx.PushCopyToClient(pinCtx)
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
func (pl *planetSess) PushTx(tx *arc.MsgBatch) error {
	cellSymID := pl.symTable.GetSymbolID(tx.Msgs[0].ValBuf, true)

	var buf [512]byte
	key := append(buf[:0], kCellStorage)
	key = cellSymID.WriteTo(key)

	txn := arc.Txn{
		Msgs: tx.Msgs,
	}
	txData, _ := txn.Marshal()
	tx.Reclaim()

	dbTx := pl.db.NewTransaction(true)
	defer dbTx.Discard()

	dbTx.Set(key, txData)
	err := dbTx.Commit()

	return err
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





func (csess *cellInst) pushState(req *nodeReq) error {
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
	err := req.pushMsg(msg)
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
	err = req.pushMsg(msg)
	if err != nil {
		return err
	}

	return nil
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
		//newReqs:  make(chan *reqContext, 1),
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
