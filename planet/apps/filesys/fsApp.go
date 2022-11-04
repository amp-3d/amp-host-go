package filesys

import (
	"os"
	"path"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/genesis3systems/go-planet/planet"
)

type fsApp struct {
	// openMu  sync.Mutex
	// openDirs map[string]*hfsDir
	nextID uint64
}

func (app *fsApp) AppURI() string {
	return AppURI
}

func (app *fsApp) DataModelURIs() []string {
	return []string{
		PinDir,
		PinFile,
	}
}


// IssueEphemeralID issued a new ID that will persist
func (app *fsApp) IssueCellID() planet.CellID {
	return planet.CellID(atomic.AddUint64(&app.nextID, 1) + 100)
}

func (app *fsApp) ResolveRequest(req *planet.CellReq) error {

	if req.Target == 0 {
		if req.CellURI == "" {
			return planet.ErrCode_InvalidCell.Error("invalid root URI")
		}

		req.Target = app.IssueCellID()

		dir := &hfsDir{
			CellID:   req.Target,
			pathname: path.Clean(req.CellURI),
		}

		//dir, _ := app.getOpenDir(req.URI, 0)
		req.AppItem = dir

	} else {
		if req.Parent == nil || req.Parent.AppItem == nil {
			return planet.ErrCode_InvalidCell.Error("invalid parent")
		}

		parent, ok := req.Parent.AppItem.(*hfsDir)
		if !ok {
			return planet.ErrCode_NotPinnable.Error("parent is not a hfsDir")
		}

		item := parent.itemByID[req.Target]
		if item == nil {
			return planet.ErrCode_InvalidCell.Error("invalid target cell")
		}

		switch {
		case item.isDir != 0:
			{
				dir := &hfsDir{
					CellID:   req.Target,
					pathname: path.Join(parent.pathname, item.name),
				}

				//dir, _ := app.getOpenDir(path.Join(parent.pathname, item.name), req.Target)
				req.AppItem = dir
			}
		default:
			{
				item := &hfsPlayable{}
				req.AppItem = item
			}
		}
	}

	return nil
}

func (app *fsApp) PushCellState(sub planet.CellSub) error {
	var err error

	switch item := sub.Req().AppItem.(type) {
	case *hfsDir:
		err = item.pushCellState(sub)
	case *hfsPlayable:
		err = item.pushCellState(sub)
	default:
		err = planet.ErrCode_InternalErr.Error("invalid AppItem")
	}

	return err
}

/*
func (app *fsApp) getOpenDir(pathname string, cellID planet.CellID) (*hfsDir, error) {
    pathname = path.Clean(pathname)

    app.openMu.Lock()
    defer app.openMu.Unlock()

    dir := app.openDirs[pathname]
    if dir == nil {
        if cellID == 0 {
            cellID = app.IssueCellID()
        }
        dir = &hfsDir{
			CellID:   cellID,
            pathname: pathname,
            //itemMap:  make(map[planet.CellID]*hfsItem),
        }
        app.openDirs[pathname] = dir
    }
    return dir, nil
}
*/

type hfsDir struct {
	planet.CellID

	pathname string // full hfs pathname
	readtAt  time.Time
	itemByID map[planet.CellID]*hfsItem
	//itemsByName  map[planet.CellID]*hfsItem
	items []*hfsItem // ordered
	//nextID   uint64
}

type hfsItem struct {
	planet.CellID

	name    string
	mode    os.FileMode
	size    int64
	isDir   int8
	modTime time.Time
}

func (item *hfsItem) Compare(oth *hfsItem) int {
	if item.isDir != oth.isDir {
		return int(item.isDir) - int(oth.isDir)
	}
	if diff := strings.Compare(item.name, oth.name); diff != 0 {
		return diff
	}
	if diff := item.modTime.Unix() - oth.modTime.Unix(); diff != 0 {
		return int(diff >> 31)
	}
	if diff := int(item.mode) - int(oth.mode); diff != 0 {
		return diff
	}
	if diff := item.size - oth.size; diff != 0 {
		return int(diff >> 31)
	}
	return 0
}

type hfsPlayable struct {
}

func (item *hfsPlayable) pushCellState(sub planet.CellSub) error {
	return nil
}

func (dir *hfsDir) readDir(sub planet.CellSub) error {

	app := sub.Req().App.(*fsApp)

	{
		//dir.subs = make(map[planet.CellID]os.DirEntry)
		f, err := os.Open(dir.pathname)
		if err != nil {
			return err
		}
		defer f.Close()

		lookup := make(map[string]*hfsItem, len(dir.itemByID))
		for _, sub := range dir.itemByID {
			lookup[sub.name] = sub
		}

		fsItems, err := f.Readdir(-1)
		f.Close()
		if err != nil {
			return nil
		}

		N := len(fsItems)
		dir.itemByID = make(map[planet.CellID]*hfsItem, N)

		dir.items = dir.items[:0]

		var tmp *hfsItem
		for _, fsItem := range fsItems {
			sub := tmp
			if sub == nil {
				sub = &hfsItem{}
			}
			sub.CellID = 0
			sub.name = fsItem.Name()
			if strings.HasPrefix(sub.name, ".") {
				continue
			}
			sub.mode = fsItem.Mode()
			sub.modTime = fsItem.ModTime()
			if fsItem.IsDir() {
				sub.isDir = 1
			} else {
				sub.size = fsItem.Size()
			}

			// preserve items that have not changed
			old := lookup[sub.name]
			if old == nil || old.Compare(sub) != 0 {
				sub.CellID = app.IssueCellID()
				tmp = nil
			} else {
				sub = old
			}

			dir.itemByID[sub.CellID] = sub
			dir.items = append(dir.items, sub)
		}

		items := dir.items
		sort.Slice(items, func(i, j int) bool {
			return items[i].Compare(items[j]) < 0
		})

	}
	return nil

}

