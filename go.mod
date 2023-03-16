module github.com/arcspace/go-arcspace

go 1.18

// List versions of a module:
//    go list -m -versions github.com/arcspace/go-cedar
//
// go list & get cheatsheet:
//    https://stackoverflow.com/a/61312937/3958082

//replace github.com/arcspace/go-cedar => ../go-cedar

require (
	github.com/arcspace/go-cedar v1.2022.3
	github.com/brynbellomy/klog v0.0.0-20200414031930-87fbf2e555ae
	github.com/dgraph-io/badger/v4 v4.0.1
	github.com/gogo/protobuf v1.3.2
	github.com/pkg/errors v0.9.1
	github.com/zmb3/spotify/v2 v2.3.1
	golang.org/x/crypto v0.7.0
	google.golang.org/grpc v1.53.0
)

require (
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgraph-io/ristretto v0.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/golang/glog v1.1.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/flatbuffers v23.3.3+incompatible // indirect
	github.com/h2non/filetype v1.1.3
	github.com/klauspost/compress v1.16.3 // indirect
	github.com/rs/cors v1.8.3 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/oauth2 v0.6.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230306155012-7f2fa6fef1f4 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
)
