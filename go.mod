module github.com/amp-3d/amp-host-go

go 1.22

// List versions of a module:
//    go list -m -versions -json github.com/amp-3d/amp-sdk-go
//
// go list & get cheatsheet:
//    https://stackoverflow.com/a/61312937/3958082
//
// Go Modules Reference:
//    https://www.practical-go-lessons.com/chap-17-go-modules

// replace github.com/amp-3d/amp-sdk-go => ../amp-sdk-go

// replace github.com/amp-3d/amp-librespot-go => ../amp-librespot-go

require (
	github.com/ajstarks/svgo v0.0.0-20211024235047-1546f124cd8b
	github.com/amp-3d/amp-librespot-go v0.0.0-20240711142803-f90549da09cd
	github.com/amp-3d/amp-sdk-go v0.8.1
	github.com/dgraph-io/badger/v4 v4.2.0
	github.com/gocarina/gocsv v0.0.0-20240520201108-78e41c74b4b1
	github.com/gogo/protobuf v1.3.2
	github.com/h2non/filetype v1.1.3
	github.com/llgcode/draw2d v0.0.0-20240627062922-0ed1ff131195
	github.com/pkg/errors v0.9.1
	github.com/slack-go/slack v0.13.1
	github.com/zmb3/spotify/v2 v2.4.2
	golang.org/x/crypto v0.25.0
	golang.org/x/oauth2 v0.21.0
	xojoc.pw/useragent v0.0.0-20200116211053-1ec61d55e8fe
)

require (
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/brynbellomy/klog v0.0.0-20200414031930-87fbf2e555ae // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgraph-io/ristretto v0.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/golang/glog v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/flatbuffers v24.3.25+incompatible // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/rs/cors v1.11.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/image v0.18.0 // indirect
	golang.org/x/net v0.27.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/sys v0.22.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)
