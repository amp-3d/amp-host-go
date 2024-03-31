package apps

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/apps/av"
	"github.com/arcspace/go-archost/apps/av/bcat"
	"github.com/arcspace/go-archost/apps/av/spotify"
)

func RegisterFamily(reg arc.Registry) {
	reg.RegisterElemType(&av.PlayableMediaItem{})
	reg.RegisterElemType(&av.PlayableMediaAssets{})
	reg.RegisterElemType(&av.MediaPlaylist{})

	bcat.RegisterApp(reg)
	spotify.RegisterApp(reg)
}
