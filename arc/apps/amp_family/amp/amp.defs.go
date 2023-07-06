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
	GetChildCell(target arc.CellID) Cell[AppT]

	WillPinCell(app AppT, parent Cell[AppT], req arc.CellReq) (pinLabel string, err error)
	SpawnAsPinnedCell(app AppT, label string) (arc.PinnedCell, error)
	PinInto(dst *PinnedCell[AppT]) error
	ID() arc.CellID
	//Info() arc.CellInfo
}

type CellBase[AppT arc.AppInstance] struct {
	CellID arc.CellID
}

type PinnedCell[AppT arc.AppInstance] struct {
	Cell[AppT]
	App AppT

	cellCtx   task.Context
	pinnedAt  arc.TimeFS           // 0 iff PinInto() has not yet been called
	children  []Cell[AppT]         // ordered list of children
	childByID map[arc.CellID]int32 // index into []children
}
