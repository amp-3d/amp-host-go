package apps

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/apps/amp"
	"github.com/arcspace/go-archost/apps/amp/bcat"
	"github.com/arcspace/go-archost/apps/amp/spotify"
)

func RegisterFamily(reg arc.Registry) {
	reg.RegisterElemType(&amp.MediaInfo{})
	reg.RegisterElemType(&amp.MediaPlaylist{})

	bcat.RegisterApp(reg)
	spotify.RegisterApp(reg)
}
