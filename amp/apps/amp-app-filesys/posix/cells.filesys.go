package posix

import (
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/amp-3d/amp-host-go/amp/apps/amp-app-av/av"
	"github.com/amp-3d/amp-host-go/amp/assets"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/basic"
	"github.com/amp-3d/amp-sdk-go/stdlib/media"
	"github.com/amp-3d/amp-sdk-go/stdlib/tag"
)

type fsItem struct {
	basic.CellInfo[*appInst]

	basename    string // base file name
	dirname     string // non-nil when pinned (could be alternative OS handle)
	mode        os.FileMode
	size        int64
	isDir       bool
	modTime     time.Time
	contentType string

	mediaFlags av.MediaFlags
}

func (item *fsItem) pathname() string {
	return path.Join(item.dirname, item.basename)
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

func newFsItem(dirname string, fi os.FileInfo) basic.Cell[*appInst] {

	item := fsItem{
		dirname:  dirname,
		basename: fi.Name(),
		mode:     fi.Mode(),
		modTime:  fi.ModTime(),
		isDir:    fi.IsDir(),
	}
	//item.ID = tag.FromString(item.pathname())

	stripExt := false

	extLen := 0
	if item.isDir {
		item.Tab.Tags = []*amp.Tag{
			amp.GenericFolderGlyph,
			amp.PinnableCatalog,
		}
	} else {

		item.size = fi.Size()
		item.contentType, extLen = assets.GetContentTypeForExt(item.basename)
		item.Tab.AddPinnableContentTag(item.contentType)

		// TODO: make smarter
		switch {
		case strings.HasPrefix(item.contentType, "audio/"):
			item.mediaFlags |= av.MediaFlags_HasAudio
			stripExt = true
		case strings.HasPrefix(item.contentType, "video/"):
			item.mediaFlags |= av.MediaFlags_HasVideo
			stripExt = true
		}
		item.mediaFlags |= av.MediaFlags_IsSeekable
	}

	//////////////////  TagTab
	{
		item.Tab.SetModifiedAt(item.modTime)
		base := item.basename
		if stripExt {
			base = item.basename[:len(base)-extLen]
		}
		splitAt := strings.LastIndex(base, " - ")
		if splitAt > 0 {
			item.Tab.Label = base[splitAt+3:]
			item.Tab.Caption = base[:splitAt]
		} else {
			item.Tab.Label = base
		}
	}

	if item.isDir {
		dir := &fsDir{
			fsItem: item,
		}
		return dir
	} else {
		file := &fsFile{
			fsItem: item,
		}
		return file
	}

}

type fsFile struct {
	fsItem
	contentURL string
}

func (item *fsFile) MarshalAttrs(pin *basic.Pin[*appInst]) {
	item.fsItem.MarshalAttrs(pin)

	if item.contentURL == "" {
		return
	}

	if item.mediaFlags != 0 {
		playable := av.PlayableMedia{
			Flags: item.mediaFlags,
			Media: &amp.TagTab{
				Label:   item.Tab.Label,
				Caption: item.Tab.Caption,
				Tags: []*amp.Tag{
					{
						ContentType: item.contentType,
						URL:         item.contentURL,
					},
				},
			},
		}
		pin.Upsert(item.ID, av.PlayableMediaSpec.ID, tag.Nil, &playable)
	} else {
		// TODO: 'ContentStream' should be sent instead since we want to sent any file
	}
}

func (item *fsFile) PinInto(pin *basic.Pinned[*appInst]) error {
	asset, err := assets.AssetForFilePathname(item.pathname(), "")
	if err != nil {
		return err
	}
	item.contentURL, err = pin.App.PublishAsset(asset, media.PublishOpts{
		HostAddr: pin.App.Session().Auth().HostAddr,
	})
	return err
}

type fsDir struct {
	fsItem
}

// reads the fsDir's catalog and issues new items as needed.
func (dir *fsDir) PinInto(pin *basic.Pinned[*appInst]) error {

	{
		//dir.subs = make(map[tag.ID]os.DirEntry)
		openDir, err := os.Open(dir.pathname())
		if err != nil {
			return err
		}

		//
		// TODO
		//
		// Read ".amp/amp.posix.tags.csv", allowing persistent tag IDs
		//

		subItems, err := openDir.Readdir(-1)
		openDir.Close()
		if err != nil {
			return nil
		}

		sort.Slice(subItems, func(i, j int) bool {
			ii := subItems[i]
			jj := subItems[j]
			if ii.IsDir() != jj.IsDir() { // return directories first
				return ii.IsDir()
			}
			return ii.Name() < jj.Name() // then sort by name
		})

		for _, fsInfo := range subItems {
			if strings.HasPrefix(fsInfo.Name(), ".") {
				continue
			}
			cell := newFsItem(dir.dirname, fsInfo)
			pin.AddChild(cell)

		}

	}
	return nil
}
