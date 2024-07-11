// go/cmd/libarchost/main.go
package main

import "C"

import (
	"flag"
	"fmt"
	"io"

	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/stdlib/log"

	"github.com/amp-3d/amp-host-go/amp/host"
	"github.com/amp-3d/amp-host-go/amp/host/lib_service"
)

var (
	gLibSession lib_service.LibSession
	gLibService lib_service.LibService
	gHost       amp.Host
)

//export Call_SessionBegin
func Call_SessionBegin(userDataPath, sharedCachePath string) int64 {
	if gLibSession != nil {
		return 0
	}

	debug := false
	if debug {
		fset := flag.NewFlagSet("", flag.ContinueOnError)
		log.InitFlags(fset)
		log.UseStockFormatter(24, false)
		fset.Set("v", "3")
		fset.Set("logtostderr", "false")
		fset.Set("log_file", "/Users/aomeara/Desktop/_archost.log.txt")
	}

	// Copy the param strings since they will be invalid after exits
	hostOpts := host.DefaultOpts(0, debug)
	hostOpts.CachePath = string(append([]byte{}, sharedCachePath...))
	hostOpts.StatePath = string(append([]byte{}, userDataPath...))

	var err error
	gHost, err = host.StartNewHost(hostOpts)
	if err != nil {
		log.Fatalf("failed to start new host: %v", err)
		return 0
	}

	opts := lib_service.DefaultLibServiceOpts()
	gLibService = opts.NewLibService()
	err = gLibService.StartService(gHost)
	if err == nil {
		gLibSession, err = gLibService.NewLibSession()
	}

	if err != nil {
		errMsg := fmt.Sprintf("failed to start LibService: %v", err)
		gHost.Log().Error(errMsg)
		gHost.Close()
		return 0
	}

	return int64(12345)
}

//export Call_SessionEnd
func Call_SessionEnd(sessID int64) {
	sess := gLibSession
	if sess == nil {
		return
	}

	sess.Close()
	gLibSession = nil
}

//export Call_Shutdown
func Call_Shutdown() {
	srv := gLibService
	if srv == nil {
		return
	}

	gLibService = nil
	gLibSession = nil

	// Closing the host will cause the lib server to detach
	gHost.Close()
	<-gHost.Done()
}

//export Call_PushMsg
func Call_PushMsg(txBuf []byte) int64 {
	sess := gLibSession
	if sess == nil {
		return -1
	}

	r := bufReader{
		buf: txBuf,
	}
	tx, err := amp.ReadTxMsg(&r)
	if err != nil {
		return -6665
	}
	sess.EnqueueIncoming(tx)
	return 0
}

//export Call_WaitOnMsg
func Call_WaitOnMsg(txBuf *[]byte) int64 {
	sess := gLibSession
	if sess == nil {
		return -1
	}

	err := sess.DequeueOutgoing(txBuf)
	if err == nil {
		return 0
	}

	arcErr, ok := err.(*amp.Err)
	if ok {
		return int64(arcErr.Code)
	} else {
		return -1
	}
}

//export Call_Realloc
func Call_Realloc(buf *[]byte, newLen int64) int64 {
	sess := gLibSession
	if sess == nil {
		return -1
	}

	sess.Realloc(buf, newLen)
	return 0
}

type UnityRenderingExtEventType int32

const (
	kUnityRenderingExtEventSetStereoTarget     UnityRenderingExtEventType = iota // issued during SetStereoTarget and carrying the current 'eye' index as parameter
	kUnityRenderingExtEventSetStereoEye                                          // issued during stereo rendering at the beginning of each eye's rendering loop. It carries the current 'eye' index as parameter
	kUnityRenderingExtEventStereoRenderingDone                                   // issued after the rendering has finished
	kUnityRenderingExtEventBeforeDrawCall                                        // issued during BeforeDrawCall and carrying UnityRenderingExtBeforeDrawCallParams as parameter
	kUnityRenderingExtEventAfterDrawCall                                         // issued during AfterDrawCall. This event doesn't carry any parameters
	kUnityRenderingExtEventCustomGrab                                            // issued during GrabIntoRenderTexture since we can't simply copy the resources
	//      when custom rendering is used - we need to let plugin handle this. It carries over
	//      a UnityRenderingExtCustomBlitParams params = { X, source, dest, 0, 0 } ( X means it's irrelevant )
	kUnityRenderingExtEventCustomBlit // issued by plugin to insert custom blits. It carries over UnityRenderingExtCustomBlitParams as param.
	kUnityRenderingExtEventUpdateTextureBegin
	kUnityRenderingExtEventUpdateTextureEnd

	// keep this last
	kUnityRenderingExtEventCount
	kUnityRenderingExtUserEventsStart = kUnityRenderingExtEventCount
)

type FrameBuf struct {
	width       int32
	height      int32
	bytesPerRow int32
	pixels      []byte
}

var curX int32
var curY int32

var gCtx []FrameBuf

//export CreateCtx
func CreateCtx() int32 {
	idx := len(gCtx)
	gCtx = append(gCtx, FrameBuf{})

	curX = 0
	curY = 0

	return int32(idx)
}

//export ResizeCtx
func ResizeCtx(ctxID int32, width int32, height int32, bytesPerRow int32, buf []byte) {
	gCtx[ctxID] = FrameBuf{
		width:       width,
		height:      height,
		bytesPerRow: bytesPerRow,
		pixels:      buf,
	}
}

//export TextureUpdateCallback
func TextureUpdateCallback(eventID UnityRenderingExtEventType, data int64) {

	switch eventID {
	case kUnityRenderingExtEventUpdateTextureBegin:
	case kUnityRenderingExtEventUpdateTextureEnd:
		break
	}
}

//export GetTextureUpdateCallback
func GetTextureUpdateCallback() func(eventID UnityRenderingExtEventType, data int64) {
	return TextureUpdateCallback
}

//export RenderFrame
func RenderFrame(fbID int32) {
	frame := gCtx[fbID]
	if curX > frame.width {
		curX = 0
		curY = (curY + 1) % frame.height
	} else {
		curX++
	}

	offset := (int32)(curX*4 + curY*frame.bytesPerRow)
	// b32 := &(frame.pixels[offset])
	// *b32 = 0x7F3F8F7F;
	frame.pixels[offset] = 0xFF
	frame.pixels[offset+1] = frame.pixels[offset+1] + 2
	frame.pixels[offset+2] = frame.pixels[offset+2] + 2
	frame.pixels[offset+3] = 0xFF
}

func main() {

}

type bufReader struct {
	buf []byte
	pos int
}

func (r *bufReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.buf) {
		return 0, io.EOF
	}
	n = copy(p, r.buf[r.pos:])
	r.pos += n
	return n, nil
}
