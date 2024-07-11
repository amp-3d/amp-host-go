package posix

import (
	"os"
	"path"
	"path/filepath"

	filesys "github.com/amp-3d/amp-host-go/amp/apps/amp.app.filesys"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/registry"
	"github.com/amp-3d/amp-sdk-go/amp/std"
	"github.com/h2non/filetype"
)

const (

	// PinURL param to pin the given absolute path
	URLParam_PinPath = "path"
)

func init() {
	filetype.AddType("jpeg", "image/jpeg")
	filetype.AddType("json", "text/x-json")
	filetype.AddType("md", "text/markdown")

	reg := registry.Global()
	reg.RegisterApp(&amp.App{
		AppSpec: filesys.AppSpec.With("posix"),
		Desc:    "local file system service",
		Version: "v1.2023.2",
		NewAppInstance: func(ctx amp.AppContext) (amp.AppInstance, error) {
			app := &appInst{}
			app.Instance = app
			app.AppContext = ctx
			return app, nil
		},
	})
}

type appInst struct {
	std.App[*appInst]
}

func (app *appInst) ServeRequest(op amp.Requester) (amp.Pin, error) {
	vals := op.Request().Values
	pathname := vals.Get(URLParam_PinPath)
	if pathname == "" {
		return nil, amp.ErrCode_BadRequest.Errorf("missing param %q", URLParam_PinPath)
	}
	pathname = path.Clean(pathname)
	fsInfo, err := os.Stat(pathname)
	if err != nil {
		return nil, amp.ErrCode_CellNotFound.Errorf("path not found: %q", pathname)
	}

	dirname := filepath.Dir(pathname)
	item := newFsItem(dirname, fsInfo)
	return app.PinAndServe(item, op)
}
