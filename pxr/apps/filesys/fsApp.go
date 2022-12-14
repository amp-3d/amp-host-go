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

	"github.com/arcspace/go-arcspace/pxr"
)

type fsApp struct {
	// openMu  sync.Mutex
	// openDirs map[string]*pinnedDir
	nextID uint64
}

func (app *fsApp) AppURI() string {
	return AppURI
}

func (app *fsApp) AttrModelURIs() []string {
	return DataModels[1:]
}

// IssueEphemeralID issued a new ID that will persist
func (app *fsApp) IssueCellID() pxr.CellID {
	return pxr.CellID(atomic.AddUint64(&app.nextID, 1) + 100)
}

func (app *fsApp) ResolveRequest(req *pxr.CellReq) error {
	item := fsItem{}

	if req.PinCell == 0 {
		if req.PinURI == "" {
			return pxr.ErrCode_InvalidCell.Error("invalid root URI")
		}

		item.pathname = path.Clean(req.PinURI)

		fi, err := os.Stat(item.pathname)
		if err != nil {
			return pxr.ErrCode_InvalidCell.Errorf("path not found: %q", item.pathname)
		}
		req.PinCell = app.IssueCellID()
		item.setFrom(fi)
		item.CellID = req.PinCell

	} else {
		if req.ParentReq == nil || req.ParentReq.PinnedCell == nil {
			return pxr.ErrCode_InvalidCell.Error("parent cell is nil")
		}

		parent, ok := req.ParentReq.PinnedCell.(*pinnedDir)
		if !ok {
			return pxr.ErrCode_NotPinnable.Error("parent is not a pinned dir")
		}

		itemRef := parent.itemByID[req.PinCell]
		if itemRef == nil {
			return pxr.ErrCode_InvalidCell.Error("invalid target cell")
		}

		item = *itemRef
		item.pathname = path.Join(parent.pathname, item.name)
	}

	switch item.model {
	case DirItem:
		pinned := &pinnedDir{}
		pinned.fsItem = item
		req.PinnedCell = pinned
	case FileItem:
		req.PinnedCell = &item
	}

	return nil
}

type pinnedDir struct {
	fsItem             // base file info
	items    []*fsItem // ordered
	itemByID map[pxr.CellID]*fsItem
}

type fsItem struct {
	pxr.CellID

	name        string // base file name
	pathname    string // only set for pinned items  (could be alternative OS handle)
	lastRefresh time.Time
	isHidden    bool
	mode        os.FileMode
	size        int64
	model       DataModel
	modTime     time.Time
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

func (dir *pinnedDir) readDir(req *pxr.CellReq) error {
	app := req.ParentApp.(*fsApp)

	{
		//dir.subs = make(map[pxr.CellID]os.DirEntry)
		f, err := os.Open(dir.pathname)
		if err != nil {
			return err
		}
		defer f.Close()

		lookup := make(map[string]*fsItem, len(dir.itemByID))
		for _, sub := range dir.itemByID {
			lookup[sub.name] = sub
		}

		fsItems, err := f.Readdir(-1)
		f.Close()
		if err != nil {
			return nil
		}

		N := len(fsItems)
		dir.itemByID = make(map[pxr.CellID]*fsItem, N)
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

func (dir *pinnedDir) PushCellState(req *pxr.CellReq) error {

	// Refresh if first time or too old
	now := time.Now()
	if dir.lastRefresh.Before(now.Add(-time.Minute)) {
		dir.lastRefresh = now
		dir.readDir(req)
	}

	dir.pushCellState(req, false)
	for _, item := range dir.items {
		item.pushCellState(req, true)
	}

	return nil
}

func (item *fsItem) PushCellState(req *pxr.CellReq) error {
	return item.pushCellState(req, false)
}

func (item *fsItem) setFrom(fi os.FileInfo) {
	item.name = fi.Name()
	item.mode = fi.Mode()
	item.modTime = fi.ModTime()
	item.isHidden = strings.HasPrefix(item.name, ".")
	switch {
	case fi.IsDir():
		item.model = DirItem
	// case strings.HasSuffix(sub.name, ".mp3"):
	// 	sub.model = PlayableCell
	default:
		item.model = FileItem
		item.size = fi.Size()
	}
}

func (item *fsItem) pushCellState(req *pxr.CellReq, asChild bool) error {
	schema := req.ContentSchema
	if asChild {
		schema = req.GetChildSchema(DataModels[item.model])
	}

	if schema == nil {
		return nil
	}
	req.PushInsertCell(item.CellID, schema)

	req.PushAttr(item.CellID, schema, attr_ItemName, item.name)

	if !asChild {
		req.PushAttr(item.CellID, schema, attr_Pathname, item.pathname)
	}

	switch item.model {
	case DirItem:
		req.PushAttr(item.CellID, schema, attr_MimeType, "filesys/directory")
	case FileItem:
		if mimeType := mime.TypeByExtension(filepath.Ext(item.name)); len(mimeType) > 1 {
			req.PushAttr(item.CellID, schema, attr_MimeType, mimeType)
		}
		req.PushAttr(item.CellID, schema, attr_ByteSz, item.size)
		req.PushAttr(item.CellID, schema, attr_LastModified, pxr.ConvertToTimeFS(item.modTime))
	}
	return nil
}
