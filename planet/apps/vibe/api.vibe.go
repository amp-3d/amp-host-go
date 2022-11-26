package vibe

import "github.com/arcverse/go-arcverse/planet"

const (
	AppURI = "vibe.planet.tools/vibe.app/v1.2022.1"

	// DataModelURIs
	PinAppHome     = "pin/app/home"
	pinAppSettings = "pin/home/settings"
	pinAppFiles    = "pin/home/files" //invokes hfs app
	pinAppStations = "pin/home/stations"

	// childPlayable = "child/vibe/playable"
	// childPlaylist = "child/vibe/playlist"
)

func NewApp() planet.App {
	return &vibeApp{}
}