func (dir *hfsDir) pushCellState(sub planet.CellSub) error {

	now := time.Now()
	if dir.readtAt.Before(now.Add(-time.Minute)) {
		dir.readtAt = now
		dir.readDir(sub)
	}

	req := sub.Req()

	batch := planet.NewMsgBatch()
	msgs := batch.Reset(2)

	for _, item := range dir.items {

		for i, m := range msgs {
			m.TargetCellID = item.CellID.U64()

			switch i {
			case 0:
				m.Op = planet.MsgOp_InsertChildCell
				m.ValType = uint64(planet.ValType_SchemaID)
				m.ValInt = int64(req.PinChildren[0].SchemaID)
			case 1:
				m.Op = planet.MsgOp_PushAttr
				m.AttrID = req.PinChildren[0].Attrs[1].AttrID // main.label
				m.SetVal(item.name)
				// case 2:
				//     msg.Op = planet.MsgOp_PushAttr
				//     msg.AttrID = req.PinChildren[0].Attrs[0].AttrID // main.glyph
				//     msg.SetVal(item.name)
				// case 3:
				//     msg.Op = planet.MsgOp_PushAttr
				//     msg.AttrID = req.PinChildren[0].Attrs[0].AttrID // MIME type
				//     msg.SetVal(item.MIMEType())
			}
		}
		sub.PushUpdate(batch)
	}

	{
		msgs := batch.Reset(1)
		m := msgs[0]
		m.Op = planet.MsgOp_Commit
		m.TargetCellID = dir.CellID.U64()

		sub.PushUpdate(batch)
	}

	batch.Reclaim()

	return nil
}

// // func (app *fsApp) ResolveRequest(req *planet.CellReq) error {
// //     req.AppItem =
// //     req.Target = app.IssueEphemeralID()
// //     return nil
// // }

// func (app *fsApp) ServeCell(sub planet.CellSub) error {

//     req := sub.Req()

//     dir := ""
//     items, err := os.ReadDir(dir)
//     sort.Slice(items, func(i, j int) bool {
//         ti, tj := 0, 0
//         if items[i].IsDir() {
//             ti = 1
//         }
//         if items[j].IsDir() {
//             tj = 1
//         }
//         if (ti != tj) {
//             return ti < tj
//         }

//         return items[i].Name() < items[j].Name()
//     })

//     batch := planet.NewMsgBatch()

//     for _, item := range items {
//         cellID := app.IssueEphemeralID()
//         msgs = batch.AddNew(3)

//         m := msgs[0]

//         m.TargetCellID = cellID.U64()
//         m.ReqID = req.ReqID

//         switch i {
//         case 0:
//             m.Op = planet.MsgOp_InsertChildCell
//             //m.ParentCellID = ?
//             m.ValType = uint64(planet.ValType_SchemaID)
//             m.ValInt = int64(req.PinChildren[0].SchemaID)
//         case 1:
//             msg.Op = planet.MsgOp_PushAttr
//             msg.TargetCellID = childID1
//             msg.AttrID = req.PinChildren[0].Attrs[1].AttrID // main.label
//             msg.SetVal("Hello World 1")
//         }
//     }

//     cellID := req.Target.U64()
//     childID1 := app.IssueEphemeralID().U64()  // TEMP
//     childID2 := app.IssueEphemeralID().U64()  // TEMP

//     for i, msg := range batch.AddNew(6) {
//         msg.ReqID = req.ReqID

//         switch i {
//         case 0:
//             msg.Op = planet.MsgOp_PushAttr
//             msg.TargetCellID = cellID
//         case 1:
//             msg.Op = planet.MsgOp_InsertChildCell
//             msg.TargetCellID = childID1
//             msg.ValType = uint64(planet.ValType_SchemaID)
//             msg.ValInt = int64(req.PinChildren[0].SchemaID)
//         case 2:
//             msg.Op = planet.MsgOp_PushAttr
//             msg.TargetCellID = childID1
//             msg.AttrID = req.PinChildren[0].Attrs[1].AttrID // main.label
//             msg.SetVal("Hello World 1")
//         case 3:
//             msg.Op = planet.MsgOp_InsertChildCell
//             msg.TargetCellID = childID2
//             msg.ValType = uint64(planet.ValType_SchemaID)
//             msg.ValInt = int64(req.PinChildren[0].SchemaID)
//         case 4:
//             msg.Op = planet.MsgOp_PushAttr
//             msg.TargetCellID = childID2
//             msg.AttrID = req.PinChildren[0].Attrs[1].AttrID // main.label
//             msg.SetVal("Hello World 2")
//         case 5:
//             msg.Op = planet.MsgOp_Commit
//             msg.TargetCellID = cellID
//         }
//     }

//     sub.Cell().PushUpdate(batch)
//     batch.Reclaim()

//     return nil
// }
