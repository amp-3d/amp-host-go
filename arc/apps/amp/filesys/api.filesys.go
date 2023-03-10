package filesys

import "github.com/arcspace/go-arcspace/arc"

func NewApp() arc.App {
	return &fsApp{}
}

const (
	AppBaseName = "filesys"
	AppURI      = "arcspace.systems/filesys.app/v1.2022.1"
)
