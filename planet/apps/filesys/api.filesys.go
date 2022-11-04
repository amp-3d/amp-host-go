package filesys

import "github.com/genesis3systems/go-planet/planet"

const (
	AppBaseName = "filesys"
	AppURI      = "planet.tools/filesys.app/v1.2022.1"

	// DataModelURIs
	PinDir  = "pin/filesys/dir"
	PinFile = "pin/filesys/file"
	PinItem = "pin/filesys/item"

	childPlayable = "child/filesys/playable"
	childPlaylist = "child/filesys/playlist"
)

func NewApp() planet.App {
	return &fsApp{}
}
