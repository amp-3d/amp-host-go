package main

import (
	"flag"
	"path"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/log"
	"github.com/arcspace/go-arc-sdk/stdlib/task"
	"github.com/arcspace/go-arc-sdk/stdlib/utils"
	"github.com/arcspace/go-archost/arc/archost"
	"github.com/arcspace/go-archost/arc/grpc_service"
)

func main() {

	exePath, err := utils.GetExePath()
	if err != nil {
		log.Fatalf("%v", err)
	}
	defaultDataPath := path.Join(exePath, "archost.data")

	hostPort := flag.Int("host-port", int(arc.Const_DefaultGrpcServicePort), "Sets the port used to bind HostGrpc service")
	assetPort := flag.Int("asset-port", int(arc.Const_DefaultGrpcServicePort+1), "Sets the port used for serving pinned assets")
	debugMode := flag.Bool("debug", false, "Enables debug mode")
	showTree := flag.Int("show-tree", 0, "Prints the task tree periodically, checking every given number of seconds")
	dataPath := flag.String("data-path", defaultDataPath, "Specifies the path for all file access and storage")
	flag.Set("logtostderr", "true")
	flag.Set("v", "2")

	fset := flag.NewFlagSet("", flag.ContinueOnError)
	log.InitFlags(fset)
	fset.Set("logtostderr", "true")
	fset.Set("v", "2")
	log.UseStockFormatter(24, true)

	flag.Parse()

	hostOpts := archost.DefaultOpts(*assetPort, *debugMode)
	hostOpts.StatePath = *dataPath
	host, err := archost.StartNewHost(hostOpts)
	if err != nil {
		log.Fatalf("failed to start new host: %v", err)
	}

	opts := grpc_service.DefaultGrpcServerOpts(*hostPort)
	srv := opts.NewGrpcServer()
	err = srv.StartService(host)
	if err != nil {
		srv.Fatalf("failed to start grpc service: %v", err)
	}

	gracefulStop, immediateStop := log.AwaitInterrupt()

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
		go task.PrintTreePeriodically(host, time.Duration(*showTree)*time.Second, 2)
	}

	// Block on grpc shutdown completion, then initiate host shutdown.
	<-srv.Done()
	host.Close()

	// Block on host shutdown completion
	<-host.Done()

	log.Flush()
}
