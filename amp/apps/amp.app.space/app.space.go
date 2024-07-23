package space

import (
	"fmt"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amp-3d/amp-host-go/amp/badger/symbol_table"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/registry"
	"github.com/amp-3d/amp-sdk-go/stdlib/symbol"
	"github.com/amp-3d/amp-sdk-go/stdlib/tag"
	"github.com/amp-3d/amp-sdk-go/stdlib/task"
	"github.com/dgraph-io/badger/v4"
)

// EDIT ME:
// A node maps to current state where each db itr step corresponds to an attr ID:
//    cellID+AttrSpecID           => TxMsg
// An attr declared as a series has keys of the form:
//    cellID+AttrSpecID+SI+FromID => TxMsg  (future: SI+FromID is replaced by SI+CollisionSortByte)
//
// This means:
//    - future machinery can be added to perform multi-node locking txns (that update the state node and push to subs)
//    - AttrSpecIDs can be reserved to denote special internal state values -- e.g. revID, NodeType (NodeSpecID)
//    - idea: edits to values name the SI, FromID, and RevID they are replacing in order to be accepted?
//
// 2 possible approaches:
//    node maps to state: a node ID maps to the "state" node, where each db itr step corresponds to an attr ID
//       => a time series is its own node type (where each node entry key is SI rather than an attr ID)
//       => machinery to perform multi-node locking txns would then update the node safely (and push to subs) -- but can be deferred (tech debt)
//    node embeds all history: node key has AttrSpecID+SI+From suffix (non-trivial impl)
//       => reading node state means seeking the latest SI of each attr
//       => merging a Tx only pushes state if its SI maps/replaces what is already mapped  (non-trivial impl)
//       => multi-node locking ops get MESSY
//
// A memorial in the dusty lands of Drew's fumbling path...
//
// ValType_AttrSet            = 4; // .ValInt is a AttrSet Tag
// ValType_NameSet            = 5; // CellID+AttrID+NameID     => TxMsg.(Type)          Values only
// ValType_CellSet            = 6; // CellID+AttrID+Tag      => Cell_NID            AttrSet NIDs only
// ValType_Series             = 8; // CellID+AttrID+SI+FromID => TxMsg.(Type)          Values only
// ValType_CellRef            = 20; // .FromID and .SI together identify a cell
// ValType_CellSetID          = 21; // .ValInt is a CellSet ID (used for SetValType_CellSet)

// Form key for target node to read, seek, and read and send each node entry / attr

var (
	DevicePlanet   = tag.IntsToID(0, 0, 1)
	UserHomePlanet = tag.IntsToID(0, 0, 2)
	AppHomePlanet  = tag.IntsToID(0, 0, 3)
)

var (
	AppSpec = amp.AppSpec.With("space")
)

const (
	HomeAlias = "~" // hard-wired alias for the the current user's home space
)

func init() {
	reg := registry.Global()

	reg.RegisterApp(&amp.App{
		AppSpec: AppSpec,
		Desc:    "cell storage & sync",
		Version: "v1.2024.2",
		NewAppInstance: func(ctx amp.AppContext) (amp.AppInstance, error) {
			app := &appInst{
				plSess:     make(map[tag.ID]*plSess),
				AppContext: ctx,
			}
			return app, nil
		},
	})
}

type appInst struct {
	amp.AppContext
	home   *plSess            // current user's home space
	plSess map[tag.ID]*plSess // mounted spaces
	plMu   sync.RWMutex       // plSess mutex
}

// plSess represents a mounted space (a CellID+AttrID+SI LSM database), allowing it to be accessed, served, and updated.
type plSess struct {
	ctx      task.Context
	app      *appInst
	symTable symbol.Table
	dbID     tag.ID
	dbName   string
	db       *badger.DB
	cells    map[tag.ID]*plCell // working set of mounted cells (pinned or idle)
	cellsMu  sync.RWMutex       // cells mutex
}

