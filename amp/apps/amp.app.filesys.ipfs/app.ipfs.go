package ipfs

import (
	filesys "github.com/amp-3d/amp-host-go/amp/apps/amp.app.filesys"
	"github.com/amp-3d/amp-sdk-go/amp"
)

func RegisterApp(reg amp.Registry) {
	reg.RegisterApp(&amp.App{
		AppSpec: filesys.AppSpec.With("filesys.ipfs"),
		Desc:    "bridge for Protocol Lab's IPFS",
		Version: "v1.2023.2",
		NewAppInstance: func(ctx amp.AppContext) (amp.AppInstance, error) {
			return nil, nil
		},
	})
}
