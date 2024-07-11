package chat

import (
	"github.com/amp-3d/amp-sdk-go/amp"
)

func RegisterApp(reg amp.Registry) {
	reg.RegisterApp(&amp.App{
		AppSpec: amp.AppSpec.With("chat"),
		Desc:    "",
		Version: "v0.7.0",
		NewAppInstance: func(ctx amp.AppContext) (amp.AppInstance, error) {
			return nil, nil
		},
	})
}
