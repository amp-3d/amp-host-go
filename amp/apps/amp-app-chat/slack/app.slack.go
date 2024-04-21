package app_

import (
	"fmt"

	av "github.com/amp-3d/amp-host-go/amp/apps/amp-app-av"
	bridges "github.com/amp-3d/amp-host-go/amp/apps/amp-app-filesys"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/slack-go/slack"
)

const (
	AppID = bridges.AppFamilyDomain + "slack"
)

func RegisterApp(reg amp.Registry) {
	reg.RegisterApp(&amp.App{
		AppID:   AppID,
		TagID:   amp.StringToTagID(AppID),
		Desc:    "bridge for Slack",
		Version: "v1.0",
		NewAppInstance: func() amp.AppInstance {
			return &appCtx{}
		},
	})
}

type appCtx struct {
	av.AppBase
}

func (app *appCtx) OnNew(ctx amp.AppContext) (err error) {
	err = app.AppBase.OnNew(ctx)
	if err != nil {
		return
	}

	api := slack.New("YOUR_TOKEN_HERE")
	// If you set debugging, it will log all requests to the console
	// Useful when encountering issues
	// slack.New("YOUR_TOKEN_HERE", slack.OptionDebug(true))
	groups, err := api.GetUserGroups(slack.GetUserGroupsOptionIncludeUsers(false))
	if err != nil {
		fmt.Printf("%s\n", err)
		return
	}
	for _, group := range groups {
		fmt.Printf("ID: %s, Name: %s\n", group.ID, group.Name)
	}

	return nil
}

func (app *appCtx) PinCell(parent amp.PinnedCell, req amp.PinOp) (amp.PinnedCell, error) {
	if parent != nil {
		return parent.PinCell(req)
	}

	// var path string
	// if url := req.Params().URL; url != nil {
	// 	query := url.Query()
	// 	paths := query[URLParam_PinPath]
	// 	if len(paths) == 0 {
	// 		return nil, amp.ErrCode_CellNotFound.Error("missing URL argument 'path'")
	// 	}
	// 	path = paths[0]
	// }

	return nil, nil
}
