package main

import (
	"flag"
	"os"
	"path"
	"time"

	"github.com/arcspace/go-arcspace/arc"
	"github.com/arcspace/go-arcspace/arc/archost"
	"github.com/arcspace/go-arcspace/arc/grpc_service"
	"github.com/arcspace/go-arcspace/arc/host"
	"github.com/arcspace/go-cedar/log"
	"github.com/arcspace/go-cedar/process"
	"github.com/arcspace/go-cedar/utils"
)

func main() {

	exePath, err := utils.GetExePath()
	if err != nil {
		log.Fatalf("%v", err)
	}
	defaultDataPath := path.Join(exePath, "archost.data")

	hostPort := flag.Int("host-port", int(arc.Const_DefaultGrpcServicePort), "Sets the port used to bind HostGrpc service")
	showTree := flag.Int("show-tree", 0, "Prints the process tree periodically, checking every given number of seconds")
	dataPath := flag.String("data-path", defaultDataPath, "Specifies the path for all file access and storage")

	flag.Set("logtostderr", "true")
	flag.Set("v", "2")

	fset := flag.NewFlagSet("", flag.ContinueOnError)
	log.InitFlags(fset)
	fset.Set("logtostderr", "true")
	fset.Set("v", "2")
	log.UseStockFormatter(24, true)

	flag.Parse()

	hostOpts := host.DefaultHostOpts()
	hostOpts.StatePath = *dataPath
	host := archost.StartNewHost(hostOpts)

	opts := grpc_service.DefaultGrpcServerOpts(*hostPort)
	srv := opts.NewGrpcServer()
	err = srv.StartService(host)
	if err != nil {
		srv.Fatalf("failed to start grpc service: %v", err)
	}

	gracefulStop, immediateStop := log.AwaitInterrupt()

	host.Infof(0, "Graceful stop: \x1b[1m^C\x1b[0m or \x1b[1mkill -s SIGINT %d\x1b[0m" /*, or \x1b[1mkill -9 %d\x1b[0m", os.Getpid(),*/, os.Getpid())

	go func() {
		<-gracefulStop
		srv.Info(2, "<-gracefulStop")
		srv.GracefulStop()
		srv.Close()
	}()

	go func() {
		<-immediateStop
		srv.Info(2, "<-immediateStop")
		srv.Close()
	}()

	if *showTree > 0 {
		go process.PrintTreePeriodically(host, time.Duration(*showTree)*time.Second, 2)
	}

	// Block on grpc shutdown completion, then initiate host shutdown.
	<-srv.Done()
	host.Close()

	// Block on host shutdown completion
	<-host.Done()

	log.Flush()
}
