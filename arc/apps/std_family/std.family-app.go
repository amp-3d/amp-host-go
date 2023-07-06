package std_family

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/std_family/planet"
	"github.com/arcspace/go-archost/arc/apps/std_family/std_home"
)

func RegisterFamily(reg arc.Registry) {
	arc.RegisterBuiltInTypes(reg)

	std_home.RegisterApp(reg)
	planet.RegisterApp(reg)
}
