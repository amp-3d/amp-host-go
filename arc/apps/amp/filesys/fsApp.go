package filesys

import (
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/arcspace/go-archost/arc"
	"github.com/arcspace/go-archost/arc/apps/amp/api"
	"github.com/arcspace/go-archost/arc/assets"
	"github.com/h2non/filetype"
)

func init() {
	filetype.AddType("jpeg", "image/jpeg")
}

type fsApp struct {
}

func (app *fsApp) AppURI() string {
	return AppURI
}

func (app *fsApp) SupportedDataModels() []string {
	return api.SupportedDataModels
}

func (app *fsApp) PinCell(req *arc.CellReq) error {

	if req.CellID == 0 {
		pathname, _ := req.GetKwArg(api.KwArg_CellURI)
		if pathname == "" {
			return arc.ErrCode_InvalidCell.Errorf("filesys: missing %q pathname", api.KwArg_CellURI)
		}
		item := fsInfo{}
		pathname = path.Clean(pathname)
		fi, err := os.Stat(pathname)
		if err != nil {
			return arc.ErrCode_InvalidCell.Errorf("path not found: %q", item.pathname)
		}
		item.pathname = pathname
		item.setFrom(fi)
		item.CellID = req.IssueCellID()
		req.Cell = item.newAppCell()

	} else {
		panic("ampApp should have caught this")
		// if req.ParentReq == nil || req.ParentReq.Cell == nil {
		// 	return arc.ErrCode_InvalidCell.Error("missing parent cell")
		// }

		// if err := req.ParentReq.Cell.PinCell(req); err != nil {
		// 	return err
		// }
	}

	return nil
}

type fsInfo struct {
	arc.CellID

	dataModel   string // TODO: make this a read-only util struct to facilitate cell schema access??
	basename    string // base file name
	pathname    string // only set for pinned items (could be alternative OS handle)
	lastRefresh time.Time
	isHidden    bool
	mode        os.FileMode
	size        int64
	isDir       bool
	modTime     time.Time
}

func (item *fsInfo) ID() arc.CellID {
	return item.CellID
}

func (item *fsInfo) CellDataModel() string {
	return item.dataModel
}

