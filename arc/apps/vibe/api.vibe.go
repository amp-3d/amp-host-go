package vibe

import "github.com/arcspace/go-arcspace/arc"

const (
	AppURI = "vibe.arc.tools/vibe.app/v1.2022.1"

	// AttrModelURIs
	PinAppHome     = "pin/app/home"
	pinAppSettings = "pin/home/settings"
	pinAppFiles    = "pin/home/files" //invokes hfs app
	pinAppStations = "pin/home/stations"

	// childPlayable = "child/vibe/playable"
	// childPlaylist = "child/vibe/playlist"
)

func NewApp() arc.App {
	return &vibeApp{}
}
