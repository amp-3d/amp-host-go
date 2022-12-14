package vibe

import "github.com/arcspace/go-arcspace/pxr"

const (
	AppURI = "vibe.pxr.tools/vibe.app/v1.2022.1"

	// AttrModelURIs
	PinAppHome     = "pin/app/home"
	pinAppSettings = "pin/home/settings"
	pinAppFiles    = "pin/home/files" //invokes hfs app
	pinAppStations = "pin/home/stations"

	// childPlayable = "child/vibe/playable"
	// childPlaylist = "child/vibe/playlist"
)

func NewApp() pxr.App {
	return &vibeApp{}
}
