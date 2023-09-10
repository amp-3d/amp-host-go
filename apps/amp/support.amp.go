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

func (cell *CellBase[AppT]) FormAttrUpsert() arc.CellOp {
	return arc.CellOp{
		OpCode: arc.CellOpCode_UpsertAttr,
		CellID: cell.CellID,
	}
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

func (parent *PinnedCell[AppT]) MergeTx(tx *arc.TxMsg) error {
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
	tx := arc.NewTxMsg()
	
	if parent.CellID.IsNil() {
		panic("parent.CellID")
	}

	if err := parent.Self.MarshalAttrs(tx, ctx); err != nil {
		return err
	}
	
	for _, child := range parent.children {
		if child.CellID.IsNil() {
			child.CellID = parent.App.IssueCellID()
		}
		if err := child.Self.MarshalAttrs(tx, ctx); err != nil {
			return err
		}
	}

	tx.Status = arc.ReqStatus_Synced
	return ctx.PushTx(tx)
}

func (v *LoginInfo) MarshalToStore(dst []byte) ([]byte, error) {
	return arc.MarshalPbToStore(v, dst)
}

func (v *LoginInfo) ElemTypeName() string {
	return "LoginInfo"
}

func (v *LoginInfo) New() arc.AttrElemVal {
	return &LoginInfo{}
}

func (v *MediaInfo) MarshalToStore(dst []byte) ([]byte, error) {
	return arc.MarshalPbToStore(v, dst)
}

func (v *MediaInfo) ElemTypeName() string {
	return "MediaInfo"
}

func (v *MediaInfo) New() arc.AttrElemVal {
	return &MediaInfo{}
}

func (v *MediaPlaylist) MarshalToStore(dst []byte) ([]byte, error) {
	return arc.MarshalPbToStore(v, dst)
}

func (v *MediaPlaylist) ElemTypeName() string {
	return "MediaPlaylist"
}

func (v *MediaPlaylist) New() arc.AttrElemVal {
	return &MediaPlaylist{}
}
