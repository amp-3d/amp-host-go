package amp

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/task"
)

func (app *AppBase) OnNew(ctx arc.AppContext) (err error) {
	err = app.AppBase.OnNew(ctx)
	if err != nil {
		return
	}

	if app.MediaInfoAttr, err = app.ResolveAppAttr((&MediaInfo{}).AttrSpec()); err != nil {
		return
	}
	if app.MediaPlaylistAttr, err = app.ResolveAppAttr((&MediaPlaylist{}).AttrSpec()); err != nil {
		return
	}
	if app.PlayableAssetAttr, err = app.ResolveAppAttr(PlayableAssetAttrSpec); err != nil {
		return err
	}

	if app.PlayableCellSpec, err = app.ResolveAppCell(PlayableCellSpec); err != nil {
		return err
	}
	if app.PlaylistCellSpec, err = app.ResolveAppCell(PlaylistCellSpec); err != nil {
		return
	}

	return nil
}

func (cell *CellBase[AppT]) ID() (cellID arc.CellID) {
	return cell.CellID
}

func (cell *CellBase[AppT]) GetChildCell(target arc.CellID) Cell[AppT] {
	return nil
}

func (cell *CellBase[AppT]) SpawnAsPinnedCell(app AppT, label string) (arc.PinnedCell, error) {
	if cell.CellID == 0 {
		cell.CellID = app.IssueCellID()
	}

	pinned := &PinnedCell[AppT]{
		Cell: cell,
		App:  app,
	}

	// Like most apps, pinned items are started as direct child contexts of the app context\
	var err error
	pinned.cellCtx, err = app.StartChild(&task.Task{
		Label: label,
	})
	if err != nil {
		return nil, err
	}

	return pinned, nil
}

func (cell *CellBase[AppT]) ExportAttrs(app AppT, dst *arc.AttrBatch) error {
	return nil
}

func (cell *CellBase[AppT]) PinInto(dst *PinnedCell[AppT]) error {
	return arc.ErrNotPinnable
}

func (cell *CellBase[AppT]) WillPinCell(app AppT, parent Cell[AppT], req arc.CellReq) (label string, err error) {
	return req.String(), nil
}

func (parent *PinnedCell[AppT]) NewChild(child *CellBase[AppT], onPin Cell[AppT]) {
	if child.CellID == 0 {
		child.CellID = parent.App.IssueCellID()
	}
	parent.children = append(parent.children, child)
}

func (parent *PinnedCell[AppT]) GetChildCell(target arc.CellID) (cell Cell[AppT]) {
	parentID := parent.ID()
	if target == parentID {
		cell = parent
	} else {
		// build a child lookup map on-demand
		if len(parent.children) > 6 {
			if parent.childByID == nil {
				parent.childByID = make(map[arc.CellID]int32, len(parent.children))
				for i, child := range parent.children {
					childID := child.ID()
					parent.childByID[childID] = int32(i)
				}
			}
			if idx, exists := parent.childByID[target]; exists {
				cell = parent.children[idx]
			}
		} else {
			for _, child := range parent.children {
				childID := child.ID()
				if childID == target {
					cell = child
					break
				}
			}
		}
	}
	return cell
}

func (parent *PinnedCell[AppT]) ResolveCell(req arc.CellReq) (arc.PinnedCell, error) {
	target := req.PinID()

	parentID := parent.ID()
	if target == parentID {
		return parent, nil
	}

	cell := parent.GetChildCell(target)
	if cell == nil {
		return nil, arc.ErrCellNotFound
	}

	// TODO: this breaks if same child child is pinned two different times?
	label, err := cell.WillPinCell(parent.App, parent, req)
	if err != nil {
		return nil, err
	}

	return cell.SpawnAsPinnedCell(parent.App, label)
}

func (parent *PinnedCell[AppT]) Context() task.Context {
	return parent.cellCtx
}

func (parent *PinnedCell[AppT]) PushState(ctx arc.PinContext) error {

	if parent.pinnedAt == 0 {
		parent.pinnedAt = arc.TimeNowFS()
		parent.children = parent.children[:0]
		parent.childByID = nil

		err := parent.Cell.PinInto(parent)
		if err != nil {
			return err
		}
	}

	batch := arc.AttrBatch{}
	parentID := parent.ID()
	batch.Clear(parentID)

	if err := parent.ExportAttrs(parent.App, &batch); err != nil {
		return err
	}
	if err := batch.PushBatch(ctx); err != nil {
		return err
	}

	for _, child := range parent.children {
		batch.Clear(child.ID())
		if err := child.ExportAttrs(parent.App, &batch); err != nil {
			return err
		}
		if err := batch.PushBatch(ctx); err != nil {
			return err
		}
	}

	{
		m := arc.NewMsg()
		m.Op = arc.MsgOp_Checkpoint
		m.CellID = int64(parentID)
		if !ctx.PushMsg(m) {
			return arc.ErrPinCtxClosed
		}
	}

	return nil
}

func (v *LoginInfo) MarshalToBuf(dst *[]byte) error {
	return arc.MarshalPbValueToBuf(v, dst)
}

func (v *LoginInfo) AttrSpec() string {
	return "Login"
}

func (v *LoginInfo) New() arc.ElemVal {
	return &LoginInfo{}
}

func (v *MediaInfo) MarshalToBuf(dst *[]byte) error {
	return arc.MarshalPbValueToBuf(v, dst)
}

func (v *MediaInfo) AttrSpec() string {
	return "MediaInfo"
}

func (v *MediaInfo) New() arc.ElemVal {
	return &MediaInfo{}
}

func (v *MediaPlaylist) MarshalToBuf(dst *[]byte) error {
	return arc.MarshalPbValueToBuf(v, dst)
}

func (v *MediaPlaylist) AttrSpec() string {
	return "MediaPlaylist"
}

func (v *MediaPlaylist) New() arc.ElemVal {
	return &MediaPlaylist{}
}
