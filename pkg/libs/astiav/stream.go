package astiavflow

import "github.com/asticode/go-astiav"

// TODO Test
type Stream struct {
	CodecParameters *astiav.CodecParameters
	ID              int
	Index           int
	MediaContext    MediaContext
}

func newStream(s *astiav.Stream) Stream {
	return Stream{
		CodecParameters: s.CodecParameters(),
		ID:              s.ID(),
		Index:           s.Index(),
		MediaContext:    newMediaContextFromStream(s),
	}
}
