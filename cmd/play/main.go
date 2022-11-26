package main

import (
	"fmt"
	"os"

	"github.com/arcverse/go-arcverse/shoutcast"
	"github.com/arcverse/go-cedar/utils"
)


func main() {
	
    playFrisky()
}




// func playFrisky() {

//     client := &http.Client{
//         Transport: &http.Transport{
//             Dial: func(network, a string) (net.Conn, error) {
//                 realConn, err := net.Dial(network, a)
//                 if err != nil {
//                     return nil, err
//                 }
//                 return &IcyConnWrapper{
//                     Conn: realConn,
//                 }, nil
//             },
//         },
//     }

//     var url = "http://stream.friskyradio.com:8000/frisky_aac_hi"
//     req, err := http.NewRequest("GET", url, nil)
//     if err != nil {
//         fmt.Printf("Got error %s\n", err.Error())
//         return
//     }
//     req.Header.Set("Icy-MetaData", "1")

//     // url = "http://prem1.di.fm:80/psychill?a03c469845de992689bbed4f"
//     // url = "http://stream.ancientfm.com:8058/stream"
//     // url = "http://deep.friskyradio.com/friskydeep_aachi"

//     resp, err := client.Do(req)
//     fmt.Println("err: ", err)
//     fmt.Println("StatusCode: ", resp.StatusCode)
//     for k, v := range resp.Header {
//         fmt.Println( k, ":", v)
//     }

//     if err != nil {
// 		log.Fatal(err)
// 	}

// 	// var args = os.Args
// 	// if len(args) != 2 {
// 	// 	log.Fatal("Run test like this:\n\n\t./networkAudio.test [mp3url]\n\n")
// 	// }

// 	// var response *http.Response
// 	// if response, err = http.Get(url);

//     NewMultiDecoder(resp.Body)

// 	var dec *minimp3.Decoder
// 	if dec, err = minimp3.NewDecoder(resp.Body); err != nil {
// 		log.Fatal(err)
// 	}
// 	<-dec.Started()

// 	log.Printf("Convert audio sample rate: %d, channels: %d\n", dec.SampleRate, dec.Channels)

// 	var context *oto.Context
// 	if context, err = oto.NewContext(dec.SampleRate, dec.Channels, 2, 4096); err != nil {
// 		log.Fatal(err)
// 	}

// 	var waitForPlayOver = new(sync.WaitGroup)
// 	waitForPlayOver.Add(1)

// 	var player = context.NewPlayer()

// 	go func() {
// 		defer resp.Body.Close()
// 		for {
// 			var data = make([]byte, 512)
// 			_, err = dec.Read(data)
// 			if err == io.EOF {
// 				break
// 			}
// 			if err != nil {
// 				log.Fatal(err)
// 				break
// 			}
// 			player.Write(data)
// 		}
// 		log.Println("over play.")
// 		waitForPlayOver.Done()
// 	}()

// 	waitForPlayOver.Wait()

// 	<-time.After(time.Second)
// 	dec.Close()
// 	player.Close()
// }

// type EasyDecoder struct {
// 	readerLocker  *sync.Mutex
// 	data          []byte
// 	decoderLocker *sync.Mutex
// 	decodedData   []byte
// 	context       context.Context
// 	contextCancel context.CancelFunc
// 	SampleRate    int
// 	Channels      int
// 	Kbps          int
// 	Layer         int
// }

// // BufferSize Decoded data buffer size.
// var BufferSize = 1024 * 16

// // WaitForDataDuration wait for the data time duration.
// var WaitForDataDuration = time.Millisecond * 10

// const maxSamplesPerFrame = 1152 * 2

// func NewMultiDecoder(reader io.Reader) {

// 	buf := make([]byte, 16 * 4096)
//     N := 0
//     for N < cap(buf) {
//         n, err := reader.Read(buf[N:])
//         if err != nil {
//             log.Fatal(err)
//         }
//         N += n
//     }

// 	// {
//     //     adts, err := gaad.ParseADTS(buf[:N])
//     //     if err != nil {
//     //         log.Fatal(err)
//     //     }

//     //     // Looping through top level elements and accessing sub-elements
//     //    // var sbr bool
//     //     if adts.Fill_elements != nil {
//     //         for _, e := range adts.Fill_elements {
//     //             if e.Extension_payload != nil &&
//     //                 e.Extension_payload.Extension_type == gaad.EXT_SBR_DATA {
//     //                 //sbr = true
//     //             }
//     //         }
//     //     }
//     // }

// }



func playFrisky() {

    ofile, err := utils.CreateTemp("", "stream*", os.O_APPEND|os.O_WRONLY)
    if err != nil {
        panic(err)
    }
    
    
    // open stream
    
    url := "http://streamingp.shoutcast.com/TomorrowlandOneWorldRadio"
    //url = "http://stream.ancientfm.com:8058/stream"
    //url = "http://fr.ah.fm:8000/192k"
    //url = "http://deep.friskyradio.com/friskydeep_aachi"
    url = "http://stream.friskyradio.com:8000/frisky_aac_hi"
    stream, err := shoutcast.Open(url)
    if err != nil {
        panic(err)
    }
    
    defer stream.Close()
    
    // optionally register a callback function to be called when song changes
    stream.MetadataCallbackFunc = func(meta *shoutcast.Metadata) {
       
    }
    
    buf := make([]byte, 16 * 1024)
    

   //file, err := os.OpenFile(string(filepath), os.O_APPEND|os.O_RDWR|os.O_CREATE, 0600)

    for {
        _, err := stream.Read(buf)
        if err != nil {
            fmt.Println("Error:", err)
            break
        }
        ofile.Write(buf)
    }



    // // setup mp3 decoder
    // decoder, err := mp3.NewDecoder(stream)
    // if err != nil {
    //     panic(err)
    // }
    // defer decoder.Close()

    // // initialise audio player
    // player, err := oto.NewPlayer(decoder.SampleRate(), 2, 2, 8192)
    // if err != nil {
    //     panic(err)
    // }
    // defer player.Close()

    // // enjoy the music
    // if _, err := io.Copy(player, decoder); err != nil {
    //     panic(err)
    // }
}





