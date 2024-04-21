package planet

import (
	"encoding/binary"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	sys "github.com/amp-3d/amp-host-go/amp/apps/amp-app-planet"
	"github.com/amp-3d/amp-host-go/amp/badger/symbol_table"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/stdlib/symbol"
	"github.com/amp-3d/amp-sdk-go/stdlib/task"
	"github.com/dgraph-io/badger/v4"
)

const (
	AppID           = "planet" + sys.AppFamilyDomain
	HomePlanetAlias = "~" // hard-wired alias for the the current user's home planet
)

var AppTagID = amp.StringToTagID(AppID)

func RegisterApp(reg amp.Registry) {
	reg.RegisterApp(&amp.App{
		AppID:   AppID,
		TagID:   amp.StringToTagID(AppID),
		Desc:    "cell storage & sync",
		Version: "v1.2024.2",
		NewAppInstance: func() amp.AppInstance {
			return &appCtx{
				plSess: make(map[amp.TagID]*plSess),
			}
		},
	})
}

type appCtx struct {
	amp.AppContext
	home   *plSess               // current user's home planet
	plSess map[amp.TagID]*plSess // mounted planets
	plMu   sync.RWMutex          // plSess mutex

	// AppBase is off-limits for this app b/c it bootstraps the system.
	// the system, in order to boot, requires cell storage to load and store.
	// Or in other words, the genesis bootstrap comes from the genesis planet
}

func (app *appCtx) OnNew(ctx amp.AppContext) error {
	app.AppContext = ctx
	return app.mountHomePlanet()
}

func (app *appCtx) HandleURL(*url.URL) error {
	return amp.ErrUnimplemented
}

// func (app *appCtx) WillPinCell(res amp.CellResolver, req amp.PinReq) error {
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
	if len(login.UserTagID) == 0 {
		return amp.ErrCode_PlanetFailure.Errorf("no user logged in")
	}

	var err error
	app.home, err = app.mountPlanet(amp.NilTag, login.UserTagID)
	if err != nil {
		return err
	}

	return nil
}

var (
	ErrInvalidPlanet = amp.ErrCode_CellNotFound.Error("invalid planet or cell URI")
)

func (app *appCtx) PinCell(parent amp.PinnedCell, req amp.PinOp) (amp.PinnedCell, error) {

	// amp://planet/<planet-alias>/<cell-uri-alias>...
	url := req.URL()
	if url == nil {
		return nil, ErrInvalidPlanet
	}
	aliasLen := strings.IndexByte(url.Path, '/')
	if aliasLen < 0 || aliasLen == len(url.Path)-1 {
		return nil, ErrInvalidPlanet
	}

	planetAlias := url.Path[:aliasLen]

	pl, err := app.getPlanet(planetAlias, true, true)
	if err != nil {
		return nil, err
	}
	//	cellID := amp.StringToTagID(cellAlias)

	cellAlias := url.Path[aliasLen+1:]
	cell, err := pl.getCell(cellAlias, amp.TagID{}, true)
	if err != nil {
		return nil, err
	}

	return cell, nil
}

func (app *appCtx) getPlanet(findAlias string, autoMount bool, createOK bool) (*plSess, error) {

	// Most of the time, the home planet is the target
	if findAlias == HomePlanetAlias {
		return app.home, nil
	}

	if findAlias == "" {
		return nil, amp.ErrCode_PlanetFailure.Errorf("missing planet alias")
	}

	planetTagID := app.home.GetSymbolID(findAlias, createOK)
	if planetTagID.IsNil() {
		return nil, amp.ErrCode_PlanetFailure.Errorf("failed to resolve planet alias '%q'", findAlias)
	}

	app.plMu.RLock()
	pl := app.plSess[planetTagID]
	app.plMu.RUnlock()
	if pl != nil {
		return pl, nil
	} else if !autoMount {
		return nil, nil
	}

	return app.mountPlanet(planetTagID, findAlias)
}

