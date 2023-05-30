package archost

import (
	"fmt"
	"sync"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/process"
	"github.com/arcspace/go-arc-sdk/stdlib/symbol"
	"github.com/arcspace/go-archost/arc/badger/symbol_table"
	"github.com/dgraph-io/badger/v4"
)

// cellInst is a "mounted" cell servicing requests for a specific cell (typically one).
// This can be thought of as the controller for one or more active cell pins.
// cellService?  cellSupe?
type cellInst struct {
	arc.CellID
	process.Context // TODO: make custom lightweight later

	pl       *planetSess        // parent planet
	subsHead *openReq           // single linked list of open reqs on this cell
	subsMu   sync.Mutex         // mutex for subs
	newReqs  chan *openReq      // new requests waiting for state
	newTxns  chan *arc.MsgBatch // txns to be pushed to subs
	idleSecs int32              // ticks up as time passes when there are no subs
}

func (pl *planetSess) onStart(opts symbol_table.TableOpts) error {
	var err error

	dbOpts := badger.DefaultOptions(pl.dbPath)
	dbOpts.Logger = nil

	// Limit ValueLogFileSize to ~134mb since badger does a mmap size test on init, causing iOS 13 to error out.
	// Also, massive value file sizes aren't appropriate for mobile.  TODO: make configurable.
	dbOpts.ValueLogFileSize = 1 << 27
	pl.db, err = badger.Open(dbOpts)
	if err != nil {
		return err
	}

	opts.Db = pl.db
	pl.symTable, err = opts.CreateTable()
	if err != nil {
		return err
	}

	return nil
}

