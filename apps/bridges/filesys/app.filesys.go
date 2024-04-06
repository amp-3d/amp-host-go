package filesys

import (
	"os"
	"path"

	"github.com/amp-space/amp-host-go/apps/av"
	"github.com/amp-space/amp-host-go/apps/bridges"
	"github.com/amp-space/amp-sdk-go/amp"
	"github.com/h2non/filetype"
)

func init() {
	filetype.AddType("jpeg", "image/jpeg")
	filetype.AddType("json", "text/x-json")
	filetype.AddType("md", "text/markdown")
}

const (
	AppID = "filesys" + bridges.AppFamilyDomain

	// PinURL param to pin the given absolute path
	URLParam_PinPath = "path"
)

func UID() amp.UID {
	return amp.FormUID(0x3dae178d099340dc, 0x8b111f3a4a6b0263)
}

func RegisterApp(reg amp.Registry) {
	reg.RegisterElemType(&av.MediaPlaylist{})

	reg.RegisterApp(&amp.App{
		AppID:   AppID,
		UID:     UID(),
		Desc:    "local file system service",
		Version: "v1.2023.2",
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
	return nil
}

func (app *appCtx) PinCell(parent amp.PinnedCell, req amp.PinReq) (amp.PinnedCell, error) {
	if parent != nil {
		return parent.PinCell(req)
	}

	var path string
	if url := req.Params().URL; url != nil {
		query := url.Query()
		paths := query[URLParam_PinPath]
		if len(paths) == 0 {
			return nil, amp.ErrCode_CellNotFound.Error("missing URL argument 'path'")
		}
		path = paths[0]
	}

	return app.pinnedCellForPath(path)
}

func (app *appCtx) pinnedCellForPath(pathname string) (amp.PinnedCell, error) {
	pathname = path.Clean(pathname)
	if pathname == "" {
		return nil, amp.ErrCode_CellNotFound.Error("missing cell ID / URL")
	}

	fsInfo, err := os.Stat(pathname)
	if err != nil {
		return nil, amp.ErrCode_CellNotFound.Errorf("path not found: %q", pathname)
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

	return av.NewPinnedCell(app, &item.CellBase)
}
