package amp_spotify

import "github.com/arcspace/go-arcspace/arc"

func NewApp() arc.App {
	return &spotifyApp{}
}

const (
	AppBaseName = "spotify"
	AppURI      = "arcspace.systems/spotify.app/v1.2023.1"
)
