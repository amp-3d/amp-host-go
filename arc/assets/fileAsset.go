package assets

import (
	"os"
	"path/filepath"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/process"
	"github.com/h2non/filetype"
)

type fileAsset struct {
	mediaType string
	pathname  string
}

func GetMediaTypeForExt(pathname string) (mediaType string, extLen int) {
	ext := filepath.Ext(pathname)
	extLen = len(ext)
	if extLen > 0 {
		mediaType = filetype.GetType(ext[1:]).MIME.Value
	}
	return
}

func AssetForFilePathname(pathname, mediaType string) (arc.MediaAsset, error) {
	a := &fileAsset{
		pathname:  pathname,
		mediaType: mediaType,
	}
	if a.mediaType == "" {
		a.mediaType, _ = GetMediaTypeForExt(a.pathname)
	}

	return a, nil
}

func (a *fileAsset) Label() string {
	return a.pathname
}

func (a *fileAsset) MediaType() string {
	return a.mediaType
}

func (a *fileAsset) OnStart(ctx process.Context) error {
	return nil
}

func (a *fileAsset) NewAssetReader() (arc.AssetReader, error) {
	file, err := os.Open(a.pathname)
	if err != nil {
		return nil, err
	}
	return file, nil
}
