package assets

import (
	"os"
	"path/filepath"

	"github.com/amp-3d/amp-sdk-go/stdlib/media"
	"github.com/amp-3d/amp-sdk-go/stdlib/task"
	"github.com/h2non/filetype"
)

type fileAsset struct {
	contentType string
	pathname    string
}

func GetContentTypeForExt(pathname string) (contentType string, extLen int) {
	ext := filepath.Ext(pathname)
	extLen = len(ext)
	if extLen > 0 {
		contentType = filetype.GetType(ext[1:]).MIME.Value
	}
	return
}

func AssetForFilePathname(pathname, contentType string) (media.Asset, error) {
	a := &fileAsset{
		pathname:    pathname,
		contentType: contentType,
	}
	if a.contentType == "" {
		a.contentType, _ = GetContentTypeForExt(a.pathname)
	}

	return a, nil
}

func (a *fileAsset) Label() string {
	return a.pathname
}

func (a *fileAsset) ContentType() string {
	return a.contentType
}

func (a *fileAsset) OnStart(ctx task.Context) error {
	return nil
}

func (a *fileAsset) NewAssetReader() (media.AssetReader, error) {
	file, err := os.Open(a.pathname)
	if err != nil {
		return nil, err
	}
	return file, nil
}
