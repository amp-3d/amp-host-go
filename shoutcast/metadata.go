package shoutcast

import (
	"bytes"
	"log"
	"strings"
)

const (
	StreamTitle = "StreamTitle"
	StreamURL   = "StreamURL"
)


// Metadata represents the stream metadata sent by the server
type Metadata struct {
	Fields      map[string] string
	RawBuf      []byte
}


func (meta *Metadata) StreamTitle() string {
	return meta.Fields[StreamTitle]
}

func (meta *Metadata) StreamURL() string {
	return meta.Fields[StreamURL]
}


// NewMetadata returns parsed metadata
func NewMetadata(metaBuf []byte) *Metadata {
	meta := &Metadata{
		RawBuf: metaBuf,
		Fields: make(map[string]string),
	}
	
	for len(metaBuf) > 0 {
		EOL := bytes.IndexAny(metaBuf, ";\r\n")
		if EOL < 0 {
			break
		}
		if EOL > 0 {
			splitAt := bytes.IndexAny(metaBuf[:EOL], "=")
			if splitAt > 0 {
				key := strings.TrimSpace(string(metaBuf[:splitAt]))
				val := strings.TrimSpace(string(metaBuf[splitAt+1:EOL]))
				if key == StreamTitle {
					val = strings.Trim(val, "'\"")
				}
				meta.Fields[key] = val
				
				log.Print("[DEBUG] Received metadata: ", key, ":", val)

			}
		}
		// Resume after the break
		metaBuf = metaBuf[EOL+1:]
	}

	return meta
}

// Equals compares two Metadata structures for equality
func (meta *Metadata) Equals(other *Metadata) bool {
	if other == nil {
		return false
	}
	return bytes.Equal(meta.RawBuf, other.RawBuf)
}
