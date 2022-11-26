package main

import (
	"bytes"
	"flag"
	"os"
	"path"
	"time"

	"github.com/arcverse/go-arcverse/planet"
	"github.com/arcverse/go-arcverse/planet/apps/filesys"
	"github.com/arcverse/go-arcverse/planet/apps/vibe"
	"github.com/arcverse/go-arcverse/planet/grpc_server"
	"github.com/arcverse/go-arcverse/planet/host"
	"github.com/arcverse/go-cedar/log"
	"github.com/arcverse/go-cedar/process"
	"github.com/arcverse/go-cedar/utils"
	"github.com/brynbellomy/klog"
)

func main() {

	exePath, err := utils.GetExePath()
	if err != nil {
		log.Fatalf("%v", err)
	}
	defaultDataPath := path.Join(exePath, "phost.data")

	hostPort := flag.Int("host-port", int(planet.Const_DefaultGrpcServicePort), "Sets the port used to bind HostGrpc service")
	showTree := flag.Int("show-tree", 0, "Prints the process tree periodically, checking every given number of seconds")
	dataPath := flag.String("data-path", defaultDataPath, "Specifies the path for all file access and storage")

	flag.Set("logtostderr", "true")
	flag.Set("v", "2")

	fset := flag.NewFlagSet("", flag.ContinueOnError)
	klog.InitFlags(fset)
	fset.Set("logtostderr", "true")
	fset.Set("v", "2")
	klog.SetFormatter(&klog.FmtConstWidth{
		FileNameCharWidth: 24,
		UseColor:          true,
	})

	klog.Flush()
	flag.Parse()

	params := host.HostOpts{
		BasePath: *dataPath,
	}

	host, err := host.StartNewHost(params)
	if err != nil {
		log.Fatalf("failed to start new host: %v", err)
	}

	host.RegisterApp(vibe.NewApp())
	host.RegisterApp(filesys.NewApp())

	opts := grpc_server.DefaultGrpcServerOpts(*hostPort)
	srv, err := opts.AttachNewGrpcServer(host)
	if err != nil {
		srv.Fatalf("failed to start grpc service: %v", err)
	}

	gracefulStopSignal, immediateStopSignal := log.AwaitInterrupt()

	host.Infof(0, "Graceful stop: \x1b[1m^C\x1b[0m or \x1b[1mkill -s SIGINT %d\x1b[0m" /*, or \x1b[1mkill -9 %d\x1b[0m", os.Getpid(),*/, os.Getpid())

	go func() {
		<-gracefulStopSignal
		srv.Info(2, "<-gracefulStopSignal")
		srv.GracefulStop()
		srv.Close()
	}()

	go func() {
		<-immediateStopSignal
		srv.Info(2, "<-immediateStopSignal")
		srv.Close()
	}()

	if *showTree > 0 {
		go PrintTreePeriodically(host, time.Duration(*showTree)*time.Second, 2)
	}

	// Block on grpc shutdown completion, then initiate host shutdown.
	<-srv.Done()
	host.Close()

	// Block on host shutdown completion
	<-host.Done()

	klog.Flush()
}

func PrintTreePeriodically(ctx process.Context, period time.Duration, verboseLevel int32) {
	block := [32]byte{}
	var text []byte
	buf := bytes.Buffer{}
	buf.Grow(256)

	ticker := time.NewTicker(period)
	for running := true; running; {
		select {
		case <-ticker.C:
			{
				process.PrintContextTree(ctx, &buf, verboseLevel)
				R := buf.Len()
				if R != len(text) {
					if cap(text) < R {
						text = make([]byte, R, (R+0x1FF)&^0x1FF)
					} else {
						text = text[:R]
					}
				}
				same := true
				for pos := 0; pos < R; {
					n, _ := buf.Read(block[:])
					if n == 0 {
						break
					}
					if same {
						same = bytes.Equal(block[:n], text[pos:pos+n])
					}
					if !same {
						copy(text[pos:], block[:n])
					}
					pos += n
				}
				if !same {
					ctx.Info(verboseLevel, string(text[:R]))
				}
				buf.Reset()
			}
		case <-ctx.Closing():
			running = false
		}
	}
	ticker.Stop()
}