// plCell is a mounted cell serving requests for a specific cell (typically 0-2 open plReq).
// This can be thought of as the controller for one or more active cell pins.
type plCell struct {
	pl     *plSess       // parent space
	ctx    task.Context  // cells are parents of their pins
	ID     tag.ID        // cell ID
	pins   []*cellPin    // pins that are maintaining sync with this cell
	pinsMu sync.Mutex    // cells mutex
	rev    atomic.Uint64 // revision counter
}

// *** implements amp.Pin ***
type cellPin struct {
	cell    *plCell         // parent cell
	ctx     task.Context    // amp.Pin ctx
	req     *amp.Request    // client request
	op      amp.Requester   // originator
	changes chan *amp.TxMsg // incoming changesets
	rev     uint64          // revision of the snapshot that was sent
}

func (app *appInst) MakeReady(req amp.Requester) error {
	return nil
}

func (app *appInst) OnClosing() {
	// TODO: close all mounted spaces
}

func (app *appInst) homePlanet() (*plSess, error) {
	if app.home != nil {
		return app.home, nil
	}

	var err error
	login := app.Session().LoginInfo()
	app.home, err = app.mountSpace(login.UserUID.TagID(), login.UserUID.Text)
	if err != nil {
		return nil, err
	}

	return app.home, err
}

func (app *appInst) ServeRequest(op amp.Requester) (amp.Pin, error) {
	req := op.Request()
	url := req.URL
	if url == nil || len(url.Path) < 1 {
		return nil, amp.ErrCode_BadRequest.Errorf("missing URL")
	}

	subPath := url.Path[1:]
	nameLen := strings.IndexByte(subPath, '/') // amp://space/<space-alias>/<cell-alias>
	cellName := ""
	if nameLen < 0 {
		nameLen = len(subPath)
	} else {
		cellName = subPath[nameLen+1:]
	}
	spaceName := subPath[:nameLen]
	pl, err := app.getSpaceSess(spaceName, true, true)
	if err != nil {
		return nil, err
	}

	cellID := req.TargetID()
	if cellID.IsNil() && cellName != "" {
		cellID, err = pl.resolveCellAlias(cellName, true) // TODO: autoIssue == true is a security risk
		if err != nil {
			return nil, err
		}
	}
	if cellID.IsNil() {
		return nil, amp.ErrCode_CellNotFound.Errorf("missing cell identifier")
	}
	return pl.launchPin(cellID, op)
}

func (app *appInst) getSpaceSess(findAlias string, autoMount bool, createOK bool) (*plSess, error) {

	// Most of the time, the home space is the target
	if findAlias == HomeAlias {
		return app.homePlanet()
	}
	if findAlias == "" {
		return nil, amp.ErrCode_PlanetFailure.Errorf("missing space name")
	}
	spaceTag := app.home.GetSymbolID(findAlias, createOK)
	if spaceTag.IsNil() {
		return nil, amp.ErrCode_PlanetFailure.Errorf("failed to resolve space alias '%q'", findAlias)
	}

	app.plMu.RLock()
	pl := app.plSess[spaceTag]
	app.plMu.RUnlock()
	if pl != nil {
		return pl, nil
	} else if !autoMount {
		return nil, nil
	}

	return app.mountSpace(spaceTag, findAlias)
}

