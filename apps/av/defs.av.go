package av

import (
	"github.com/git-amp/amp-sdk-go/amp"
	"github.com/git-amp/amp-sdk-go/stdlib/task"
)

const (
	AppFamilyDomain = ".av.arcspace.systems"
)

const (
	ListItemSeparator = " · "
)

var (
	DirGlyph = &amp.AssetTag{
		URI: amp.GlyphURIPrefix + "application/x-directory",
	}
)

type AppBase struct {
	amp.AppBase
}

const (
	MediaPlaylistAttrSpec       = "MediaPlaylist"
	PlayableMediaItemAttrSpec   = "PlayableMediaItem"
	PlayableMediaAssetsAttrSpec = "PlayableMediaAssets:main"
)

type Cell[AppT amp.AppContext] interface {
	amp.Cell

	// Returns a human-readable label for this cell useful for debugging and logging.
	GetLogLabel() string

	MarshalAttrs(dst *amp.CellTx, ctx amp.PinContext) error

	// Called when this cell is pinned from a parent (vs an absolute URL)
	OnPinned(parent Cell[AppT]) error

	PinInto(dst *PinnedCell[AppT]) error
}

type CellBase[AppT amp.AppContext] struct {
	amp.CellID
	Self Cell[AppT]
}

type PinnedCell[AppT amp.AppContext] struct {
	*CellBase[AppT]
	App AppT

	cellCtx   task.Context
	children  []*CellBase[AppT]     // ordered list of children
	childByID map[amp.CellID]uint32 // index into []children
}
