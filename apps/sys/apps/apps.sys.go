package reg

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/apps/sys/planet"
	"github.com/arcspace/go-archost/apps/sys/ux"
)

func RegisterFamily(reg arc.Registry) {
	planet.RegisterApp(reg)
	ux.RegisterApp(reg)
}
