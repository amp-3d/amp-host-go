package apps

import (
	"github.com/arcspace/go-archost/apps/av"
	"github.com/arcspace/go-archost/apps/av/bcat"
	"github.com/arcspace/go-archost/apps/av/spotify"
	"github.com/git-amp/amp-sdk-go/amp"
)

func RegisterFamily(reg amp.Registry) {
	reg.RegisterElemType(&av.PlayableMediaItem{})
	reg.RegisterElemType(&av.PlayableMediaAssets{})
	reg.RegisterElemType(&av.MediaPlaylist{})

	bcat.RegisterApp(reg)
	spotify.RegisterApp(reg)
}
