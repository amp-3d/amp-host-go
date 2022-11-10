package main

import (
	"flag"
	"os"
	"path"
	"time"

	"github.com/brynbellomy/klog"
	"github.com/genesis3systems/go-cedar/log"
	"github.com/genesis3systems/go-cedar/process"
	"github.com/genesis3systems/go-cedar/utils"
	"github.com/genesis3systems/go-planet/planet"
	"github.com/genesis3systems/go-planet/planet/apps/filesys"
	"github.com/genesis3systems/go-planet/planet/apps/vibe"
	"github.com/genesis3systems/go-planet/planet/grpc_server"
	"github.com/genesis3systems/go-planet/planet/host"
)

func main() {

	exePath, err := utils.GetExePath()
	if err != nil {
		log.Fatalf("%v", err)
	}
	defaultDataDir := path.Join(exePath, "phost.data")

	hostPort := flag.Int("port", int(planet.Const_DefaultGrpcServicePort), "Sets the port used to bind HostGrpc service")
	dataDir := flag.String("data", defaultDataDir, "Specifies the path for all file access and storage")

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
		BasePath: *dataDir,
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

	host.Infof(0, "Graceful stop: \x1b[1m^C\x1b[0m or \x1b[1mkill -s SIGINT %d\x1b[0m", /*, or \x1b[1mkill -9 %d\x1b[0m", os.Getpid(),*/ os.Getpid())

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

	go func() {
		ticker := time.NewTicker(time.Minute)
		debugAbort := int(0)
		for debugAbort == 0 {
			tick := <-ticker.C
			process.PrintContextTree(host, nil, 2)
			if tick.IsZero() {
				debugAbort = 1
			}
		}

		host.Close()
	}()
	
	// Block on grpc shutdown completion, then initiate host shutdown.
	<-srv.Done()
	host.Close()
	
	// Block on host shutdown completion
	<-host.Done()
	
	klog.Flush()
}
