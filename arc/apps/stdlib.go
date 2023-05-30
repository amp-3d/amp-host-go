package apps

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp"
)

func RegisterStdApps(reg arc.Registry) {
	amp.RegisterApp(reg)
	//planet.RegisterApp(reg)
	//home.
}
