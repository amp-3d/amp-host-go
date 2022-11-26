package filesys

import "github.com/arcverse/go-arcverse/pxr"

func NewApp() pxr.App {
	return &fsApp{}
}

const (
	AppBaseName = "filesys"
	AppURI      = "arcverse.systems/filesys.app/v1.2022.1"
)

type DataModel int

const (
	DataModel_nil DataModel = iota
	DirItem
	FileItem
)

var DataModels = []string{
	"",
	"filesys.v1.dir",
	"filesys.v1.file",
}

// AttrURIs
const (
	attr_LastModified  = "modified.DateTime"
	attr_ByteSz        = "size.bytes.int"
	attr_ItemName      = "name.string"
	attr_ThumbGlyphURL = "thumb.glyph.URL"
	attr_BadgeGlyphURL = "badge.glyph.URL"
	attr_MimeType      = "playable.mimetype.string"
)
