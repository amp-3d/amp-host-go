package posix

import (
	"os"
	"path"

	av "github.com/amp-3d/amp-host-go/amp/apps/amp-app-av"
	filesys "github.com/amp-3d/amp-host-go/amp/apps/amp-app-filesys"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/h2non/filetype"
)

func init() {
	filetype.AddType("jpeg", "image/jpeg")
	filetype.AddType("json", "text/x-json")
	filetype.AddType("md", "text/markdown")
}

const (
	AppID = "posix" + filesys.AppFamilyDomain

	// PinURL param to pin the given absolute path
	URLParam_PinPath = "path"
)

func RegisterApp(reg amp.Registry) {
	reg.RegisterPrototype("", &av.MediaPlaylist{})

	reg.RegisterApp(&amp.App{
		AppID:   AppID,
		TagID:   amp.StringToTagID(AppID),
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

func (app *appCtx) PinCell(parent amp.PinnedCell, op amp.PinOp) (amp.PinnedCell, error) {
	if parent != nil {
		return parent.PinCell(op)
	}

	var path string
	if url := op.URL(); url != nil {
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
