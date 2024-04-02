package av

import (
	"time"

	"github.com/git-amp/amp-sdk-go/amp"
	"github.com/git-amp/amp-sdk-go/stdlib/task"
)

func (app *AppBase) OnNew(ctx amp.AppContext) (err error) {
	err = app.AppBase.OnNew(ctx)
	if err != nil {
		return
	}
	return nil
}

func NewPinnedCell[AppT amp.AppContext](app AppT, cell *CellBase[AppT]) (amp.PinnedCell, error) {
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

func (parent *PinnedCell[AppT]) GetCell(target amp.CellID) *CellBase[AppT] {
	parentID := parent.CellID
	if target == parentID {
		return parent.CellBase
	} else {
		return parent.GetChildCell(target)
	}
}

func (cell *CellBase[AppT]) Info() amp.CellID {
	return cell.CellID
}

func (cell *CellBase[AppT]) OnPinned(parent Cell[AppT]) error {
	return nil
}

func (parent *PinnedCell[AppT]) AddChild(child *CellBase[AppT]) {
	parent.children = append(parent.children, child)
}

func (parent *PinnedCell[AppT]) MergeUpdate(tx *amp.Msg) error {
	return amp.ErrUnimplemented
}

func (parent *PinnedCell[AppT]) GetChildCell(target amp.CellID) (cell *CellBase[AppT]) {
	if target == parent.CellID {
		cell = parent.CellBase
	} else {
		// build a child lookup map on-demand
		if len(parent.children) > 6 {
			if parent.childByID == nil {
				parent.childByID = make(map[amp.CellID]uint32, len(parent.children))
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

func (parent *PinnedCell[AppT]) PinCell(req amp.PinReq) (amp.PinnedCell, error) {
	reqParams := req.Params()
	if reqParams.PinCell == parent.CellID {
		return parent, nil
	}

	child := parent.GetChildCell(reqParams.PinCell)
	if child == nil {
		return nil, amp.ErrCellNotFound
	}

	if err := child.Self.OnPinned(parent.Self); err != nil {
		return nil, err
	}

	return NewPinnedCell[AppT](parent.App, child)
}

func (parent *PinnedCell[AppT]) Context() task.Context {
	return parent.cellCtx
}

func (parent *PinnedCell[AppT]) ServeState(ctx amp.PinContext) error {

	marshalToTx := func(dst **amp.CellTxPb, target *CellBase[AppT]) error {
		var tx amp.CellTx
		tx.Clear(amp.CellTxOp_UpsertCell)
		tx.TargetCell = target.CellID
		if tx.TargetCell.IsNil() {
			return amp.ErrBadCellTx
		}
		err := target.Self.MarshalAttrs(&tx, ctx)
		if err != nil {
			return err
		}
		pb := &amp.CellTxPb{
			Op:    tx.Op,
			Elems: tx.ElemsPb,
		}
		pb.CellID_0, pb.CellID_1 = tx.TargetCell.ExportAsU64()
		tx.ElemsPb = nil
		*dst = pb
		return err
	}

	txs := make([]*amp.CellTxPb, 1+len(parent.children))

	if err := marshalToTx(&txs[0], parent.CellBase); err != nil {
		return err
	}
	for ci, child := range parent.children {
		if err := marshalToTx(&txs[1+ci], child); err != nil {
			return err
		}
	}

	msg := amp.NewMsg()
	msg.CellTxs = txs
	msg.Status = amp.ReqStatus_Synced
	return ctx.PushUpdate(msg)
}

func (v *LoginInfo) MarshalToBuf(dst *[]byte) error {
	return amp.MarshalPbValueToBuf(v, dst)
}

func (v *LoginInfo) TypeName() string {
	return "LoginInfo"
}

func (v *LoginInfo) New() amp.ElemVal {
	return &LoginInfo{}
}

func (v *PlayableMediaItem) MarshalToBuf(dst *[]byte) error {
	return amp.MarshalPbValueToBuf(v, dst)
}

func (v *PlayableMediaItem) TypeName() string {
	return "PlayableMediaItem"
}

func (v *PlayableMediaItem) New() amp.ElemVal {
	return &PlayableMediaItem{}
}

func (v *PlayableMediaAssets) MarshalToBuf(dst *[]byte) error {
	return amp.MarshalPbValueToBuf(v, dst)
}

func (v *PlayableMediaAssets) TypeName() string {
	return "PlayableMediaAssets"
}

func (v *PlayableMediaAssets) New() amp.ElemVal {
	return &PlayableMediaAssets{}
}

func (v *MediaPlaylist) MarshalToBuf(dst *[]byte) error {
	return amp.MarshalPbValueToBuf(v, dst)
}

func (v *MediaPlaylist) TypeName() string {
	return "MediaPlaylist"
}

func (v *MediaPlaylist) New() amp.ElemVal {
	return &MediaPlaylist{}
}
