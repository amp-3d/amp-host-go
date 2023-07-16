package amp

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/task"
)

const (
	AppFamilyDomain = ".amp.arcspace.systems"
)

// MimeType (aka MediaType)
const (
	MimeType_Dir      = "application/x-directory"
	MimeType_Album    = "application/x-album"
	MimeType_Playlist = "application/x-playlist"
)

var (
	DirGlyph = &arc.AssetRef{
		MediaType: MimeType_Dir,
	}
)

type AppBase struct {
	arc.AppBase

	// cell types
	PlayableCellSpec uint32
	PlaylistCellSpec uint32

	// attr specs
	MediaInfoAttr     uint32
	MediaPlaylistAttr uint32
	PlayableAssetAttr uint32
}

func (parent *PinnedCell[AppT]) GetCell(target arc.CellID) Cell[AppT] {
	parentID := parent.ID()
	if target == parentID {
		return parent
	} else {
		return parent.GetChildCell(target)
	}
}

type Cell[AppT arc.AppInstance] interface {
	ExportAttrs(app AppT, dst *arc.AttrBatch) error
	WillPinChild(child Cell[AppT]) error // Called when a cell is pinned as a child
	PinInto(dst *PinnedCell[AppT]) error
	ID() arc.CellID
	Label() string
	//Info() arc.CellInfo
}

type CellBase[AppT arc.AppInstance] struct {
	CellID arc.CellID
	//Impl   Cell[AppT]
}

type PinnedCell[AppT arc.AppInstance] struct {
	Cell[AppT]
	App AppT

	cellCtx   task.Context
	children  []Cell[AppT]         // ordered list of children
	childByID map[arc.CellID]int32 // index into []children
}
