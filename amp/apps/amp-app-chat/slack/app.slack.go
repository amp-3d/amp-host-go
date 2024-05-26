package app_

import (
	"fmt"

	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/basic"
	"github.com/amp-3d/amp-sdk-go/stdlib/tag"
	"github.com/slack-go/slack"
)

const (
	AppID = "amp.app.chat.slack"
)

func RegisterApp(reg amp.Registry) {
	reg.RegisterApp(&amp.App{
		AppSpec: tag.FormSpec(amp.AppSpec, "chat.slack"),
		Desc:    "bridge for Slack",
		Version: "v1.0",
		NewAppInstance: func(ctx amp.AppContext) (amp.AppInstance, error) {
			app := &appInst{}
			app.Instance = app
			app.AppContext = ctx
			return app, nil
		},
	})
}

type appInst struct {
	basic.App[*appInst]
}

func (app *appInst) MakeReady(req amp.Requester) error {
	return nil // TODO
}

func (app *appInst) ServeRequest(op amp.Requester) (amp.Pin, error) {

	api := slack.New("YOUR_TOKEN_HERE")
	// If you set debugging, it will log all requests to the console
	// Useful when encountering issues
	// slack.New("YOUR_TOKEN_HERE", slack.OptionDebug(true))
	groups, err := api.GetUserGroups(slack.GetUserGroupsOptionIncludeUsers(false))
	if err != nil {
		fmt.Printf("%s\n", err)
		return nil, err
	}
	for _, group := range groups {
		fmt.Printf("ID: %s, Name: %s\n", group.ID, group.Name)
	}

	return nil, nil //app.PinAndServe(nil /* TODO */)
}
