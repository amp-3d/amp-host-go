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

	"github.com/genesis3systems/go-planet/planet"
)

type fsApp struct {
	// openMu  sync.Mutex
	// openDirs map[string]*pinnedDir
	nextID uint64
}

func (app *fsApp) AppURI() string {
	return AppURI
}

func (app *fsApp) DataModelURIs() []string {
	return DataModels[1:]
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

		dir := &pinnedDir{
			pathname: path.Clean(req.CellURI),
		}
		fi, err := os.Stat(dir.pathname)
		if err != nil {
			return planet.ErrCode_InvalidCell.Errorf("path not found: %q", dir.pathname)
		}
		dir.fsItem.setFrom(fi)
		req.PinnedCell = dir

	} else {
		if req.Parent == nil || req.Parent.PinnedCell == nil {
			return planet.ErrCode_InvalidCell.Error("parent cell is nil")
		}

		parent, ok := req.Parent.PinnedCell.(*pinnedDir)
		if !ok {
			return planet.ErrCode_NotPinnable.Error("parent is not an pinnedDir")
		}

		item := parent.itemByID[req.Target]
		if item == nil {
			return planet.ErrCode_InvalidCell.Error("invalid target cell")
		}

		//
		switch item.model {
		case DirItem:
			pinned := &pinnedDir{
				pathname: path.Join(parent.pathname, item.name),
			}
			pinned.fsItem = *item
			req.PinnedCell = pinned
		// case PlayableItem:
		// 	playable := &hfsPlayable{}
		// 	req.AppCell = playable
		case FileItem:
			req.PinnedCell = item
		}
	}

	return nil
}

type pinnedDir struct {
	fsItem

	pathname string    // full pathname (couple be some other OS handle to a file system dir item)
	items    []*fsItem // ordered
	itemByID map[planet.CellID]*fsItem
}

type fsItem struct {
	planet.CellID

	lastRefresh time.Time
	isHidden    bool
	name        string // base file name
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

const crateURL = "crate-asset://crates.planet.tools/filesys.crate/"

func (dir *pinnedDir) readDir(req *planet.CellReq) error {
	app := req.ParentApp.(*fsApp)

	{
		//dir.subs = make(map[planet.CellID]os.DirEntry)
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
		dir.itemByID = make(map[planet.CellID]*fsItem, N)
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

func (dir *pinnedDir) PushCellState(req *planet.CellReq) error {

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

func (item *fsItem) PushCellState(req *planet.CellReq) error {
	return item.pushCellState(req, false)
}

func (item *fsItem) setFrom(fi os.FileInfo) {
	item.CellID = 0
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

func (item *fsItem) pushCellState(req *planet.CellReq, asChild bool) error {
	schema := req.ParentSchema
	if asChild {
		schema = req.GetChildSchema(DataModels[item.model])
	}

	if schema == nil {
		return nil
	}
	if asChild {
		req.PushInsertChildCell(item.CellID, schema)
	}

	req.PushAttr(item.CellID, schema, attr_ItemName, item.name)

	url := crateURL
	switch {
	case item.model == DirItem:
		url += "generic-dir"
	default:
		url += "generic-file"
	}
	req.PushAttr(item.CellID, schema, attr_ThumbGlyphURL, url)

	if item.model == FileItem {
		mimeType := mime.TypeByExtension(filepath.Ext(item.name))
		req.PushAttr(item.CellID, schema, attr_MimeType, mimeType)

		req.PushAttr(item.CellID, schema, attr_ByteSz, item.size)

		req.PushAttr(item.CellID, schema, attr_LastModified, planet.ConvertToTimeFS(item.modTime))
	}
	return nil
}

// // func (app *fsApp) ResolveRequest(req *planet.CellReq) error {
// //     req.AppItem =
// //     req.Target = app.IssueEphemeralID()
// //     return nil
// // }
