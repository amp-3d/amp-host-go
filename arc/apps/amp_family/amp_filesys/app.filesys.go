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
	AppID = "filesys" + amp.AppFamilyDomain
)

func UID() arc.UID {
	return arc.FormUID(0x3dae178d099340dc, 0x8b111f3a4a6b0263)
}

func RegisterApp(reg arc.Registry) {
	reg.RegisterApp(&arc.AppModule{
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

func (app *appCtx) PinCell(parent arc.PinnedCell, req arc.PinReq) (arc.PinnedCell, error) {
	var pathname string
	if url := req.Params().URL; url != nil {
		pathname = url.Path
		pathname = path.Clean(pathname)
		cell, err := app.newCellForPath(pathname)
		if err != nil {
			return nil, err
		}
		return amp.NewPinnedCell(app, cell)
	}

	if parent != nil {
		return parent.PinCell(req)
	}

	return nil, arc.ErrCode_CellNotFound.Error("missing cell ID / URL")
}

func (app *appCtx) newCellForPath(pathname string) (*amp.CellBase[*appCtx], error) {
	if pathname == "" {
		return nil, arc.ErrCode_CellNotFound.Error("missing cell ID / URL")
	}
	if len(pathname) > 0 && pathname[0] == '/' {
		pathname = pathname[1:]
	}
	fi, err := os.Stat(pathname)
	if err != nil {
		return nil, arc.ErrCode_CellNotFound.Errorf("local pathname not found: %q", pathname)
	}

	var item *fsItem
	isDir := fi.IsDir()
	if isDir {
		dir := &fsDir{}
		dir.Self = dir
		item = &dir.fsItem
	} else {
		file := &fsFile{}
		file.Self = file
		item = &file.fsItem
	}
	item.pathname = pathname
	item.setFrom(fi)
	item.CellID = app.IssueCellID()

	return &item.CellBase, nil
}

// func (app *appCtx) pinByPath(pathname string, req arc.PinReq) (arc.PinnedCell, error) {
// 	if pathname == "" {
// 		if url := req.URL(); url != nil {
// 			pathname = url.Path
// 		}
// 		pathname = path.Clean(pathname)
// 	}
// 	// if pathname == "" {
// 	// 	return nil, arc.ErrCode_CellNotFound.Error("missing cell ID / URL")
// 	// }
// 	// if len(pathname) > 0 && pathname[0] == '/' {
// 	// 	pathname = pathname[1:]
// 	// }
// 	// fi, err := os.Stat(pathname)
// 	// if err != nil {
// 	// 	return nil, arc.ErrCode_CellNotFound.Errorf("local pathname not found: %q", pathname)
// 	// }

// 	var fsCell *fsItem
// 	isDir := fi.IsDir()
// 	if isDir {
// 		//fsCell = (&fsDir{}).
// 	} else {
// 		fsCell = &fsFile{}
// 	}
// 	fsCell.pathname = pathname
// 	fsCell.setFrom(fi)
// 	fsCell.CellID = app.IssueCellID()
// 	//fsCell.CellSpec =

// 	pinned, err := fsCell.SpawnAsPinnedCell(app, pathname)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return pinned, nil
// }
