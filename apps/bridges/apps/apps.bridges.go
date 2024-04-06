package bridges

import (
	"github.com/amp-space/amp-host-go/apps/bridges/filesys"
	"github.com/amp-space/amp-host-go/apps/bridges/ipfs"
	"github.com/amp-space/amp-sdk-go/amp"
)

func RegisterFamily(reg amp.Registry) {
	filesys.RegisterApp(reg)
	ipfs.RegisterApp(reg)
}
