package ipfs

import (
	filesys "github.com/amp-3d/amp-host-go/amp/apps/amp-app-filesys"
	"github.com/amp-3d/amp-sdk-go/amp"
)

const (
	AppID = "ipfs" + filesys.AppFamilyDomain
)

func RegisterApp(reg amp.Registry) {
	reg.RegisterApp(&amp.App{
		AppID:   AppID,
		TagID:   amp.StringToTagID(AppID),
		Desc:    "bridge for Protocol Lab's IPFS",
		Version: "v1.2023.2",
		NewAppInstance: func() amp.AppInstance {
			return nil //&appCtx{}
		},
	})
}
