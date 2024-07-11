package posix

import (
	"os"
	"path"
	"sort"
	"strings"

	"github.com/amp-3d/amp-host-go/amp/apps/amp.app.av/av"
	"github.com/amp-3d/amp-host-go/amp/assets"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/std"
	"github.com/amp-3d/amp-sdk-go/stdlib/media"
)

type fsItem struct {
	std.CellNode[*appInst]
	std.FSInfo

	ordering   float32
	dirname    string
	mediaFlags av.MediaFlags
	contentURL string
}

func (item *fsItem) pathname() string {
	return path.Join(item.dirname, item.Name)
}

func (item *fsItem) Compare(oth *fsItem) int {
	// if item.isDir != oth.isDir {
	// 	return int(item.isDir) - int(oth.isDir)
	// }
	if diff := strings.Compare(item.Name, oth.Name); diff != 0 {
		return diff
	}
	if diff := item.ModifiedAt - oth.ModifiedAt; diff != 0 {
		return int(diff >> 31)
	}
	// if diff := int(item.) - int(oth.mode); diff != 0 {
	// 	return diff
	// }
	if diff := item.ByteSize - oth.ByteSize; diff != 0 {
		return int(diff >> 31)
	}
	return 0
}

// ??/
func (item *fsItem) GetLogLabel() string {
	label := item.Name
	if item.IsDir {
		label += "/"
	}
	return label
}

func newFsItem(dirname string, fi os.FileInfo) *fsItem {

	item := &fsItem{
		dirname: dirname,
		FSInfo: std.FSInfo{
			Name:     fi.Name(),
			Mode:     fi.Mode().String(),
			IsDir:    fi.IsDir(),
			ByteSize: fi.Size(),
		},
	}
	item.SetModifiedAt(fi.ModTime())

	//item.ID = tag.FromString(item.pathname())

	extLen := 0
	item.ContentType, extLen = assets.GetContentTypeForExt(item.Name)
	item.NameLen = int32(len(item.Name) - extLen)

	if !item.IsDir {
		// TODO: make smarter
		switch {
		case strings.HasPrefix(item.ContentType, "audio/"):
			item.mediaFlags |= av.MediaFlags_HasAudio
		case strings.HasPrefix(item.ContentType, "video/"):
			item.mediaFlags |= av.MediaFlags_HasVideo
		}
		item.mediaFlags |= av.MediaFlags_IsSeekable
	}

	return item
}

func (item *fsItem) PinInto(pin *std.Pin[*appInst]) error {
	if item.IsDir {
		return item.pinAsDir(pin)
	} else {
		return item.pinAsFile(pin)
	}
}

func (item *fsItem) pinAsFile(pin *std.Pin[*appInst]) error {
	if item.contentURL != "" {
		return nil
	}

	asset, err := assets.AssetForFilePathname(item.pathname(), "")
	if err != nil {
		return err
	}
	item.contentURL, err = pin.App.PublishAsset(asset, media.PublishOpts{
		HostAddr: pin.App.Session().LoginInfo().HostAddr,
		OnExpired: func() {
			item.contentURL = ""
		},
	})
	return err
}

// reads the fsDir's catalog and issues new items as needed.
func (item *fsItem) pinAsDir(pin *std.Pin[*appInst]) error {

	{
		dirName := item.pathname()
		openDir, err := os.Open(dirName)
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

		for i, fsInfo := range subItems {
			if strings.HasPrefix(fsInfo.Name(), ".") {
				continue
			}
			cell := newFsItem(dirName, fsInfo)
			cell.ordering = float32(i)
			pin.AddChild(cell)
		}

	}
	return nil
}

func (item *fsItem) MarshalAttrs(w std.CellWriter) {
	w.PutItem(std.CellFileInfo, &item.FSInfo)

	var (
		label string
		group string
	)
	base := item.Name[:item.NameLen]
	splitAt := strings.LastIndex(base, " - ")
	if splitAt > 0 {
		group = base[:splitAt]
		label = base[splitAt+3:]
	} else {
		label = base
	}
	w.PutText(std.CellLabel, label)
	if group != "" {
		w.PutText(std.CellCaption, group)
	}

	{
		if item.IsDir {
			w.PutItem(std.CellGlyphs, std.GenericFolderGlyph)
		} else {
			w.PutItem(std.CellGlyphs, &amp.Tag{
				Use: amp.TagUse_Glyph,
				URL: std.GenericGlyphURL + item.ContentType,
			})
		}
	}

	if item.mediaFlags != 0 {
		if group != "" {
			w.PutText(std.CellCollection, group)
		}
		w.PutItem(av.MediaTrackInfoID, &av.MediaTrackInfo{
			Flags: item.mediaFlags,
		})

	}

	if item.contentURL != "" {
		w.PutItem(std.CellContentLink, &amp.Tag{
			URL:         item.contentURL,
			ContentType: item.ContentType,
		})
	}

	if item.contentURL == "" {
		return
	}

}
