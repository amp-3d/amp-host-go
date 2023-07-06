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
	AppID = "v1.filesys" + amp.AppFamilyDomain
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

func (app *appCtx) ResolveCell(req arc.CellReq) (arc.PinnedCell, error) {
	var pathname string
	if url := req.URL(); url != nil {
		pathname = url.Path
	}
	pathname = path.Clean(pathname)
	cell, err := app.newCellForPath(pathname)
	if err != nil {
		return nil, err
	}

	pinned, err := cell.SpawnAsPinnedCell(app, pathname)
	if err != nil {
		return nil, err
	}

	return pinned, nil
}

func (app *appCtx) newCellForPath(pathname string) (amp.Cell[*appCtx], error) {
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

	var cell amp.Cell[*appCtx]
	var item *fsItem
	isDir := fi.IsDir()
	if isDir {
		dir := &fsDir{}
		cell = dir
		item = &dir.fsItem
	} else {
		file := &fsFile{}
		cell = file
		item = &file.fsItem
	}
	item.pathname = pathname
	item.setFrom(fi)
	item.CellID = app.IssueCellID()

	return cell, nil
}

// func (app *appCtx) pinByPath(pathname string, req arc.CellReq) (arc.PinnedCell, error) {
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
