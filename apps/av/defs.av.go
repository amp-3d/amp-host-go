package av

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/task"
)

const (
	AppFamilyDomain = ".av.arcspace.systems"
)

const (
	ListItemSeparator = " · "
)

var (
	DirGlyph = &arc.AssetTag{
		URI: arc.GlyphURIPrefix + "application/x-directory",
	}
)

type AppBase struct {
	arc.AppBase
}

const (
	MediaPlaylistAttrSpec       = "MediaPlaylist"
	PlayableMediaItemAttrSpec   = "PlayableMediaItem"
	PlayableMediaAssetsAttrSpec = "PlayableMediaAssets:main"
)

type Cell[AppT arc.AppContext] interface {
	arc.Cell

	// Returns a human-readable label for this cell useful for debugging and logging.
	GetLogLabel() string

	MarshalAttrs(dst *arc.CellTx, ctx arc.PinContext) error

	// Called when this cell is pinned from a parent (vs an absolute URL)
	OnPinned(parent Cell[AppT]) error

	PinInto(dst *PinnedCell[AppT]) error
}

type CellBase[AppT arc.AppContext] struct {
	arc.CellID
	Self Cell[AppT]
}

type PinnedCell[AppT arc.AppContext] struct {
	*CellBase[AppT]
	App AppT

	cellCtx   task.Context
	children  []*CellBase[AppT]     // ordered list of children
	childByID map[arc.CellID]uint32 // index into []children
}
