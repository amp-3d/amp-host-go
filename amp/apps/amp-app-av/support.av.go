package av

import (
	"time"

	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/stdlib/task"
)

func (app *AppBase) OnNew(ctx amp.AppContext) (err error) {
	err = app.AppBase.OnNew(ctx)
	if err != nil {
		return
	}
	return nil
}

func NewPinnedCell[AppT amp.AppContext](app AppT, cell *CellBase[AppT]) (amp.PinnedCell, error) {
	if cell.TagID.IsNil() {
		cell.TagID = app.IssueTagID()
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
	cell.TagID = dst.App.IssueTagID()
	cell.Self = self
	dst.AddChild(cell)
}

func (cell *CellBase[AppT]) FormAttrUpsert(attrSpecID amp.TagID) amp.TxOp {
	op := amp.TxOp{}
	op.OpCode = amp.TxOpCode_UpsertAttr
	op.TargetID = cell.TagID
	op.AttrID = attrSpecID
	return op
}

func (cell *CellBase[AppT]) Info() amp.TagID {
	return cell.TagID
}

func (cell *CellBase[AppT]) OnPinned(parent Cell[AppT]) error {
	return nil
}

func (parent *PinnedCell[AppT]) AddChild(child *CellBase[AppT]) {
	parent.children = append(parent.children, child)
}

func (parent *PinnedCell[AppT]) GetCell(target amp.TagID) *CellBase[AppT] {
	parentID := parent.TagID
	if target == parentID {
		return parent.CellBase
	} else {
		return parent.GetChildCell(target)
	}
}

func (parent *PinnedCell[AppT]) MergeTx(tx *amp.TxMsg) error {
	return amp.ErrUnimplemented
}

func (parent *PinnedCell[AppT]) GetChildCell(target amp.TagID) (cell *CellBase[AppT]) {
	if target == parent.TagID {
		cell = parent.CellBase
	} else {
		// build a child lookup map on-demand
		if len(parent.children) > 6 {
			if parent.childByID == nil {
				parent.childByID = make(map[amp.TagID]uint32, len(parent.children))
				for i, child := range parent.children {
					childID := child.TagID
					parent.childByID[childID] = uint32(i)
				}
			}
			if idx, exists := parent.childByID[target]; exists {
				cell = parent.children[idx]
			}
		} else {
			for _, child := range parent.children {
				childID := child.TagID
				if childID == target {
					cell = child
					break
				}
			}
		}
	}
	return cell
}

func (parent *PinnedCell[AppT]) PinCell(op amp.PinOp) (amp.PinnedCell, error) {
	req := op.RawRequest()
	targetCell := req.TargetCell()
	if targetCell == parent.TagID {
		return parent, nil
	}

	child := parent.GetChildCell(targetCell)
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
	if parent.TagID.IsNil() {
		panic("parent.TagID is nil")
	}

	tx := amp.NewTxMsg(true)
	if err := parent.Self.MarshalAttrs(tx, ctx); err != nil {
		return err
	}

	for _, child := range parent.children {
		if child.TagID.IsNil() {
			child.TagID = parent.App.IssueTagID()
		}
		if err := child.Self.MarshalAttrs(tx, ctx); err != nil {
			return err
		}
	}

	tx.Status = amp.ReqStatus_Synced
	return ctx.PushTx(tx)
}

func (v *LoginInfo) MarshalToStore(dst []byte) ([]byte, error) {
	return amp.MarshalPbToStore(v, dst)
}

func (v *LoginInfo) ElemTypeName() string {
	return "LoginInfo"
}

func (v *LoginInfo) New() amp.ElemVal {
	return &LoginInfo{}
}

func (v *PlayableMediaItem) MarshalToStore(dst []byte) ([]byte, error) {
	return amp.MarshalPbToStore(v, dst)
}

func (v *PlayableMediaItem) ElemTypeName() string {
	return "PlayableMediaItem"
}

func (v *PlayableMediaItem) New() amp.ElemVal {
	return &PlayableMediaItem{}
}

func (v *MediaPlaylist) MarshalToStore(dst []byte) ([]byte, error) {
	return amp.MarshalPbToStore(v, dst)
}

func (v *MediaPlaylist) ElemTypeName() string {
	return "MediaPlaylist"
}

func (v *MediaPlaylist) New() amp.ElemVal {
	return &MediaPlaylist{}
}
