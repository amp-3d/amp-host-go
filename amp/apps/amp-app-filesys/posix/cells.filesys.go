package posix

import (
	"os"
	"path"
	"sort"
	"strings"
	"time"

	av "github.com/amp-3d/amp-host-go/amp/apps/amp-app-av"
	"github.com/amp-3d/amp-host-go/amp/assets"
	"github.com/amp-3d/amp-sdk-go/amp"
)

type fsItem struct {
	av.CellBase[*appCtx]

	basename  string // base file name
	pathname  string // non-nil when pinned (could be alternative OS handle)
	mode      os.FileMode
	size      int64
	isDir     bool
	modTime   time.Time
	mediaType string

	hdr        amp.CellHeader
	mediaFlags av.MediaFlags
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
	item.isDir = fi.IsDir()

	extLen := 0
	if !item.isDir {
		item.size = fi.Size()
		item.mediaType, extLen = assets.GetMediaTypeForExt(item.basename)
	}

	stripExt := false

	item.mediaFlags = 0
	if !item.isDir {

		// TODO: make smarter
		switch {
		case strings.HasPrefix(item.mediaType, "audio/"):
			item.mediaFlags |= av.HasAudio
			stripExt = true
		case strings.HasPrefix(item.mediaType, "video/"):
			item.mediaFlags |= av.HasVideo
			stripExt = true
		}
		item.mediaFlags |= av.IsSeekable
	}

	//////////////////  CellHeader
	{
		hdr := amp.CellHeader{}
		hdr.SetModifiedAt(item.modTime)
		if item.isDir {
			hdr.Glyphs = []*amp.AssetTag{
				av.DirGlyph,
			}
		} else {
			hdr.Glyphs = []*amp.AssetTag{
				{
					URL: amp.GenericGlyphURL + item.mediaType,
				},
			}
		}
		base := item.basename
		if stripExt {
			base = item.basename[:len(base)-extLen]
		}
		splitAt := strings.LastIndex(base, " - ")
		if splitAt > 0 {
			hdr.Title = base[splitAt+3:]
			hdr.Subtitle = base[:splitAt]
		} else {
			hdr.Title = base
		}
		item.hdr = hdr
	}

}

func (item *fsItem) MarshalAttrs(dst *amp.TxMsg, ctx amp.PinContext) error {
	op := item.FormAttrUpsert(amp.CellHeaderAttrID)
	ctx.MarshalTxOp(dst, op, &item.hdr)
	return nil
}

func (item *fsItem) OnPinned(parent av.Cell[*appCtx]) error {
	parentDir := parent.(*fsDir)
	item.pathname = path.Join(parentDir.pathname, item.basename)
	return nil
}

type fsFile struct {
	fsItem
	pinnedURL string
}

func (item *fsFile) MarshalAttrs(dst *amp.TxMsg, ctx amp.PinContext) error {
	item.fsItem.MarshalAttrs(dst, ctx)

	if item.mediaFlags != 0 {
		media := av.PlayableMediaItem{
			Flags:      item.mediaFlags,
			Title:      item.hdr.Title,
			Collection: item.hdr.Subtitle,
		}

		if item.pinnedURL != "" {
			media.Tracks = []*amp.AssetTag{
				{
					ContentType: item.mediaType,
					URL:         item.pinnedURL,
				},
			}
		}

		op := item.FormAttrUpsert(av.PlayableMediaItemID)
		ctx.MarshalTxOp(dst, op, &media)
	}
	return nil
}

func (item *fsFile) PinInto(dst *av.PinnedCell[*appCtx]) error {
	asset, err := assets.AssetForFilePathname(item.pathname, "")
	if err != nil {
		return err
	}
	app := dst.App
	item.pinnedURL, err = app.PublishAsset(asset, amp.PublishOpts{
		HostAddr: app.Session().LoginInfo().HostAddr,
	})
	return err
}

type fsDir struct {
	fsItem
}

// reads the fsDir's catalog and issues new items as needed.
func (dir *fsDir) PinInto(dst *av.PinnedCell[*appCtx]) error {

	{
		//dir.subs = make(map[amp.TagID]os.DirEntry)
		openDir, err := os.Open(dir.pathname)
		if err != nil {
			return err
		}

		dirItems, err := openDir.Readdir(-1)
		openDir.Close()
		if err != nil {
			return nil
		}

		sort.Slice(dirItems, func(i, j int) bool {
			ii := dirItems[i]
			jj := dirItems[j]
			if ii.IsDir() != jj.IsDir() { // return directories first
				return ii.IsDir()
			}
			return ii.Name() < jj.Name() // then sort by name
		})

		for _, fsInfo := range dirItems {
			if strings.HasPrefix(fsInfo.Name(), ".") {
				continue
			}
			if fsInfo.IsDir() {
				dir := &fsDir{}
				dir.setFrom(fsInfo)
				dir.AddTo(dst, dir)
			} else {
				file := &fsFile{}
				file.setFrom(fsInfo)
				file.AddTo(dst, file)
			}
		}

	}
	return nil
}
