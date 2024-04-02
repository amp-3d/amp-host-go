package assets

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/git-amp/amp-sdk-go/amp"
	"github.com/git-amp/amp-sdk-go/stdlib/task"
)

const kAssetLinkPrefix = "/asset/"

type httpServer struct {
	task.Context

	host      task.Context
	opts      HttpServerOpts
	server    *http.Server
	serverMux *http.ServeMux
	assets    map[string]assetEntry
	assetMu   sync.Mutex
	rng       *rand.Rand
}

type assetEntry struct {
	task.Context
	amp.MediaAsset
}

func newHttpServer(opts HttpServerOpts) AssetServer {
	srv := &httpServer{
		opts:      opts,
		serverMux: http.NewServeMux(),
		assets:    map[string]assetEntry{},
	}

	srv.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	srv.rng.Seed(time.Now().UnixNano())

	srv.server = &http.Server{
		Addr:        srv.opts.ListenAddr,
		ReadTimeout: 60 * time.Second,
		IdleTimeout: 1 * time.Hour,
		Handler:     srv.serverMux,
	}

	// format: "asset/{asset_ident}"
	srv.serverMux.HandleFunc(kAssetLinkPrefix, func(w http.ResponseWriter, r *http.Request) {
		assetID := r.URL.Path[len(kAssetLinkPrefix):]

		srv.assetMu.Lock()
		entry := srv.assets[assetID]
		srv.assetMu.Unlock()

		asset := entry.MediaAsset
		if asset == nil {
			http.NotFound(w, r)
			return
		}

		assetReader, err := asset.NewAssetReader()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", asset.MediaType())

		readerCtx, _ := entry.Context.StartChild(&task.Task{
			IdleClose: time.Nanosecond,
			Label:     fmt.Sprintf("AssetReader.(*%v)", reflect.ValueOf(assetReader).Elem().Type().Name()),
			OnRun: func(ctx task.Context) {
				http.ServeContent(w, r, asset.Label(), time.Time{}, assetReader)
			},
			OnClosing: func() {
				assetReader.Close()
			},
		})

		<-readerCtx.Done()

	})

	return srv
}

func (srv *httpServer) StartService(host task.Context) error {
	if srv.host != nil || srv.Context != nil {
		panic("already started")
	}

	lis, err := net.Listen("tcp", srv.opts.ListenAddr)
	if err != nil {
		return err
	}

	srv.host = host
	srv.Context, err = srv.host.StartChild(&task.Task{
		Label: fmt.Sprintf("AssetServer %v", lis.Addr().String()),
		OnRun: func(ctx task.Context) {
			srv.server.Serve(lis)
			ctx.Info(2, "Serve COMPLETE")
		},
		OnClosing: func() {
			if srv.server != nil {
				srv.server.Close()
			}
			// 	srv.Info(1, "Stop")
			// 	go func() {
			// 		srv.server.Shutdown()
			// 		srv.server.Close()
			// 		srv.Info(2, "Stop COMPLETE")
			// 	}()
			// }
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func (srv *httpServer) GracefulStop() {
	if srv.server != nil {
		srv.server.Shutdown(context.Background())
	}
}

func (srv *httpServer) PublishAsset(asset amp.MediaAsset, opts amp.PublishOpts) (URL string, err error) {
	assetID := GenerateAssetID(srv.rng, 28)

	// Extract extension and put on asset name
	{
		mediaType := asset.MediaType()
		if extPos := strings.LastIndexByte(mediaType, '/'); extPos > 0 {
			assetID += "." + mediaType[extPos+1:]
		}
	}

	assetCtx, err := srv.Context.StartChild(&task.Task{
		Label:   asset.Label(),
		OnStart: asset.OnStart,
		OnClosing: func() {
			srv.assetMu.Lock()
			delete(srv.assets, assetID)
			srv.assetMu.Unlock()
		},
	})
	if err != nil {
		return
	}
	if opts.Expiry <= 0 {
		opts.Expiry = srv.opts.IdleExpire
	}
	assetCtx.CloseWhenIdle(opts.Expiry)

	srv.assetMu.Lock()
	srv.assets[assetID] = assetEntry{
		MediaAsset: asset,
		Context:    assetCtx,
	}
	srv.assetMu.Unlock()

	if opts.HostAddr == "" {
		opts.HostAddr = "localhost"
	}

	URL = fmt.Sprintf("http://%s%s%s%s", opts.HostAddr, srv.opts.ListenAddr, kAssetLinkPrefix, assetID)
	return
}

var kAssetChars = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789az") // 64 -> 6 bits

func GenerateAssetID(rng *rand.Rand, numChars int) string {
	s := make([]byte, numChars)
	for i := 0; i < numChars; {
		bits := rng.Int63()
		for j := 0; j < 10 && i < numChars; j++ {
			s[i] = kAssetChars[bits&63]
			i++
			bits >>= 6
		}
	}
	return string(s)
}
