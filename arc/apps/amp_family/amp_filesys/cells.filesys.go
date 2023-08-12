package amp_filesys

import (
	"os"
	"path"
	"strings"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp"
	"github.com/arcspace/go-archost/arc/assets"
)

type fsItem struct {
	amp.CellBase[*appCtx]

	basename string // base file name
	pathname string // non-nil when pinned (could be alternative OS handle)
	isHidden bool
	mode     os.FileMode
	size     int64
	isDir    bool
	modTime  time.Time

	hdr        arc.CellHeader
	text       arc.CellText
	mediaFlags amp.MediaFlags
}

func (item *fsItem) Compare(oth *fsItem) int {
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

func (item *fsItem) GetLogLabel() string {
	label := item.basename
	if item.isDir {
		label += "/"
	}
	return label
}

func (item *fsItem) setFrom(fi os.FileInfo) {
	item.basename = fi.Name()
	item.mode = fi.Mode()
	item.modTime = fi.ModTime()
	item.isHidden = strings.HasPrefix(item.basename, ".")
	item.isDir = fi.IsDir()

	mediaType := ""
	extLen := 0
	if item.isDir {

	} else {
		item.size = fi.Size()
		mediaType, extLen = assets.GetMediaTypeForExt(item.basename)
	}

	//////////////////  CellHeader
	{
		hdr := arc.CellHeader{
			Modified: int64(arc.ConvertToUTC(item.modTime)),
		}
		if item.isDir {
			hdr.Icon = amp.DirGlyph
		} else {
			hdr.Icon = &arc.AssetRef{
				MediaType: mediaType,
			}
			hdr.Link = &arc.AssetRef{
				MediaType: mediaType,
				URI:       item.pathname,
				Scheme:    arc.URIScheme_File,
			}
		}
		item.hdr = hdr
	}

	//////////////////  CellText
	{
		text := arc.CellText{}
		base := item.basename[:len(item.basename)-extLen]
		splitAt := strings.LastIndex(base, " - ")
		if splitAt > 0 {
			text.Title = base[splitAt+3:]
			text.Subtitle = base[:splitAt]
		} else {
			text.Title = base
		}
		item.text = text
	}

	//////////////////  MediaInfo
	item.mediaFlags = 0
	if !item.isDir {

		// TODO: make smarter
		switch {
		case strings.HasPrefix(mediaType, "audio/"):
			item.mediaFlags |= amp.HasAudio
		case strings.HasPrefix(mediaType, "video/"):
			item.mediaFlags |= amp.HasVideo
		}
		item.mediaFlags |= amp.IsSeekable
	}
}

func (item *fsItem) MarshalAttrs(app *appCtx, dst *arc.CellTx) error {
	dst.Marshal(app.CellHeaderAttr, 0, &item.hdr)
	dst.Marshal(app.CellTextAttr, 0, &item.text)
	return nil
}

func (item *fsItem) OnPinned(parent amp.Cell[*appCtx]) error {
	parentDir := parent.(*fsDir)
	item.pathname = path.Join(parentDir.pathname, item.basename)
	return nil
}

type fsFile struct {
	fsItem
	pinnedURL string
}

func (item *fsFile) MarshalAttrs(app *appCtx, dst *arc.CellTx) error {
	item.fsItem.MarshalAttrs(app, dst)

	if item.mediaFlags != 0 {
		media := &amp.MediaInfo{
			Flags:      item.mediaFlags,
			Title:      item.text.Title,
			Collection: item.text.Subtitle,
		}
		dst.Marshal(app.MediaInfoAttr, 0, media)
	}

	if item.pinnedURL != "" {
		dst.Marshal(app.PlayableAssetAttr, 0, &arc.AssetRef{
			URI: item.pinnedURL,
		})
	}

	return nil
}

func (item *fsFile) PinInto(dst *amp.PinnedCell[*appCtx]) error {
	asset, err := assets.AssetForFilePathname(item.pathname, "")
	if err != nil {
		return err
	}
	app := dst.App
	item.pinnedURL, err = app.PublishAsset(asset, arc.PublishOpts{
		HostAddr: app.Session().LoginInfo().HostAddr,
	})
	return err
}

type fsDir struct {
	fsItem
}

// reads the fsDir's catalog and issues new items as needed.
func (dir *fsDir) PinInto(dst *amp.PinnedCell[*appCtx]) error {
	/*

		{
			//dir.subs = make(map[arc.CellID]os.DirEntry)
			f, err := os.Open(dir.pathname)
			if err != nil {
				return err
			}
			defer f.Close()

			lookup := make(map[string]*fsItem, len(dir.itemsByID))
			for _, sub := range dir.itemsByID {
				lookup[sub.basename] = sub
			}

			dirItems, err := f.Readdir(-1)
			f.Close()
			if err != nil {
				return nil
			}

			N := len(dirItems)
			dir.itemsByID = make(map[arc.CellID]*fsItem, N)
			dir.items = dir.items[:0]

			var tmp *fsItem
			for _, fi := range dirItems {
				sub := tmp
				if sub == nil {
					sub = &fsItem{}
				}
				sub.setFrom(fi)
				if sub.isHidden {
					continue
				}

				// preserve items that have not changed
				old := lookup[sub.basename]
				if old == nil || old.Compare(sub) != 0 {
					sub.CellID = dir.app.IssueCellID()
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

			}*/
	return nil
}
