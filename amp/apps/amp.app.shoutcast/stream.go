package shoutcast

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// MetadataCallbackFunc is the type of the function called when the stream metadata changes
type MetadataCallbackFunc func(m *Metadata)

// Stream represents an open shoutcast stream.
type Stream struct {
	Name        string
	Genre       string
	Description string
	URL         string
	Bitrate     int

	// Optional function to be executed when stream metadata changes
	MetadataCallbackFunc MetadataCallbackFunc

	// Amount of bytes to read before expecting a metadata block
	metaint int

	// Stream metadata
	metadata *Metadata

	// The number of bytes read since last metadata block
	pos int

	// The underlying data stream
	reader io.ReadCloser
}

type IcyConnWrapper struct {
	net.Conn
	firstLineComplete bool
}

func (icy *IcyConnWrapper) Read(buf []byte) (int, error) {
	if icy.firstLineComplete {
		return icy.Conn.Read(buf)
	}

	lineLen := 0
	for {
		n, err := icy.Conn.Read(buf[lineLen : lineLen+1])
		if err != nil {
			return lineLen, err
		}
		newByte := buf[lineLen]
		if newByte == '\r' || newByte == '\n' {
			firstLine := strings.ToUpper(string(buf[0:lineLen]))
			if pos := strings.Index(firstLine, "ICY"); pos >= 0 {
				buf = append(buf[:0], []byte("HTTP/1.0")...)
				buf = append(buf, []byte(firstLine[3:])...)
				buf = append(buf, newByte)
			}
			break
		}

		lineLen += n
	}
	icy.firstLineComplete = true
	return len(buf), nil
}

// Open establishes a connection to a remote server.
func Open(url string) (*Stream, error) {
	log.Print("[INFO] Opening ", url)

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				realConn, err := net.Dial(network, addr)
				if err != nil {
					return nil, err
				}
				return &IcyConnWrapper{
					Conn: realConn,
				}, nil
			},
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("accept", "*/*")
	req.Header.Add("user-agent", "iTunes/12.9.2 (Macintosh; OS X 10.14.3) AppleWebKit/606.4.5")
	req.Header.Add("icy-metadata", "1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	for k, v := range resp.Header {
		log.Print("[DEBUG] HTTP header ", k, ": ", v[0])
	}

	var bitrate int
	if rawBitrate := resp.Header.Get("icy-br"); rawBitrate != "" {
		bitrate, err = strconv.Atoi(rawBitrate)
		if err != nil {
			return nil, fmt.Errorf("cannot parse bitrate: %v", err)
		}
	}

	metaint, err := strconv.Atoi(resp.Header.Get("icy-metaint"))
	if err != nil {
		return nil, fmt.Errorf("cannot parse metaint: %v", err)
	}

	stream := &Stream{
		Name:        resp.Header.Get("icy-name"),
		Genre:       resp.Header.Get("icy-genre"),
		Description: resp.Header.Get("icy-description"),
		URL:         resp.Header.Get("icy-url"),
		Bitrate:     bitrate,
		metaint:     metaint,
		metadata:    nil,
		pos:         0,
		reader:      resp.Body,
	}

	return stream, nil
}

// Read implements the standard Read interface
func (stream *Stream) Read(buf []byte) (N int, err error) {
	N, err = stream.reader.Read(buf)

	checked := 0
	unchecked := N
	for stream.pos+unchecked > stream.metaint {
		offset := stream.metaint - stream.pos
		skip, metaErr := stream.extractMetadata(buf[checked+offset:])
		if metaErr != nil {
			err = metaErr
		}
		stream.pos = 0
		if offset+skip > unchecked {
			N = checked + offset
			unchecked = 0
		} else {
			checked += offset
			N -= skip
			unchecked = N - checked
			copy(buf[checked:], buf[checked+skip:])
		}
	}
	stream.pos += unchecked

	return
}

// Close closes the stream
func (stream *Stream) Close() error {
	log.Print("[INFO] Closing ", stream.URL)
	return stream.reader.Close()
}

func (stream *Stream) extractMetadata(bufIn []byte) (int, error) {
	var metabuf []byte
	var err error
	metaLen := int(bufIn[0]) * 16
	end := metaLen + 1
	complete := false
	
	if metaLen > 0 {
		if len(bufIn) < end {
			// The provided buffer was not large enough for the metadata block to fit in.
			// Read whole metadata into our own buffer.
			metabuf = make([]byte, metaLen)
			copy(metabuf, bufIn[1:])
			n := len(bufIn) - 1
			for n < metaLen && err == nil {
				var nn int
				nn, err = stream.reader.Read(metabuf[n:])
				n += nn
			}
			if n == metaLen {
				complete = true
			} else if err == nil || err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
		} else {
			metabuf = bufIn[1:end]
			complete = true
		}
	}
	if complete {
		if m := NewMetadata(metabuf); !m.Equals(stream.metadata) {
			stream.metadata = m
			if stream.MetadataCallbackFunc != nil {
				stream.MetadataCallbackFunc(stream.metadata)
			}
		}
	}
	return end, err
}
