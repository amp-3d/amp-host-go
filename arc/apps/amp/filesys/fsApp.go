package filesys

import (
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/arcspace/go-arcspace/arc"
)

// data model type of a fsItem
type dataModel int

const (
	dataModel_nil dataModel = iota
	dataModel_Dir
	dataModel_File
)

type fsApp struct {
	nextID uint64
}

func (app *fsApp) AppURI() string {
	return AppURI
}

func (app *fsApp) CellDataModels() []string {
	return []string{
		CellDataModel_Dir,
		CellDataModel_File,
	}
}

// Issues a new and unique ID that will persist during runtime
func (app *fsApp) IssueCellID() arc.CellID {
	return arc.CellID(atomic.AddUint64(&app.nextID, 1) + 100)
}

func (app *fsApp) ResolveRequest(req *arc.CellReq) error {
	item := fsItem{}

	if req.PinCell == 0 {
		pinPath, _ := req.GetKwArg(KwArg_PinPath)
		if pinPath == "" {
			return arc.ErrCode_InvalidCell.Errorf("missing Cell ID or valid %q arg", KwArg_PinPath)
		}
		item.pathname = path.Clean(pinPath)

		fi, err := os.Stat(item.pathname)
		if err != nil {
			return arc.ErrCode_InvalidCell.Errorf("path not found: %q", item.pathname)
		}

		item.setFrom(fi)
		item.CellID = app.IssueCellID()

	} else {
		if req.ParentReq == nil || req.ParentReq.PinnedCell == nil {
			return arc.ErrCode_InvalidCell.Error("missing parent")
		}

		parent, ok := req.ParentReq.PinnedCell.(*pinnedDir)
		if !ok {
			return arc.ErrCode_NotPinnable.Error("parent is not a pinned dir")
		}

		itemRef := parent.itemsByID[req.PinCell]
		if itemRef == nil {
			return arc.ErrCode_InvalidCell.Error("invalid target cell")
		}

		item = *itemRef
		item.pathname = path.Join(parent.pathname, item.name)
	}

	switch item.model {
	case dataModel_Dir:
		pinned := &pinnedDir{}
		pinned.fsItem = item
		req.PinnedCell = pinned
	case dataModel_File:
		req.PinnedCell = &item
	}

	return nil
}

type pinnedDir struct {
	fsItem                 // base file info
	items     []arc.CellID // ordered
	itemsByID map[arc.CellID]*fsItem
}

type fsItem struct {
	arc.CellID

	name        string // base file name
	pathname    string // only set for pinned items  (could be alternative OS handle)
	lastRefresh time.Time
	isHidden    bool
	mode        os.FileMode
	size        int64
	model       dataModel
	modTime     time.Time
}

func (item *fsItem) ID() arc.CellID {
	return item.CellID
}

func (item *fsItem) Compare(oth *fsItem) int {
	// if item.isDir != oth.isDir {
	// 	return int(item.isDir) - int(oth.isDir)
	// }
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

// reads the pinnedDir's catalog and issues new items as needed.
func (dir *pinnedDir) readDir(req *arc.CellReq) error {
	app := req.ParentApp.(*fsApp)

	{
		//dir.subs = make(map[arc.CellID]os.DirEntry)
		f, err := os.Open(dir.pathname)
		if err != nil {
			return err
		}
		defer f.Close()

		lookup := make(map[string]*fsItem, len(dir.itemsByID))
		for _, sub := range dir.itemsByID {
			lookup[sub.name] = sub
		}

		fsItems, err := f.Readdir(-1)
		f.Close()
		if err != nil {
			return nil
		}

		N := len(fsItems)
		dir.itemsByID = make(map[arc.CellID]*fsItem, N)
		dir.items = dir.items[:0]

		var tmp *fsItem
		for _, fi := range fsItems {
			sub := tmp
			if sub == nil {
				sub = &fsItem{}
			}
			sub.setFrom(fi)
			if sub.isHidden {
				continue
			}

			// preserve items that have not changed
			old := lookup[sub.name]
			if old == nil || old.Compare(sub) != 0 {
				sub.CellID = app.IssueCellID()
				tmp = nil
			} else {
				sub = old
			}

			dir.itemsByID[sub.CellID] = sub
			dir.items = append(dir.items, sub.CellID)
		}

		items := dir.items
		sort.Slice(items, func(i, j int) bool {
			ii := dir.itemsByID[items[i]]
			jj := dir.itemsByID[items[j]]
			return ii.Compare(jj) < 0
		})

	}
	return nil
}

func (dir *pinnedDir) PushCellState(req *arc.CellReq) error {

	// Refresh if first time or too old
	now := time.Now()
	if dir.lastRefresh.Before(now.Add(-time.Minute)) {
		dir.lastRefresh = now
		dir.readDir(req)
	}

	// Push the dir as the content item (vs child)
	dir.pushCellState(req, false)

	// Push each dir item as a child cell
	for _, itemID := range dir.items {
		dir.itemsByID[itemID].pushCellState(req, true)
	}

	return nil
}

func (item *fsItem) PushCellState(req *arc.CellReq) error {
	return item.pushCellState(req, false)
}

func (item *fsItem) setFrom(fi os.FileInfo) {
	item.name = fi.Name()
	item.mode = fi.Mode()
	item.modTime = fi.ModTime()
	item.isHidden = strings.HasPrefix(item.name, ".")
	switch {
	case fi.IsDir():
		item.model = dataModel_Dir
	// case strings.HasSuffix(sub.name, ".mp3"):
	// 	sub.model = PlayableCell
	default:
		item.model = dataModel_File
		item.size = fi.Size()
	}
}

func (item *fsItem) DataModelURI() string {
	switch item.model {
	case dataModel_Dir:
		return CellDataModel_Dir
	case dataModel_File:
		return CellDataModel_File
	}
	return ""
}

func (item *fsItem) pushCellState(req *arc.CellReq, asChild bool) error {
	schema := req.ContentSchema
	if asChild {
		schema = req.GetChildSchema(item.DataModelURI())
	}
	if schema == nil {
		return nil
	}

	if asChild {
		req.PushInsertCell(item.CellID, schema)
	}

	req.PushAttr(item.CellID, schema, attr_ItemName, item.name)

	if !asChild {
		req.PushAttr(item.CellID, schema, attr_Pathname, item.pathname)
	}

	switch item.model {
	case dataModel_Dir:
		req.PushAttr(item.CellID, schema, attr_MimeType, "filesys/directory")
	case dataModel_File:
		if mimeType := mime.TypeByExtension(filepath.Ext(item.name)); len(mimeType) > 1 {
			req.PushAttr(item.CellID, schema, attr_MimeType, mimeType)
		}
		req.PushAttr(item.CellID, schema, attr_ByteSz, item.size)
		req.PushAttr(item.CellID, schema, attr_LastModified, arc.ConvertToTimeFS(item.modTime))
	}
	return nil
}
