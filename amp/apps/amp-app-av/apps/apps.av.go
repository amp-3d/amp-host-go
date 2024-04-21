package av_apps

import (
	core_suite "github.com/amp-3d/amp-host-go/amp/app-suites/core/registry"
	av "github.com/amp-3d/amp-host-go/amp/apps/amp-app-av"
	"github.com/amp-3d/amp-host-go/amp/apps/amp-app-av/bcat"
	"github.com/amp-3d/amp-host-go/amp/apps/amp-app-av/spotify"
)

func init() {
	reg := core_suite.Registry()
	
	reg.RegisterPrototype("", &av.PlayableMediaItem{})
	reg.RegisterPrototype("", &av.MediaPlaylist{})

	bcat.RegisterApp(reg)
	spotify.RegisterApp(reg)
}