// mountPlanet mounts the given planet by ID, or creates a new one if genesis is non-nil.
func (app *appCtx) mountPlanet(
	dbID amp.TagID, dbName string,
) (*plSess, error) {

	// 	asciiTID := bufs.Base32Encoding.EncodeToString(genesis.EpochTID)
	// 	fsName = utils.MakeFSFriendly(genesis.CommonName, nil) + " " + asciiTID[:6]
	// 	planetID = host.home.GetSymbolID([]byte(fsName), true)

	// 	// Create new planet ID entries that all map to the same ID value
	// 	host.home.SetSymbolID([]byte(asciiTID), planetID)
	// 	host.home.SetSymbolID(genesis.EpochTID, planetID)
	// }

	if dbName == "" {
		dbName = dbID.String()
	}
	if dbID.IsNil() {
		dbID = amp.StringToTagID(dbName)
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
		cells:  make(map[amp.TagID]*plCell),
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

	pl.Context, err = app.StartChild(&task.Task{
		// TODO: home planet should not close -- solution:
		// IdleClose: 120 * time.Second,
		Label: fmt.Sprint("planet: ", dbName),
		OnChildClosing: func(child task.Context) {
			pl.cellsMu.Lock()
			delete(pl.cells, child.TaskRef().(amp.TagID))
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

	app.plSess[pl.dbID] = pl
	return pl, nil
}

func (app *appCtx) onPlanetClosed(pl *plSess) {
	app.plMu.Lock()
	delete(app.plSess, pl.dbID)
	app.plMu.Unlock()
}

/*
	 Db key format:

		OUTDATED -- but to articulate general operation, things are getting Biblical[T]()
		TagID
			0x01 (cell attrs)
				TagSpecID
					0x00        => {ElemVal} (SI == 0)
					SeriesIndex => {ElemVal}
					...
				....
			0x02 (child cells)
				TagID
				...
*/
const (
	kCellOfs    = 0              // byte offset of TagID (TagID) -- TODO TODO TODO 3 x 2
	kAttrOfs    = kCellOfs + 16  // byte offset of TagSpecID (TagID)
	kIndexOfs   = kAttrOfs + 16  // byte offset of SeriesIndex (TagID)
	kAttrKeyLen = kIndexOfs + 16 // total length of key
)

// plSess represents a "mounted" planet (a Cell database), allowing it to be accessed, served, and updated.
type plSess struct {
	task.Context

	app      *appCtx
	symTable symbol.Table
	dbID     amp.TagID
	dbName   string
	db       *badger.DB
	cells    map[amp.TagID]*plCell // working set of mounted cells (pinned or idle)
	cellsMu  sync.RWMutex          // cells mutex
}

// plCell is a "mounted" cell serving requests for a specific cell (typically 0-2 open plReq).
// This can be thought of as the controller for one or more active cell pins.
// *** implements amp.PinnedCell ***
type plCell struct {
	amp.TagID
	pl      *plSess         // parent planet
	ctx     task.Context    // amp.PinnedCell ctx
	newTxns chan *amp.TxMsg // txns to merge
	dbKey   [kAttrOfs]byte  // contains cell ID
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
// *** implements amp.PinContext ***
// Instantiated by a client pinning a cell.
// type plReq struct {
// 	client amp.PinContext // given by the runtime via CellResolver.ResolveCell()
// 	parent *plCell        // parent cell controller
// 	next   *plReq         // single linked list of same-cell reqs
// }
/*



    // ValType_AttrSet            = 4; // .ValInt is a AttrSet TagID
    // ValType_NameSet            = 5; // TagID+TagSpecID+NameID     => TxMsg.(Type)          Values only
    // ValType_CellSet            = 6; // TagID+TagSpecID+TagID     => Cell_NID            AttrSet NIDs only
    // ValType_Series             = 8; // TagID+TagSpecID+TSI+FromID => TxMsg.(Type)          Values only
    // ValType_CellRef            = 20; // .FromID and .SI together identify a cell
    // ValType_CellSetID          = 21; // .ValInt is a CellSet ID (used for SetValType_CellSet)


func (pl *plSess) ReadAttr(ctx amp.AppContext, nodeKey, attrKey string, decoder func(valData []byte) error) error {
	appSID := pl.GetSymbolID([]byte(ctx.StateScope()), false)
	nodeSID := pl.GetSymbolID([]byte(nodeKey), false)
	attrSID := pl.GetSymbolID([]byte(attrKey), false)

	var keyBuf [64]byte
	key := symbol.AppendID(symbol.AppendID(symbol.AppendID(keyBuf[:0], appSID), nodeSID), attrSID)

	dbTx := pl.db.NewTransaction(false)
	defer dbTx.Discard()

	item, err := dbTx.Get(key)
	if err == badger.ErrKeyNotFound {
		return amp.ErrCellNotFound
	}

	err = item.Value(decoder)
	if err != nil {
		return amp.ErrCode_CellNotFound.Errorf("failed to read cell: %v", err)
	}

	return nil
}

func (pl *plSess) WriteAttr(ctx amp.AppContext, nodeKey, attrKey string, valData []byte) error {
	appSID := pl.GetSymbolID([]byte(ctx.StateScope()), true)
	nodeSID := pl.GetSymbolID([]byte(nodeKey), true)
	attrSID := pl.GetSymbolID([]byte(attrKey), true)

	var keyBuf [64]byte
	key := symbol.AppendID(symbol.AppendID(symbol.AppendID(keyBuf[:0], appSID), nodeSID), attrSID)

	if appSID == 0 || nodeSID == 0 || attrSID == 0 {
		return amp.ErrCellNotFound
	}

	dbTx := pl.db.NewTransaction(true)
	defer dbTx.Discard()

	dbTx.Set(key, valData)
	err := dbTx.Commit()
	if err != nil {
		return amp.ErrCode_DataFailure.Errorf("failed to write cell: %v", err)
	}

	return nil
}
*/

func (pl *plSess) GetSymbolID(alias string, autoIssue bool) amp.TagID {
	if pl.symTable == nil {
		return amp.StringToTagID(alias)
	} else {
		return amp.TagID{
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

func (pl *plSess) resolveCellAlias(alias string, autoIssue bool) (uid amp.TagID, err error) {

	if alias == "" {
		err = amp.ErrCode_PlanetFailure.Errorf("missing alias")
		return
	}

	symID := uint64(pl.symTable.GetSymbolID([]byte(alias), autoIssue))
	if symID == 0 {
		return amp.TagID{}, amp.ErrCode_PlanetFailure.Errorf("failed to resolve alias %q", alias)
	}

	uid[1] = symID
	return
}

// func (pl *plSess) resolveCellAlias(alias string, autoIssue bool) (amp.TagID, error) {

// }

func (pl *plSess) getCell(cellAlias string, cellID amp.TagID, autoMount bool) (cell *plCell, err error) {
	if cellID.IsNil() {
		if cellAlias == "" {
			return nil, amp.ErrCode_PlanetFailure.Errorf("missing cell identifier")
		}

		cellID, err = pl.resolveCellAlias(cellAlias, autoMount)
		if err != nil {
			return nil, err
		}
	}

	pl.cellsMu.RLock()
	cell = pl.cells[cellID]
	pl.cellsMu.RUnlock()

	if cell != nil || !autoMount {
		return cell, nil
	}

	pl.cellsMu.Lock()
	defer pl.cellsMu.Unlock()

	// If the cell is already open, make sure it doesn't auto-close while we're handling it.
	cell = pl.cells[cellID]
	if cell != nil && cell.ctx.PreventIdleClose(time.Second) {
		return cell, nil
	}

	cell = &plCell{
		pl:      pl,
		newTxns: make(chan *amp.TxMsg),
	}
	//amp.TagID(cell.TagID).AppendTo(cell.dbKey[0:16])

	// Start the cell context as a child of the planet db it belongs to
	cell.ctx, err = pl.Context.StartChild(&task.Task{
		Label:   cell.GetLogLabel(),
		TaskRef: cellID,
		OnRun: func(ctx task.Context) {

			// Manage incoming subs, push state to subs, and then maintain state for each sub
			for running := true; running; {
				select {
				case tx := <-cell.newTxns:
					err := cell.MergeTx(tx)
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
	spec, err := sess.Registry.GetResolvedNodeSpec(mapAs)
	if err != nil {
		return err
	}

	// Go through all the attr for this NodeType and for any series types, queue them for loading.
	var head, prev *cellSub
	for _, attr := range spec.Attrs {
		if attr.SeriesType != amp.SeriesType_0 && attr.AutoPin != amp.AutoPin_0 {
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
			case amp.AutoPin_All_Ascending:
				sub.targetRange.SI_SeekTo = 0
				sub.targetRange.SI_StopAt = uint64(amp.SI_DistantFuture)

			case amp.AutoPin_All_Descending:
				sub.targetRange.SI_SeekTo = uint64(amp.SI_DistantFuture)
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


func (pl *plSess) addSub(req amp.PinReq) error {

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

func (cell *plCell) Info() amp.TagID {
	return cell.TagID
}

func (cell *plCell) PinCell(req amp.PinOp) (amp.PinnedCell, error) {
	// Hmmm, what does it even mean for this or child cells to be pinned by a client?
	// Is the idea that child cells are either implicit links to cells in the same planet or explicit links to cells in other planets?
	return nil, amp.ErrCode_Unimplemented.Error("TODO: implement cell pinning")
}

func (cell *plCell) Context() task.Context {
	return cell.ctx
}

func (cell *plCell) GetLogLabel() string {
	return fmt.Sprintf("cell: %0x", cell.TagID)
}

func setAttrElemFromKey(key []byte, elem *amp.TxOp) error {
	if len(key) < kIndexOfs+16 {
		return amp.ErrCode_InternalErr.Error("bad db attr key")
	}
	elem.AttrID[0] = 0
	elem.AttrID[1] = binary.BigEndian.Uint64(key[kAttrOfs+0:])
	elem.AttrID[2] = binary.BigEndian.Uint64(key[kAttrOfs+8:])

	elem.SI[0] = binary.BigEndian.Uint64(key[kIndexOfs+0:])
	elem.SI[1] = binary.BigEndian.Uint64(key[kIndexOfs+8:])

	return nil
}

func (cell *plCell) ServeState(ctx amp.PinContext) error {
	dbTx := cell.pl.db.NewTransaction(false)
	defer dbTx.Discard()

	opts := badger.IteratorOptions{
		PrefetchValues: true,
		PrefetchSize:   100,
		Prefix:         cell.dbKey[:],
	}

	itr := dbTx.NewIterator(opts)
	defer itr.Close()

	//sess := cell.pl.app.Session()
	//params := ctx.Params()

	tx := amp.NewTxMsg(true)
	//
	//
	// Read the Cell from the db and push enabled (current all) attrs it to the client
	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()

		// TODO: read child vs parent cell attrs

		send := true
		op := amp.TxOp{}
		op.OpCode = amp.TxOpCode_UpsertAttr
		op.TargetID = cell.TagID
		err := setAttrElemFromKey(item.Key(), &op)
		if err != nil {
			panic(err)
		}

		if !send {
			continue
		}

		// append op data (value) to the tx data store -- no need to unmarshal
		err = item.Value(func(valBuf []byte) error {
			tx.MarshalOpWithBuf(&op, valBuf)
			return nil
		})
		if err != nil {
			panic(err)
			//cell.ctx.Warnf("failed to read attr %d: %v", op.AttrElem.ToString(), err)
			//continue
		}

	}

	tx.Status = amp.ReqStatus_Synced
	return ctx.PushTx(tx)
}

func (cell *plCell) MergeTx(tx *amp.TxMsg) error {
	select {
	case cell.newTxns <- tx:
		return nil // IDEA: maybe we just block until the txn is merged?  this lets us report errors
	case <-cell.ctx.Closing():
		return amp.ErrShuttingDown
	}
}

/*
	func (cell *plSess) MergeTx(tx *amp.TxMsg) error {
		dbTx := cell.pl.db.NewTransaction(true)
		defer dbTx.Discard()

		// Keep a key buf to store keys until we call Commit()
		var keyBuf [512]byte
		key := keyBuf[:0]

		var target amp.TagID
		for _, op := range tx.Ops {

			// Assume: op applies to this cell
			//
			// Is child cells thw wrong way to go?
			// Can't [Name.TagID]ChildTypeA do the same thing?  e.g. [SpotifyID.TagID]PlayableMediaItem
			{
			}

			//getCell("", op.TargetCell, true)

			// TODO: handle child cells
			target.AssignFromU64(cellTx.TargetCell[0], cellTx.TagIDx1)
			if !target.IsNil() && target != cell.TagID {
				cell.ctx.Warnf("unsupported child cell merge %d", target)
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
				key = binary.BigEndian.AppendUint32(cellKey, uint32(elem.TagSpecID))
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
*/
func (cell *plCell) pushToSubs(tx *amp.TxMsg) {
	var childBuf [8]task.Context
	subs := cell.ctx.GetChildren(childBuf[:0])

	// TODO: lock sub while it's being pushed to?
	//   Maybe each sub maintains a queue of TxMsg channels that push to it?
	//   - when the queue closes, it moves on to the next
	//   - this would ensure a sub doesn't get 2+ msg batches at once
	//   - see related comments for appReq
	for _, sub := range subs {
		if sub.PreventIdleClose(time.Second) {
			if subCtx, ok := sub.TaskRef().(amp.PinContext); ok { // TODO -- IS THIS RIGHT?
				if subCtx.Op().RawRequest().Flags&amp.PinFlags_NoSync != 0 {
					continue
				}

				tx.AddRef()
				err := subCtx.PushTx(tx)
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
func (pl *planetSess) getUser(req amp.Login, autoCreate bool) (seat amp.UserSeat, err error) {
	var buf [128]byte

	uid := append(buf[:0], "/TagID/"...)
	uid = append(uid, req.UserTagID...)
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
func (pl *planetSess) ReadCell(cellKey []byte, schema *amp.AttrSchema, msgs func(msg *amp.TxMsg)) error {
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





func (csess *cellInst) ServeState(req *nodeReq) error {
	//target := req.req.TargetNode()
	csess.registerPin(sub)

	dbTx := csess.pl.db.NewTransaction(false)
	defer dbTx.Discard()

	// A node maps to current state where each db itr step corresponds to an attr ID:
	//    cellID+TagSpecID           => TxMsg
	// An attr declared as a series has keys of the form:
	//    cellID+TagSpecID+SI+FromID => TxMsg  (future: SI+FromID is replaced by SI+CollisionSortByte)
	//
	// This means:
	//    - future machinery can be added to perform multi-node locking txns (that update the state node and push to subs)
	//    - TagSpecIDs can be reserved to denote special internal state values -- e.g. revID, NodeType (NodeSpecID)
	//    - idea: edits to values name the SI, FromID, and RevID they are replacing in order to be accepted?



	// 2 possible approaches:
	//    node maps to state: a node ID maps to the "state" node, where each db itr step corresponds to an attr ID
	//       => a time series is its own node type (where each node entry key is SI rather than an attr ID)
	//       => machinery to perform multi-node locking txns would then update the node safely (and push to subs) -- but can be deferred (tech debt)
	//    node embeds all history: node key has TagSpecID+SI+From suffix (non-trivial impl)
	//       => reading node state means seeking the latest SI of each attr
	//       => merging a Tx only pushes state if its SI maps/replaces what is already mapped  (non-trivial impl)
	//       => multi-node locking ops get MESSY

	// Form key for target node to read, seek, and read and send each node entry / attr
	var keyBuf [64]byte
	baseKey := append(keyBuf[:0], csess.keyPrefix[:]...)

	// Announce the new node (send SetTypeID, ItemTypeID, map mode etc)
	msg := amp.NewTxMsg()
	msg.Op = amp.TxMsgOp_AnnounceNode
	msg.SetValue(&req.target)
	err := req.PushTx(msg)
	if err == nil {
		return err
	}


	for _, attr := range req.spec.Attrs {

		if req.isClosed() {
			break
		}

		attrKey := symbol.ID(attr.TagSpecID).WriteTo(baseKey)

		if attr.SeriesType == amp.SeriesType_0 {
			item, getErr := dbTx.Get(attrKey)
			if getErr == nil {
				err = req.unmarshalAndPush(item)
				if err != nil {
					csess.Error(err)
				}
			}
		} else if attr.AutoPin == amp.AutoPin_All {

			switch attr.SeriesType {



			}

		}

		// case amp.

		// case SeriesType_U16:

		// }


		if err != nil {
			return err
		}
		// autoMap := attr.AutoMap
		// if autoMap == amp.AutoMap_ForType {

		// 	switch amp.Type(attr.SetTypeID) {
		// 	case amp.Type_TimeSeries:

		// 	case amp.Type_AttrSet,
		// 		amp.Type_TimeSeries:
		// 		autoMap = amp.AutoMap_No
		// 	case amp.Type_NameSet:
		// 	}
		// }

		switch attr.AutoPin {
		case amp.AutoPin_All:

		}
	}

	// Send break when done
	msg = amp.NewTxMsg()
	msg.Op = amp.TxMsgOp_NodeUpdated
	err = req.PushTx(msg)
	if err != nil {
		return err
	}

	return nil
}




// mountPlanet mounts the given planet by ID, or creates a new one if genesis is non-nil.
func (host *host) mountPlanet(
	planetID uint64,
	genesis *amp.PlanetEpoch,
) (*planetSess, error) {

	if planetID == 0 {
		if genesis == nil {
			return nil, amp.ErrCode_PlanetFailure.Error("missing PlanetID and PlanetEpoch TID")
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
			return nil, amp.ErrCode_PlanetFailure.Errorf("planet ID=%v failed to resolve", planetID)
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
		cells:    make(map[amp.TagID]*cellInst),
		//newReqs:  make(chan *appReq, 1),
	}

	// The db should already exist if opening and vice versa
	_, err := os.Stat(pl.dbPath)
	if genesis != nil && err == nil {
		return nil, amp.ErrCode_PlanetFailure.Error("planet db already exists")
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

func (host *host) HostPlanet() amp.Planet {
	return host.home
}



*/
