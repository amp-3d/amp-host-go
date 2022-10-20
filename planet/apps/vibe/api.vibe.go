package vibe

import "github.com/genesis3systems/go-planet/planet"

const AppURI = "vibe.planet.tools/vibe.app"

func NewApp() planet.App {
    return &vibeApp{}
}