func (pl *planetSess) onRun(process.Context) {
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

func (pl *planetSess) onClosed() {
	if pl.symTable != nil {
		pl.symTable.Close()
		pl.symTable = nil
	}
	if pl.db != nil {
		pl.db.Close()
		pl.db = nil
	}
}

func (pl *planetSess) PlanetID() uint64 {
	return pl.planetID
}

func (pl *planetSess) GetSymbolID(value []byte, autoIssue bool) uint64 {
	return uint64(pl.symTable.GetSymbolID(value, autoIssue))
}

func (pl *planetSess) GetSymbol(ID uint64, io []byte) []byte {
	return pl.symTable.GetSymbol(symbol.ID(ID), io)
}

func (pl *planetSess) SetSymbolID(value []byte, ID uint64) uint64 {
	return uint64(pl.symTable.SetSymbolID(value, symbol.ID(ID)))
}

func (pl *planetSess) closeIdleCells(deltaSecs int32) {
	pl.cellsMu.Lock()
	defer pl.cellsMu.Unlock()

	const idleCloseDelay = 3*60 - 1

	// With the cells locked, we can check and close idle cells
	for _, cell := range pl.cells {
		if cell.idleTick(deltaSecs) > idleCloseDelay {
			delete(pl.cells, cell.CellID)
			cell.Close()
		}
	}
}

func (pl *planetSess) getCell(ID arc.CellID) (cell *cellInst, err error) {
	pl.cellsMu.Lock()
	defer pl.cellsMu.Unlock()

	// If the cell is already open, we're done
	cell = pl.cells[ID]
	if cell != nil {
		return
	}

	cell = &cellInst{
		pl:      pl,
		CellID:  ID,
		newReqs: make(chan *openReq),
		newTxns: make(chan *arc.MsgBatch),
	}

	cell.Context, err = pl.Context.StartChild(&process.Task{
		Label: fmt.Sprintf("cell_%d", cell.CellID),
		OnRun: func(ctx process.Context) {

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
	if err != nil {
		return
	}

	pl.cells[ID] = cell

	return
}

func (pl *planetSess) queueReq(cell *cellInst, req *openReq) error {
	if req.cell != nil {
		panic("already has sub")
	}

	var err error
	if cell == nil {
		cell, err = pl.getCell(req.pinned.ID())
		if err != nil {
			return err
		}
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

func (pl *planetSess) cancelSub(req *openReq) {
	cell := req.cell
	if cell == nil /* || req.closed != 0 */ {
		return
	}

	req.cell = nil

	cell.subsMu.Lock()
	{
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

func (cell *cellInst) idleTick(deltaSecs int32) int32 {
	if cell.subsHead != nil {
		return 0
	}
	cell.idleSecs += deltaSecs
	return cell.idleSecs
}

func (cell *cellInst) pushToSubs(tx *arc.MsgBatch) {
	cell.subsMu.Lock()
	defer cell.subsMu.Unlock()

	for sub := cell.subsHead; sub != nil; sub = sub.next {
		err := sub.PushUpdate(tx)
		if err != nil {
			panic(err)
			// sub.Error("dropping client due to error", err)
			// sub.Close()  // TODO: prevent deadlock since chSess.subsMu is locked
			// chSess.subs[i] = nil
		}
	}

}

// WIP -- to be replaced by generic cell storage
const (
	kUserTable   = byte(0xF1)
	kCellStorage = byte(0xF2)
)

// This will be replaced in the future with generic use of GetCell() with a "user" App type.
// For now, just make a table with user IDs their respective user record.
func (pl *planetSess) getUser(req arc.LoginReq, autoCreate bool) (seat arc.UserSeat, err error) {
	var buf [128]byte

	uid := append(buf[:0], "/UID/"...)
	uid = append(uid, req.UserUID...)
	userID := pl.symTable.GetSymbolID(uid, autoCreate)
	if userID == 0 {
		return arc.UserSeat{}, arc.ErrCode_InvalidLogin.Error("unknown user")
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

/*
	PushTx(tx *MsgBatch) error
	ReadCell(cellURI string, schema *AttrSchema, msgs func (msg *Msg)) error

*/

/*
	symbol.ID(cellID).WriteTo(csess.keyPrefix[:])

	// loads immutable info about this cellInst (namely NodeTypeID)
	//  The stored NodeTypeID refers
	{
		dbTx := pl.db.NewTransaction(false)
		defer dbTx.Discard()

		item, err := dbTx.Get(csess.keyPrefix[:])
		if err != nil {
			return nil, arc.ErrCode_CellNotFound.Err()
		}

		err = item.Value(func(val []byte) error {
			return csess.NodeInfo.Unmarshal(val)
		})

		if err != nil {
			return nil, arc.ErrCode_DataFailure.ErrWithMsgf("error starting Node %v", csess.NodeInfo.cellID)
		}

		// csess.NodeInfo.PlanetID = pl.NodeInfo.cellID
		// csess.NodeInfo.cellID = cellID

		// NodeSpec := csess.pl.host.getNodeSpec(csess.NodeInfo.ItemTypeID)
		// if NodeSpec == nil {
		// 	return nil, arc.ErrCode_NodeCorrupted.ErrWithMsgf("NodeSpec %v not found", csess.NodeInfo.ItemTypeID)
		// }

		// csess.NodeSpec = *NodeSpec // TODO: unpack into attr map inst
	}

	// TODO: handle error during StartChild?
	pl.nodes[cellID] = csess

	err := pl.StartChild(csess, fmt.Sprintf("cellInst %4d", cellID))
	if err != nil {
		return nil, err
	}

	return csess, nil
}


// func (pl *planetSess) getNode(nodeTID arc.TID, autoCreate bool) (arc.Node, error) {
// 	cellID := pl.Table.GetSymbolID(nodeTID, false)
// 	if cellID == 9 {
// 		return nil, arc.ErrCode_NodeNotFound.ErrWithMsgf("Node %s", chTID.Base32())
// 	}

// }



// const (
// 	kNodeSpecID_AttrID = symbol.ID(3)
// )

func (csess *cellInst) serveState(req *nodeReq) error {
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

func (sub *cellSub) pinAttrRange(attrID uint64, add arc.AttrRange) error {

	for i, attr := range sub.attrs {
		if attr.def.AttrID != attrID {
			continue
		}

		sub.mu.Lock()
		attr.targetRange.UnionRange(add)
		sub.mu.Unlock()
	}
}


func (sub *cellSub) isClosed() bool {
	return atomic.LoadUint32(&sub.closed) != 0
}

func (sub *cellSub) unmarshalAndPush(item *badger.Item) error {

	//ch.Infof(2, "GET: %s", item.Key())

	msg := arc.NewMsg()

	err := item.Value(func(val []byte) error {
		return msg.Unmarshal(val)
	})

	if err != nil {
		//ch.Errorf("failed to read entry %v: %v", string(msg.Keypath), err)
		return arc.ErrCode_DataFailure.Err()
	}

	return sub.pushMsg(msg)
}

func (sub *cellSub) pushMsg(msg *arc.Msg) error {
	var err error

	//msg.ReqID = req.pinReq.PinID

	//ch.Infof(2, "PushValue: %s", msg.Keypath)

	// If the client backs up, this will back up too which is the desired effect.
	// Otherwise, db reading would quickly fill up the Msg inbox buffer (and have no gain)
	select {
	case sub.sess.msgsOut <- msg:
	// case <-sub.reqCancel:
	// 	err = arc.ErrCode_ShuttingDown.ErrWithMsg("client closing")
	case <-sub.sess.Closing():
		err = arc.ErrCode_ShuttingDown.ErrWithMsg("planet closing")
	}

	return err
}

*/

/*
type cellBase struct {
	arc.CellID

}

type Cell struct {
	req arc.CellReq

	children map[arc.CellID]*cellBase
}

func (cell *Cell) PushCellState(req *arc.CellReq) error {


}
*/
