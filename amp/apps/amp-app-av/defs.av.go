package av

import (
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/stdlib/task"
)

const (
	AppFamilyDomain = "amp-space-archost-av"
)

const (
	ListItemSeparator = " · "
)

var (
	DirGlyph = &amp.AssetTag{
		URL: amp.GenericGlyphURL + "/application/x-directory",
	}
)

type AppBase struct {
	amp.AppBase
}

var (
	MediaPlaylistID     = amp.MustFormAttrSpec("amp.media.playlist.av.uid")
	PlayableMediaItemID = amp.MustFormAttrSpec("amp.media.playable.av.uid")

	// MediaPlaylistTagSpec       = "MediaPlaylist"
	// PlayableMediaItemTagSpec   = "PlayableMediaItem"
	// PlayableMediaAssetsTagSpec = "PlayableMediaAssets:main"
)

type Cell[AppT amp.AppContext] interface {
	amp.Cell

	// Returns a human-readable label for this cell useful for debugging and logging.
	GetLogLabel() string

	MarshalAttrs(dst *amp.TxMsg, ctx amp.PinContext) error

	// Called when this cell is pinned from a parent (vs an absolute URL)
	OnPinned(parent Cell[AppT]) error

	PinInto(dst *PinnedCell[AppT]) error
}

type CellBase[AppT amp.AppContext] struct {
	TagID amp.TagID
	Self  Cell[AppT]
}

type PinnedCell[AppT amp.AppContext] struct {
	*CellBase[AppT]
	App AppT

	cellCtx   task.Context
	children  []*CellBase[AppT]    // ordered list of children
	childByID map[amp.TagID]uint32 // index into []children
}
