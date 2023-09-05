package amp

import (
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/task"
)

func (app *AppBase) OnNew(ctx arc.AppContext) (err error) {
	err = app.AppBase.OnNew(ctx)
	if err != nil {
		return
	}
	return nil
}

func NewPinnedCell[AppT arc.AppContext](app AppT, cell *CellBase[AppT]) (arc.PinnedCell, error) {
	if cell.CellID.IsNil() {
		cell.CellID = app.IssueCellID()
	}
	pinned := &PinnedCell[AppT]{
		CellBase: cell,
		App:      app,
	}

	err := cell.Self.PinInto(pinned)
	if err != nil {
		return nil, err
	}

	// Like most apps, pinned items are started as direct child contexts of the app context\
	pinned.cellCtx, err = app.StartChild(&task.Task{
		Label:     "cell: " + cell.Self.GetLogLabel(),
		IdleClose: time.Nanosecond,
	})
	if err != nil {
		return nil, err
	}

	return pinned, nil
}

func (cell *CellBase[AppT]) AddTo(dst *PinnedCell[AppT], self Cell[AppT]) {
	cell.CellID = dst.App.IssueCellID()
	cell.Self = self
	dst.AddChild(cell)
}

func (parent *PinnedCell[AppT]) GetCell(target arc.CellID) *CellBase[AppT] {
	parentID := parent.CellID
	if target == parentID {
		return parent.CellBase
	} else {
		return parent.GetChildCell(target)
	}
}

func (cell *CellBase[AppT]) Info() arc.CellID {
	return cell.CellID
}

func (cell *CellBase[AppT]) OnPinned(parent Cell[AppT]) error {
	return nil
}

func (parent *PinnedCell[AppT]) AddChild(child *CellBase[AppT]) {
	parent.children = append(parent.children, child)
}

func (parent *PinnedCell[AppT]) MergeUpdate(tx *arc.Msg) error {
	return arc.ErrUnimplemented
}

func (parent *PinnedCell[AppT]) GetChildCell(target arc.CellID) (cell *CellBase[AppT]) {
	if target == parent.CellID {
		cell = parent.CellBase
	} else {
		// build a child lookup map on-demand
		if len(parent.children) > 6 {
			if parent.childByID == nil {
				parent.childByID = make(map[arc.CellID]uint32, len(parent.children))
				for i, child := range parent.children {
					childID := child.CellID
					parent.childByID[childID] = uint32(i)
				}
			}
			if idx, exists := parent.childByID[target]; exists {
				cell = parent.children[idx]
			}
		} else {
			for _, child := range parent.children {
				childID := child.CellID
				if childID == target {
					cell = child
					break
				}
			}
		}
	}
	return cell
}

func (parent *PinnedCell[AppT]) PinCell(req arc.PinReq) (arc.PinnedCell, error) {
	reqParams := req.Params()
	if reqParams.PinCell == parent.CellID {
		return parent, nil
	}

	child := parent.GetChildCell(reqParams.PinCell)
	if child == nil {
		return nil, arc.ErrCellNotFound
	}

	if err := child.Self.OnPinned(parent.Self); err != nil {
		return nil, err
	}

	return NewPinnedCell[AppT](parent.App, child)
}

func (parent *PinnedCell[AppT]) Context() task.Context {
	return parent.cellCtx
}

func (parent *PinnedCell[AppT]) ServeState(ctx arc.PinContext) error {

	marshalToTx := func(dst **arc.CellTxPb, target *CellBase[AppT]) error {
		var tx arc.CellTx
		tx.Clear(arc.CellTxOp_UpsertCell)
		tx.TargetCell = target.CellID
		if tx.TargetCell.IsNil() {
			return arc.ErrBadCellTx
		}
		err := target.Self.MarshalAttrs(&tx, ctx)
		if err != nil {
			return err
		}
		pb := &arc.CellTxPb{
			Op:    tx.Op,
			Elems: tx.ElemsPb,
		}
		pb.CellID_0, pb.CellID_1 = tx.TargetCell.ExportAsU64()
		tx.ElemsPb = nil
		*dst = pb
		return err
	}

	txs := make([]*arc.CellTxPb, 1+len(parent.children))

	if err := marshalToTx(&txs[0], parent.CellBase); err != nil {
		return err
	}
	for ci, child := range parent.children {
		if err := marshalToTx(&txs[1+ci], child); err != nil {
			return err
		}
	}

	msg := arc.NewMsg()
	msg.CellTxs = txs
	msg.Status = arc.ReqStatus_Synced
	return ctx.PushUpdate(msg)
}

func (v *LoginInfo) MarshalToBuf(dst *[]byte) error {
	return arc.MarshalPbValueToBuf(v, dst)
}

func (v *LoginInfo) TypeName() string {
	return "LoginInfo"
}

func (v *LoginInfo) New() arc.ElemVal {
	return &LoginInfo{}
}

func (v *MediaInfo) MarshalToBuf(dst *[]byte) error {
	return arc.MarshalPbValueToBuf(v, dst)
}

func (v *MediaInfo) TypeName() string {
	return "MediaInfo"
}

func (v *MediaInfo) New() arc.ElemVal {
	return &MediaInfo{}
}

func (v *MediaPlaylist) MarshalToBuf(dst *[]byte) error {
	return arc.MarshalPbValueToBuf(v, dst)
}

func (v *MediaPlaylist) TypeName() string {
	return "MediaPlaylist"
}

func (v *MediaPlaylist) New() arc.ElemVal {
	return &MediaPlaylist{}
}
