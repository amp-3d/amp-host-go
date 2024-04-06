package apps

import (
	"github.com/amp-space/amp-host-go/apps/av"
	"github.com/amp-space/amp-host-go/apps/av/bcat"
	"github.com/amp-space/amp-host-go/apps/av/spotify"
	"github.com/amp-space/amp-sdk-go/amp"
)

func RegisterFamily(reg amp.Registry) {
	reg.RegisterElemType(&av.PlayableMediaItem{})
	reg.RegisterElemType(&av.PlayableMediaAssets{})
	reg.RegisterElemType(&av.MediaPlaylist{})

	bcat.RegisterApp(reg)
	spotify.RegisterApp(reg)
}
