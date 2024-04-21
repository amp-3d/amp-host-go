package main

import (
	"flag"
	"fmt"
	"testing"

	"github.com/amp-3d/amp-sdk-go/stdlib/log"
)

func TestMe(t *testing.T) {
	flag.Set("logtostderr", "true")
	flag.Set("v", "2")

	fset := flag.NewFlagSet("", flag.ContinueOnError)
	log.InitFlags(fset)
	fset.Set("logtostderr", "true")
	fset.Set("v", "2")
	log.UseStockFormatter(24, true)

	code := Call_SessionBegin("/Users/aomeara/Desktop", "/Users/aomeara/Desktop")
	fmt.Println("Call_SessionBegin: ", code)

	var msgBuf []byte
	code = Call_WaitOnMsg(&msgBuf)

	fmt.Println("Call_WaitOnMsg: ", code)

}
