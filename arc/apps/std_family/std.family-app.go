package std_family

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/std_family/std_home"
	"github.com/arcspace/go-archost/arc/apps/std_family/std_planet"
)



func RegisterFamily(reg arc.Registry) {
	std_home.RegisterApp(reg)
	std_planet.RegisterApp(reg)
}
