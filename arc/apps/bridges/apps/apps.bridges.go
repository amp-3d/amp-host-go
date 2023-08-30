package bridges

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/bridges/apps/ipfs"
)

func RegisterFamily(reg arc.Registry) {
	ipfs.RegisterApp(reg)
}
