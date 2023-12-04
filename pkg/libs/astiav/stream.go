package astiavflow

import "github.com/asticode/go-astiav"

type Stream struct {
	PacketDescriptor
	ID    int
	Index int
}

func newStream(s *astiav.Stream) Stream {
	return Stream{
		ID:    s.ID(),
		Index: s.Index(),
		PacketDescriptor: PacketDescriptor{
			CodecParameters: s.CodecParameters(),
			MediaDescriptor: newMediaDescriptorFromStream(s),
		},
	}
}
