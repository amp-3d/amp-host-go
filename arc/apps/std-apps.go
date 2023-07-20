package apps

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp_family"
	"github.com/arcspace/go-archost/arc/apps/std_family"
)

func RegisterStdApps(reg arc.Registry) {
	std_family.RegisterFamily(reg)
	amp_family.RegisterFamily(reg)
}
