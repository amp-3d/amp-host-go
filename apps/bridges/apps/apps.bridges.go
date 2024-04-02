package bridges

import (
	"github.com/arcspace/go-archost/apps/bridges/filesys"
	"github.com/arcspace/go-archost/apps/bridges/ipfs"
	"github.com/git-amp/amp-sdk-go/amp"
)

func RegisterFamily(reg amp.Registry) {
	filesys.RegisterApp(reg)
	ipfs.RegisterApp(reg)
}
