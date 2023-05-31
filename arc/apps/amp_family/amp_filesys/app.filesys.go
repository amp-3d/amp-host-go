package amp_filesys

import (
	"os"
	"path"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp"
	"github.com/h2non/filetype"
)

func init() {
	filetype.AddType("jpeg", "image/jpeg")
}

const (
	AppURI = amp.AppFamily + "filesys/v1"
)

func UID() arc.UID {
	return arc.FormUID(0x3dae178d099340dc, 0x8b111f3a4a6b0263)
}

func RegisterApp(reg arc.Registry) {
	reg.RegisterApp(&arc.AppModule{
		URI:     AppURI,
		UID:     UID(),
		Desc:    "local file system service",
		Version: "v1.2023.2",
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

func (app *appCtx) HandleMetaMsg(msg *arc.Msg) (handled bool, err error) {
	return false, nil
}

func (app *appCtx) OnClosing() {
}

func (app *appCtx) PinCell(req *arc.CellReq) (arc.Cell, error) {

	if req.PinCell == 0 {
		pathname, _ := req.GetKwArg(amp.KwArg_CellURI)
		if pathname == "" {
			return nil, arc.ErrCode_CellNotFound.Errorf("filesys: missing %q pathname", amp.KwArg_CellURI)
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
