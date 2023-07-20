package amp_family

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp_bcat"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp_filesys"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp_spotify"
)

func RegisterFamily(reg arc.Registry) {
	reg.RegisterElemType(&amp.MediaInfo{})
	reg.RegisterElemType(&amp.MediaPlaylist{})

	amp_bcat.RegisterApp(reg)
	amp_filesys.RegisterApp(reg)
	amp_spotify.RegisterApp(reg)
}