func (item *fsInfo) Compare(oth *fsInfo) int {
	// if item.isDir != oth.isDir {
	// 	return int(item.isDir) - int(oth.isDir)
	// }
	if diff := strings.Compare(item.basename, oth.basename); diff != 0 {
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

func (item *fsInfo) newAppCell() arc.AppCell {
	var appCell arc.AppCell

	if item.isDir {
		pinDir := &fsDir{
			fsInfo: *item,
		}
		appCell = pinDir
	} else {
		pinFile := &fsFile{
			fsInfo: *item,
		}
		appCell = pinFile
	}

	return appCell
}

func (item *fsInfo) setFrom(fi os.FileInfo) {
	item.basename = fi.Name()
	item.mode = fi.Mode()
	item.modTime = fi.ModTime()
	item.isHidden = strings.HasPrefix(item.basename, ".")
	item.isDir = fi.IsDir()
	if item.isDir {
		item.dataModel = api.CellDataModel_Playlist
	} else {
		item.dataModel = api.CellDataModel_Playable
		item.size = fi.Size()
	}
}

type fsFile struct {
	fsInfo // base file info
}

type fsDir struct {
	fsInfo                 // base file info
	items     []arc.CellID // ordered
	itemsByID map[arc.CellID]*fsInfo
}

// reads the fsDir's catalog and issues new items as needed.
func (dir *fsDir) readDir(req *arc.CellReq) error {

	{
		//dir.subs = make(map[arc.CellID]os.DirEntry)
		f, err := os.Open(dir.pathname)
		if err != nil {
			return err
		}
		defer f.Close()

		lookup := make(map[string]*fsInfo, len(dir.itemsByID))
		for _, sub := range dir.itemsByID {
			lookup[sub.basename] = sub
		}

		fsInfos, err := f.Readdir(-1)
		f.Close()
		if err != nil {
			return nil
		}

		N := len(fsInfos)
		dir.itemsByID = make(map[arc.CellID]*fsInfo, N)
		dir.items = dir.items[:0]

		var tmp *fsInfo
		for _, fi := range fsInfos {
			sub := tmp
			if sub == nil {
				sub = &fsInfo{}
			}
			sub.setFrom(fi)
			if sub.isHidden {
				continue
			}

			// preserve items that have not changed
			old := lookup[sub.basename]
			if old == nil || old.Compare(sub) != 0 {
				sub.CellID = req.IssueCellID()
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

func (dir *fsDir) PushCellState(req *arc.CellReq, opts arc.PushCellOpts) error {

	// Refresh if first time or too old
	now := time.Now()
	if dir.lastRefresh.Before(now.Add(-time.Minute)) {
		dir.lastRefresh = now
		dir.readDir(req)
	}

	// Push the dir as the content item (vs child)
	dir.pushCellState(req, arc.PushAsParent)

	// Push each dir sub item as a child cell
	for _, itemID := range dir.items {
		dir.itemsByID[itemID].pushCellState(req, arc.PushAsChild)
	}

	return nil
}

// TODO: use generics
func (dir *fsDir) PinCell(req *arc.CellReq) error {
	if req.CellID == dir.CellID {
		req.Cell = dir // FUTURE: a pinned dir returns more detailed attrs (e.g. reads mpeg tags)
		return nil
	}

	itemRef := dir.itemsByID[req.CellID]
	if itemRef == nil {
		return arc.ErrCode_InvalidCell.Error("invalid child cell")
	}

	itemRef.pathname = path.Join(dir.pathname, itemRef.basename)
	req.Cell = itemRef.newAppCell()
	return nil
}

func (file *fsFile) PinCell(req *arc.CellReq) error {
	// if err := file.setPathnameUsingParent(req); err != nil {
	// 	return err
	// }

	// In the future pinning a file can do fancy things but for now, just use the same item
	if req.CellID == file.CellID {
		req.Cell = file
		asset, err := assets.AssetForFilePathname(file.pathname, "")
		if err != nil {
			return err
		}
		req.User.Session().AssetServer().PublishAsset(asset)
		return nil
	}

	return arc.ErrCode_InvalidCell.Error("item is a file; no children to pin")
}

func (item *fsInfo) PushCellState(req *arc.CellReq, opts arc.PushCellOpts) error {
	return item.pushCellState(req, opts)
}

var (
	dirGlyph = &arc.AssetRef{
		MediaType: api.MimeType_Dir,
	}
)

func (item *fsInfo) pushCellState(req *arc.CellReq, opts arc.PushCellOpts) error {
	var schema *arc.AttrSchema
	if opts.PushAsChild() {
		schema = req.GetChildSchema(item.CellDataModel())
	} else {
		schema = req.ContentSchema
	}
	if schema == nil {
		return nil
	}

	if opts.PushAsChild() {
		req.PushInsertCell(item.CellID, schema)
	}

	mediaType, extLen := assets.GetMediaTypeForExt(item.basename)

	{
		base := item.basename[:len(item.basename)-extLen]
		left := ""
		right := ""
		splitAt := strings.LastIndex(base, " - ")
		if splitAt > 0 {
			left = base[:splitAt]
			right = base[splitAt+3:]
		}

		if len(left) > 0 && len(right) > 0 {
			req.PushAttr(item.CellID, schema, api.Attr_Title, right)
			req.PushAttr(item.CellID, schema, api.Attr_Subtitle, left)
		} else {
			req.PushAttr(item.CellID, schema, api.Attr_Title, base)
		}
	}

	if item.isDir {
		req.PushAttr(item.CellID, schema, api.Attr_Glyph, dirGlyph)
	} else {

		asset := arc.AssetRef{
			MediaType: mediaType,
		}
		req.PushAttr(item.CellID, schema, api.Attr_Glyph, &asset)
		if item.pathname != "" {
			asset.URI = item.pathname
			asset.Scheme = arc.URIScheme_File
			req.PushAttr(item.CellID, schema, api.Attr_Playable, &asset)
		}

		req.PushAttr(item.CellID, schema, api.Attr_ByteSz, item.size)
		req.PushAttr(item.CellID, schema, api.Attr_LastModified, arc.ConvertToTimeFS(item.modTime))

	}

	return nil
}
