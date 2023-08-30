package bridges

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/apps/bridges/filesys"
	"github.com/arcspace/go-archost/apps/bridges/ipfs"
)

func RegisterFamily(reg arc.Registry) {
	filesys.RegisterApp(reg)
	ipfs.RegisterApp(reg)
}
