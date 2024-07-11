package av

import (
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/std"
)

var (
	AppSpec = amp.AppSpec.With("av")
)

const (
	ListItemSeparator = " · "
)

var (
	MediaTrackInfoID = std.CellProperty.With("av.MediaTrackInfo").ID
)
