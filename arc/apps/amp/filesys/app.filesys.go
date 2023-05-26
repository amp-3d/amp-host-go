package filesys

import (
	"os"
	"path"

	"github.com/arcspace/go-archost/arc"
	"github.com/arcspace/go-archost/arc/apps/amp/api"
	"github.com/h2non/filetype"
)

const (
	AppID = "arcspace.systems.app.amp.filesys"
)

func init() {
	filetype.AddType("jpeg", "image/jpeg")
}

func init() {
	filetype.AddType("jpeg", "image/jpeg")

	arc.RegisterApp(&arc.AppModule{
		AppID:   AppID,
		Version: "v1.2023.2",
		//DataModels: api.DataModels,
		NewAppInstance: func(ctx arc.AppContext) (arc.AppRuntime, error) {
			app := &appCtx{
				AppContext: ctx,
			}
			return app, nil
		},
	})
}

type appCtx struct {
	arc.AppContext
}

func (app *appCtx) AppID() string {
	return api.AmpAppURI
}

func (app *appCtx) HandleAppMsg(m *arc.AppMsg) (handled bool, err error) {
	return false, nil
}

func (app *appCtx) OnClosing() {
}

func (app *appCtx) PinCell(req *arc.CellReq) (arc.AppCell, error) {

	if req.PinCell == 0 {
		pathname, _ := req.GetKwArg(api.KwArg_CellURI)
		if pathname == "" {
			return nil, arc.ErrCode_CellNotFound.Errorf("filesys: missing %q pathname", api.KwArg_CellURI)
		}
		item := fsInfo{
			app: app,
		}
		pathname = path.Clean(pathname)
		fi, err := os.Stat(pathname)
		if err != nil {
			return nil, arc.ErrCode_CellNotFound.Errorf("path not found: %q", item.pathname)
		}
		item.pathname = pathname
		item.setFrom(fi)
		item.CellID = app.IssueCellID()
		return item.newAppCell(), nil
	}

	return nil, nil
}
