package filesys

import "github.com/arcspace/go-arcspace/arc"

func NewApp() arc.App {
	return &fsApp{}
}

const (
	AppBaseName = "filesys"
	AppURI      = "arcspace.systems/filesys.app/v1.2022.1"

	// KwArg names
	KwArg_PinPath = "pin-path"

	// CellDataModels
	CellDataModel_Dir  = "filesys.dir.v1.model"
	CellDataModel_File = "filesys.file.v1.model"
)

// AttrURIs
const (
	attr_LastModified = "modified.DateTime"
	attr_ByteSz       = "size.bytes.int"
	attr_ItemName     = "name.string"
	attr_MimeType     = "mimetype.string"
	attr_Pathname     = "pathname.string"
)
