// go/cmd/lib-phost/main.go
package main

import "C"

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/arcverse/go-planet/planet"
)

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

var count int
var mtx sync.Mutex

//export Add
func Add(a, b int) int {
	return a + b
}

//export Cosine
func Cosine(x float64) float64 {
	return math.Cos(x)
}

//export Sort
func Sort(vals []int) {
	sort.Ints(vals)
}

//export SortPtr
func SortPtr(vals *[]int) {
	Sort(*vals)
}

//export Log
func Log(msg string) int {
	mtx.Lock()
	defer mtx.Unlock()
	fmt.Println(msg)
	count++
	return count
}

//export LogPtr
func LogPtr(msg *string) int {
	return Log(*msg)
}


//export Call_SessionBegin
func Call_SessionBegin() int {
	return 55
}

var sessOpen = make(chan struct{})

//export Call_SessionEnd
func Call_SessionEnd(sessID int) int {
	select {
	case <-sessOpen:
	default:
		close(sessOpen)
	}

	return 0
}

// mallocs retains allocations so they are not GCed
var mallocs = make(map[*byte]struct{})

// //export Call_Malloc2
// func Call_Malloc2(numBytes int) { //, dst **int32) {
// 	buf := make([]byte, numBytes)
// 	ptr := &buf[0]
// 	mallocs[ptr] = struct{}{}

// 	//*dst = (*int32)(unsafe.Pointer(&buf[0]))

// 	//return buf
// 	//return 0
// }

//export Call_Realloc
func Call_Realloc(buf *[]byte, numBytes int) {
	if numBytes < 0 {
		numBytes = 0
	}
	
	// No-op if no size change
	curSz := cap(*buf)
	capSz := (numBytes + 0x3FF) &^ 0x3FF
	if curSz == capSz {
		*buf = (*buf)[:numBytes]
		return
	}
	
	// Free prev buffer (if allocated)
	if curSz > 0 {
		ptr := &(*buf)[0]
		delete(mallocs, ptr)
	}
	
	// Allocate new buf and place it in our tracker map so to the GC doesn't taketh away
	newBuf := make([]byte, capSz)
	ptr := &newBuf[0]
	mallocs[ptr] = struct{}{}
	*buf = newBuf[:numBytes]
}


var gOutbox = make(chan *planet.Msg, 10)

//export Call_PushMsg
func Call_PushMsg(msgBuf []byte) int {
	msg := planet.NewMsg()
	err := msg.Unmarshal(msgBuf)
	if err != nil {
		return -1
	}
	
	fmt.Printf("Call_PushMsg: ReqID: %d,   %-10s", msg.ReqID, msg.ValBuf)

	go func() {
		msg.ValBuf = []byte(string(msg.ValBuf) + " -- REPLY")
		gOutbox <- msg
	}()

	return 0
}



//export Call_WaitOnMsg
func Call_WaitOnMsg(msgBuf *[]byte) int {

	select {
	case msg := <-gOutbox:
		//msg.IsClosed = true
		sz := msg.Size()
		Call_Realloc(msgBuf, sz)
		n, err := msg.MarshalToSizedBuffer(*msgBuf)
		msg.Reclaim()
		if err != nil || n != 0 {
			return -1005
		}
		return 0
	case <-sessOpen:
	}

	//*out, _ = gMsg.Marshal()
	return -1006
}

func main() {

	//params := phost.DefaultHostParams
	// host, err := phost.NewHost(params)
	// if err != nil {
	// 	log.Fatal(err)
	// }

}
