package bs

import "github.com/arcspace/go-arcspace/arc"

func NewApp() arc.App {
	return &bsApp{}
}

const (
	AppURI = "arcspace.systems/bookmark-service.app/v1.2023.1"
)
