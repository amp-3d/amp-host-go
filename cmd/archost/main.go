package main

import (
	"flag"
	"path"
	"time"

	"github.com/amp-3d/amp-host-go/amp/host"
	"github.com/amp-3d/amp-host-go/amp/host/tcp_service"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/stdlib/log"
	"github.com/amp-3d/amp-sdk-go/stdlib/task"
	"github.com/amp-3d/amp-sdk-go/stdlib/utils"
)

func main() {

	exePath, err := utils.GetExePath()
	if err != nil {
		log.Fatalf("%v", err)
	}
	defaultDataPath := path.Join(exePath, "archost.data")

	hostPort := flag.Int("host-port", int(amp.Const_DefaultServicePort), "Sets the port used to bind tcp service")
	assetPort := flag.Int("asset-port", int(amp.Const_DefaultServicePort+1), "Sets the port used for serving pinned assets")
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

	hostOpts := host.DefaultOpts(*assetPort, *debugMode)
	hostOpts.StatePath = *dataPath
	host, err := host.StartNewHost(hostOpts)
	if err != nil {
		log.Fatalf("failed to start new host: %v", err)
	}

	opts := tcp_service.DefaultTcpServerOpts(*hostPort)
	srv := opts.NewTcpServer()
	err = srv.StartService(host)
	if err != nil {
		srv.Fatalf("failed to start tcp service: %v", err)
	}

	gracefulStop, immediateStop := log.AwaitInterrupt()

	go func() {
		<-gracefulStop
		srv.Info(1, "<-gracefulStop")
		srv.GracefulStop()
		srv.Close()
	}()

	go func() {
		<-immediateStop
		srv.Info(1, "<-immediateStop")
		srv.Close()
	}()

	if *showTree > 0 {
		go task.PrintTreePeriodically(host, time.Duration(*showTree)*time.Second, 2)
	}

	// Block on tcp shutdown completion, then initiate host shutdown.
	<-srv.Done()
	host.Close()

	// Block on host shutdown completion
	<-host.Done()

	log.Flush()
}
