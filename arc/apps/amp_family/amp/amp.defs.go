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

type Cell[AppT arc.AppContext] interface {
	arc.Cell

	// Returns a human-readable label for this cell useful for debugging and logging.
	GetLogLabel() string

	MarshalAttrs(app AppT, dst *arc.CellTx) error

	// Called when this cell is pinned from a parent (vs an absolute URL)
	OnPinned(parent Cell[AppT]) error

	PinInto(dst *PinnedCell[AppT]) error
}

type CellBase[AppT arc.AppContext] struct {
	arc.CellInfo
	Self Cell[AppT]
}

type PinnedCell[AppT arc.AppContext] struct {
	*CellBase[AppT]
	App AppT

	cellCtx   task.Context
	children  []*CellBase[AppT]     // ordered list of children
	childByID map[arc.CellID]uint32 // index into []children
}
