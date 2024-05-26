package av

import (
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/registry"
	"github.com/amp-3d/amp-sdk-go/stdlib/tag"
)

var (
	AppSpec  = tag.FormSpec(amp.AppSpec, "av")
	AttrSpec = tag.FormSpec(amp.AttrSpec, "av")
	//MediaPlayablesSpec = tag.FormSpec(AttrSpec, "playlist.PlayableMedia")
	//PinPlayableMedia   = amp.FormPinnableTag(MediaPlayablesSpec)
)

const (
	ListItemSeparator = " · "
)

var (
	MediaPlaylistSpec tag.Spec
	PlayableMediaSpec tag.Spec
)

func init() {
	reg := registry.Global()

	MediaPlaylistSpec = reg.RegisterPrototype(AttrSpec, &PlayableMedia{}, "")
	PlayableMediaSpec = reg.RegisterPrototype(AttrSpec, &MediaPlaylist{}, "")
}
