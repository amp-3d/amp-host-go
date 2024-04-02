package ipfs

import (
	"github.com/arcspace/go-archost/apps/bridges"
	"github.com/git-amp/amp-sdk-go/amp"
)

const (
	AppID = "ipfs" + bridges.AppFamilyDomain
)

func UID() amp.UID {
	return amp.FormUID(0x90ebeaf9b36e4c3a, 0xa96acc838ce54829)
}

func RegisterApp(reg amp.Registry) {
	reg.RegisterApp(&amp.App{
		AppID:   AppID,
		UID:     UID(),
		Desc:    "bridge for Protocol Lab's IPFS",
		Version: "v1.2023.2",
		NewAppInstance: func() amp.AppInstance {
			return nil //&appCtx{}
		},
	})
}
