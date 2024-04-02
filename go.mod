module github.com/arcspace/go-archost

go 1.18

// List versions of a module:
//    go list -m -versions -json github.com/git-amp/amp-sdk-go
//
// go list & get cheatsheet:
//    https://stackoverflow.com/a/61312937/3958082
//
// Go Modules Reference:
//    https://www.practical-go-lessons.com/chap-17-go-modules

// replace github.com/git-amp/amp-sdk-go => ../amp-sdk-go

// replace github.com/git-amp/librespot-go => ../librespot-go

require (
	github.com/alecthomas/participle/v2 v2.1.1
	github.com/dgraph-io/badger/v4 v4.2.0
	github.com/git-amp/amp-sdk-go v0.7.5
	github.com/git-amp/librespot-go v0.0.0-20240331174247-aadbe1f80557
	github.com/gogo/protobuf v1.3.2
	github.com/h2non/filetype v1.1.3
	github.com/pkg/errors v0.9.1
	github.com/zmb3/spotify/v2 v2.4.1
	golang.org/x/crypto v0.21.0
	golang.org/x/oauth2 v0.18.0
)

require (
	github.com/brynbellomy/klog v0.0.0-20200414031930-87fbf2e555ae // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgraph-io/ristretto v0.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/golang/glog v1.2.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/flatbuffers v24.3.25+incompatible // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.17.7 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/rs/cors v1.10.1 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/net v0.22.0 // indirect
	golang.org/x/sync v0.6.0 // indirect
	golang.org/x/sys v0.18.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)
