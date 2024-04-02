package ux

import (
	"github.com/arcspace/go-archost/apps/sys"
	"github.com/git-amp/amp-sdk-go/amp"
)

const (
	AppID = "ux" + sys.AppFamilyDomain
)

func UID() amp.UID {
	return amp.FormUID(0x5440da3a712f4550, 0x80d755186f311729)
}

func RegisterApp(reg amp.Registry) {
	reg.RegisterApp(&amp.App{
		AppID:   AppID,
		UID:     UID(),
		Desc:    "UI/UX",
		Version: "v1.2023.2",
		NewAppInstance: func() amp.AppInstance {
			return nil
		},
	})
}