// mountSpace mounts the given space by ID, or creates a new one if genesis is non-nil.
func (app *appInst) mountSpace(
	dbID tag.ID, dbName string,
) (*plSess, error) {

	// 	asciiTID := bufs.Base32Encoding.EncodeToString(genesis.EpochTID)
	// 	fsName = utils.MakeFSFriendly(genesis.CommonName, nil) + " " + asciiTID[:6]
	// 	spaceID = host.home.GetSymbolID([]byte(fsName), true)

	// 	// Create new space ID entries that all map to the same ID value
	// 	host.home.SetSymbolID([]byte(asciiTID), spaceID)
	// 	host.home.SetSymbolID(genesis.EpochTID, spaceID)
	// }

	if dbName == "" {
		dbName = dbID.String()
	}
	if dbID.IsNil() {
		dbID = tag.FromString(dbName)
	}

	app.plMu.Lock()
	defer app.plMu.Unlock()

	// Check if already mounted
	pl := app.plSess[dbID]
	if pl != nil {
		return pl, nil
	}

	pl = &plSess{
		app:    app,
		dbID:   dbID,
		dbName: dbName,
		cells:  make(map[tag.ID]*plCell),
	}

	// Limit ValueLogFileSize to ~134mb since badger does a mmap size test on init, causing iOS 13 to error out.
	// Also, massive value file sizes aren't appropriate for mobile.  TODO: make configurable.
	var err error
	dbPath := path.Join(app.LocalDataPath(), pl.dbName)
	dbOpts := badger.DefaultOptions(dbPath)
	dbOpts.Logger = nil
	dbOpts.ValueLogFileSize = 1 << 27
	pl.db, err = badger.Open(dbOpts)
	if err != nil {
		return nil, err
	}

	opts := symbol_table.DefaultOpts()
	opts.Db = pl.db
	opts.IssuerInitsAt = symbol.ID(3773)
	pl.symTable, err = opts.CreateTable()
	if err != nil {
		pl.db.Close()
		pl.db = nil
		return nil, err
	}

	pl.ctx, err = app.StartChild(&task.Task{
		// TODO: home space should not close -- solution:
		// IdleClose: 120 * time.Second,
		Info: task.Info{
			Label: fmt.Sprint("space: ", dbName),
		},
		OnChildClosing: func(child task.Context) {
			pl.cellsMu.Lock()
			delete(pl.cells, child.Info().ContextID) // ContextID is the cell ID
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
	pl.ctx.Log().Info(1, "mounted '", dbPath, "'")

	app.plSess[pl.dbID] = pl
	return pl, nil
}

func (app *appInst) onPlanetClosed(pl *plSess) {
	app.plMu.Lock()
	delete(app.plSess, pl.dbID)
	app.plMu.Unlock()
}

/*
plApp
	plSess (mounted space: "home")
        plCell
    plSess (mounted space)
	    plCell
	    plCell
	...
*/

func (pl *plSess) GetSymbolID(alias string, autoIssue bool) tag.ID {
	if pl.symTable == nil {
		return tag.FromString(alias)
	} else {
		return tag.ID{
			0, uint64(pl.symTable.GetSymbolID([]byte(alias), autoIssue)),
		}
	}
}

func (pl *plSess) GetSymbol(ID uint32, io []byte) []byte {
	return pl.symTable.GetSymbol(symbol.ID(ID), io)
}

func (pl *plSess) SetSymbolID(value []byte, ID uint32) uint32 {
	return uint32(pl.symTable.SetSymbolID(value, symbol.ID(ID)))
}

func (pl *plSess) resolveCellAlias(alias string, autoIssue bool) (uid tag.ID, err error) {
	if alias == "" {
		err = amp.ErrCode_PlanetFailure.Errorf("missing alias")
		return
	}

	symID := uint64(pl.symTable.GetSymbolID([]byte(alias), autoIssue))
	if symID == 0 {
		return tag.ID{}, amp.ErrCode_PlanetFailure.Errorf("failed to resolve alias %q", alias)
	}

	uid[1] = symID
	return
}

func (pl *plSess) getCell(cellID tag.ID, autoMount bool) (cell *plCell, err error) {

	pl.cellsMu.RLock()
	cell = pl.cells[cellID] // TODO: use periodic idle checker / closer like plan-go
	pl.cellsMu.RUnlock()
	if cell != nil || !autoMount {
		return cell, nil
	}

	pl.cellsMu.Lock()
	defer pl.cellsMu.Unlock()

	// If the cell is already open, make sure it doesn't auto-close while we're handling it.
	// TODO: use periodic idle checker / closer like plan-go -- or maybe access bumps a cell rev and unmount needs to witness repeated rev
	cell = pl.cells[cellID]
	if cell != nil && cell.ctx.PreventIdleClose(time.Second) {
		return cell, nil
	}

	cell = &plCell{
		pl:   pl,
		ID:   cellID,
		pins: make([]*cellPin, 0, 4),
	}

	// Start the cell context as a child of the space it belongs to
	cell.ctx, err = pl.ctx.StartChild(&task.Task{
		Info: task.Info{
			Label:     fmt.Sprint("cell: ..", cell.ID.Base32Suffix()),
			ContextID: cellID,
		},
	})
	if err != nil {
		return
	}

	pl.cells[cellID] = cell
	return
}

func (pl *plSess) launchPin(cellID tag.ID, op amp.Requester) (amp.Pin, error) {
	req := op.Request()

	// If no cellID given, get it from the request then fallback to app to handle
	if cellID.IsNil() {
		if req.PinTarget != nil {
			cellID = req.PinTarget.TagID()
		}
		if cellID.IsNil() {
			return pl.app.ServeRequest(op)
		}
	}

	cell, err := pl.getCell(cellID, true)
	if err != nil {
		return nil, err
	}

	pin := &cellPin{
		cell: cell,
		req:  op.Request(),
		op:   op,
	}

	if pin.req.StateSync == amp.StateSync_Maintain {
		pin.changes = make(chan *amp.TxMsg, 4)
	}

	pin.ctx, err = cell.ctx.StartChild(&task.Task{
		Info: task.Info{
			IdleClose: time.Second,
			Label:     fmt.Sprint("pin: ", pin.req.String()),
			ContextID: pin.req.ID,
		},
		OnChildClosing: func(task.Context) {
			pin.cell.removeSync(pin)
		},
		OnRun: func(task.Context) {
			exeErr := pin.execute()
			pin.op.OnComplete(exeErr)
		},
	})
	if err != nil {
		return nil, err
	}

	return pin, nil
}

func (cell *plCell) addSync(pin *cellPin) {
	cell.pinsMu.Lock()
	cell.pins = append(cell.pins, pin)
	cell.pinsMu.Unlock()
}

func (cell *plCell) removeSync(remove *cellPin) {
	cell.pinsMu.Lock()
	{
		N := len(cell.pins) - 1
		for i, pin := range cell.pins {
			if pin == remove {
				cell.pins[i] = cell.pins[N]
				cell.pins = cell.pins[:N]
				break
			}
		}
	}
	cell.pinsMu.Unlock()
}

func (cell *plCell) commitTx(tx *amp.TxMsg) error {
	if tx == nil || len(tx.Ops) == 0 {
		return nil
	}
	opKeys := make([]TxOpKey, len(tx.Ops))

	// Note: we could surround this in a mutex buy is pointless since we handle conflicts below
	for {
		dbTx := cell.pl.db.NewTransaction(true)
		defer dbTx.Discard()

		for i, op := range tx.Ops {
			key := &opKeys[i]
			op.CellID.Put24(key[kCellOfs:])
			op.AttrID.Put16(key[kAttrOfs:])
			op.SI.Put16(key[kIndexOfs:])
			op.EditID.Put16(key[kEditOfs:])
			opVal := tx.DataStore[op.DataOfs : op.DataOfs+op.DataLen]
			err := dbTx.Set(key[:], opVal)
			if err != nil {
				return err
			}
		}

		err := dbTx.Commit()
		if err == badger.ErrConflict {
			continue
		}
		if err != nil {
			return err
		}
		break
	}

	cell.rev.Add(1)

	// TODO: this needs to be way more detailed -- each TxOp:
	//   - should only propagate if is has not already been superseded (it should be the highest for that AttrID+SI)
	//   - ** badger's stream.Orchestrate() can be used to efficiently check all this! **
	//
	//   IDEA: instead of height + tx hash suffix, perhaps a 16 byte "edit" ID?
	//      - composed of the new entry's 8 byte time ID with a sum or hash of the previous entry and the new entry (8 + 8)
	//      - this provides time ordering and a way to validate that the prev entry was witnessed.
	//      - this effectively validates the previous entry witnessed and allows the tx ID to be reconstructed for each entry
	//            Problem: a missing entry?
	//      - this generally favors the newest entry as the latest and where late-arriving entries
	//
	//
	// TODO -- instead of TxOp.Height & Hash, it is better to use an 8 byte UTC16 and an 8 byte hash of the previous entry
	//   This provides canonic ordering and also reasonably witnesses (i.e. links to) the previous entry.
	//
	cell.pushToPins(tx)
	return nil
}

func (cell *plCell) pushToPins(tx *amp.TxMsg) {

	cell.pinsMu.Lock()
	N := len(cell.pins)
	pins := make([]*cellPin, N)
	copy(pins, cell.pins)
	cell.pinsMu.Unlock()

	// fast pass
	for i, pin := range pins {
		select {
		case pin.changes <- tx:
			N--
			pins[i] = nil
		default:
		}
	}

	if N > 0 {
		for _, pin := range pins {
			if pin != nil {
				select {
				case pin.changes <- tx:
				case <-pin.ctx.Closing():
				}
			}
		}
	}
	/*
	   var childBuf [8]task.Context
	   subs := cell.ctx.GetChildren(childBuf[:0])

	   // TODO: lock sub while it's being pushed to?
	   //   Maybe each sub maintains a queue of TxMsg channels that push to it?
	   //   - when the queue closes, it moves on to the next
	   //   - this would ensure a sub doesn't get 2+ msg batches at once
	   //   - see related comments for appReq

	   	for _, sub := range subs {
	   		if sub.PreventIdleClose(time.Second) {
	   			if sub, ok := sub.TaskRef().(amp.Requester); ok { // TODO -- IS THIS RIGHT?
	   				//if sub.Pin
	   				if subCtx.Op().OpRequest().Flags&amp.PinFlags_NoSync != 0 {
	   					continue
	   				}

	   				tx.AddRef()
	   				err := sub.PushTx(tx)
	   				if err != nil {
	   					sub.Warn("failed to push update: ", err)
	   				}
	   			}
	   		}
	   	}
	*/
}

func (pin *cellPin) Context() task.Context {
	return pin.ctx
}

func (pin *cellPin) ServeRequest(op amp.Requester) (amp.Pin, error) {
	return pin.cell.pl.launchPin(tag.Nil, op)
}

func (pin *cellPin) execute() error {

	// if received a commit, push it to the cell
	err := pin.cell.commitTx(pin.req.CommitTx)
	if err != nil {
		pin.ctx.Log().Errorf("failed to commit tx: %v", err)
		return err
	}

	switch pin.req.StateSync {

	case amp.StateSync_Maintain:
		pin.cell.addSync(pin)
		err = pin.pushSnapshot()
		for running := true; running && err == nil; {
			select {
			case <-pin.ctx.Closing():
				running = false
			case tx := <-pin.changes:
				tx.AddRef()
				err = pin.op.PushTx(tx) // TODO: push only requested attrs
			}
		}

	case amp.StateSync_CloseOnSync:
		err = pin.pushSnapshot()

	}

	return err
}

func (pin *cellPin) pushSnapshot() error {
	pin.rev = pin.cell.rev.Load()

	dbTx := pin.cell.pl.db.NewTransaction(false)
	defer dbTx.Discard()

	// FIX ME
	cellID := pin.cell.ID
	cellPrefix := cellID.ToLSM()

	opts := badger.IteratorOptions{
		PrefetchValues: true,
		PrefetchSize:   100,
		Prefix:         cellPrefix[:],
	}

	itr := dbTx.NewIterator(opts)
	defer itr.Close()

	tx := amp.NewTxMsg(true)
	// tx.Upsert(cellID, amp.RootTabsSpec.ID, tag.Nil, nil) -- not needed since already contained in Cell

	//
	// TODO: implement PinRequest.PinAttrs to push only requested attrs
	//

	// Read the Cell from the db and push desired attrs it to the requester
	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()
		key := item.Key()

		op := amp.TxOp{
			OpCode: amp.TxOpCode_UpsertElement,
			CellID: tag.From24(key[kCellOfs:]), // TODO: merge cell --> planet
			AttrID: tag.From16(key[kAttrOfs:]),
			SI:     tag.From24(key[kIndexOfs:]),
			EditID: tag.From16(key[kEditOfs:]),
		}

		// append op data (serialized tag.Value) to the tx data store -- no need to unmarshal
		err := item.Value(func(valBuf []byte) error {
			tx.MarshalOpWithBuf(&op, valBuf)
			return nil
		})
		if err != nil {
			return err
		}
	}

	tx.Status = amp.OpStatus_Synced
	if err := pin.op.PushTx(tx); err != nil {
		return err
	}
	return nil
}

type TxOpKey [AttrKeyLen]byte

const (
	kCellOfs   = 0                 // 24 byte cell index
	kAttrOfs   = 24                // 16 byte attr ID
	kIndexOfs  = 24 + 16           // 24 byte series index
	kEditOfs   = 24 + 16 + 24      // 16 byte revision ID
	AttrKeyLen = 24 + 16 + 24 + 16 // total byte length of key

)

/*
func (op *TxOp) LsmKey() TxOpKey {
	var key TxOpKey
	Put24(key[kCellOfs:], op.TargetID)
	Put16(key[kAttrOfs:], op.AttrID)
	Put16(key[kIndexOfs:], op.SI)

	// reverse the height to make higher entries before lower entries -- TODO: use zig zag to prevent db full of FFFFFF..
	// binary.BigEndian.PutUint64(key[kHeightOfs:], ^op.Height)
	// binary.BigEndian.PutUint64(key[kHashOfs:], op.Hash)
	return key
}
*/

/*

// WIP -- to be replaced by generic cell storage
const (
	kUserTable   = byte(0xF1)
	kCellStorage = byte(0xF2)
)

// This will be replaced in the future with generic use of GetCell() with a "user" App type.
// For now, just make a table with user IDs their respective user record.
func (pl *spaceSess) getUser(req amp.Login, autoCreate bool) (seat amp.UserSeat, err error) {
	var buf [128]byte

	uid := append(buf[:0], "/Tag/"...)
	uid = append(uid, req.UserTag...)
	userID := pl.symTable.GetSymbolID(uid, autoCreate)
	if userID == 0 {
		return amp.UserSeat{}, amp.ErrCode_LoginFailed.Error("unknown user")
	}

	dbTx := pl.db.NewTransaction(true)
	defer dbTx.Discard()

	// For now, just make a table with user IDs their respective user record.
	key := append(buf[:0], kUserTable) // User record table
	key = userID.WriteTo(key)
	item, err := dbTx.Get(key)
	if err == badger.ErrKeyNotFound && autoCreate {
		seat.UserID = uint64(userID)
		seat.HomePlanetID = pl.spaceID /// TODO: do user space genesis here!
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
func (pl *spaceSess) ReadCell(cellKey []byte, schema *amp.AttrSchema, msgs func(msg *amp.TxMsg)) error {
	cellSymID := pl.symTable.GetSymbolID(cellKey, false)
	if cellSymID == 0 {
		return amp.ErrCode_CellNotFound.Error("cell not found")
	}

	var buf [128]byte
	key := append(buf[:0], kCellStorage)
	key = cellSymID.WriteTo(key)

	dbTx := pl.db.NewTransaction(false)
	defer dbTx.Discard()
	item, err := dbTx.Get(key)
	if err == badger.ErrKeyNotFound {
		return amp.ErrCode_CellNotFound.Error("cell not found")
	}

	var txn amp.Txn
	err = item.Value(func(val []byte) error {
		return txn.Unmarshal(val)
	})

	if err != nil {
		return amp.ErrCode_CellNotFound.Errorf("failed to read cell: %v", err)
	}

	for _, msg := range txn.Msgs {
		msgs(msg)
	}

	return err
}




// We can't use a slick linked list thing since amp.Host makes requests children of the parent task.Context

func (ctx *appContext) startCell(req *appReq) error {
	if req.cell != nil {
		panic("already has sub")
	}

	cell, err := ctx.bindAndStart(req)
	if err != nil {
		return err
	}
	req.cell = cell

	// Add incoming req to the cell IDs list of cells
	cell.cellsMu.Lock()
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
	cell.cellsMu.Unlock()

	return nil
}


func (pl *plSess) attach(req amp.PinReq) error {

	// Add incoming req to the cell IDs list of cells
	cell.cellsMu.Lock()
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
	cell.cellsMu.Unlock()

	return nil
}

*/
