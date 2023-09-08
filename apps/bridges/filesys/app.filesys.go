package filesys

import (
	"os"
	"path"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/apps/amp"
	"github.com/arcspace/go-archost/apps/bridges"
	"github.com/h2non/filetype"
)

func init() {
	filetype.AddType("jpeg", "image/jpeg")
}

const (
	AppID = "filesys" + bridges.AppFamilyDomain

	// PinURL param to pin the given absolute path
	URLParam_PinPath = "path"
)

func UID() arc.UID {
	return arc.FormUID(0x3dae178d099340dc, 0x8b111f3a4a6b0263)
}

func RegisterApp(reg arc.Registry) {
	reg.RegisterElemType(&amp.MediaPlaylist{})

	reg.RegisterApp(&arc.App{
		AppID:   AppID,
		UID:     UID(),
		Desc:    "local file system service",
		Version: "v1.2023.2",
		NewAppInstance: func() arc.AppInstance {
			return &appCtx{}
		},
	})
}

type appCtx struct {
	amp.AppBase
}

func (app *appCtx) OnNew(ctx arc.AppContext) (err error) {
	err = app.AppBase.OnNew(ctx)
	if err != nil {
		return
	}
	return nil
}

func (app *appCtx) PinCell(parent arc.PinnedCell, req arc.PinReq) (arc.PinnedCell, error) {
	if parent != nil {
		return parent.PinCell(req)
	}

	var path string
	if url := req.Params().URL; url != nil {
		query := url.Query()
		paths := query[URLParam_PinPath]
		if len(paths) == 0 {
			return nil, arc.ErrCode_CellNotFound.Error("missing URL argument 'path'")
		}
		path = paths[0]
	}

	return app.pinnedCellForPath(path)
}

func (app *appCtx) pinnedCellForPath(pathname string) (arc.PinnedCell, error) {
	pathname = path.Clean(pathname)
	if pathname == "" {
		return nil, arc.ErrCode_CellNotFound.Error("missing cell ID / URL")
	}

	fsInfo, err := os.Stat(pathname)
	if err != nil {
		return nil, arc.ErrCode_CellNotFound.Errorf("path not found: %q", pathname)
	}

	var item *fsItem
	if fsInfo.IsDir() {
		dir := &fsDir{}
		dir.Self = dir
		item = &dir.fsItem
	} else {
		file := &fsFile{}
		file.Self = file
		item = &file.fsItem
	}
	item.pathname = pathname
	item.setFrom(fsInfo)

	return amp.NewPinnedCell(app, &item.CellBase)
}
