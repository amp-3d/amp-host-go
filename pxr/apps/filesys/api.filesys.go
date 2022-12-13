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
	"filesys.dir.v1",
	"filesys.file.v1",
}

// AttrURIs
const (
	attr_LastModified  = "modified.DateTime"
	attr_ByteSz        = "size.bytes.int"
	attr_ItemName      = "name.string"
	attr_MimeType      = "mimetype.string"
	attr_Pathname      = "pathname.string"
)
