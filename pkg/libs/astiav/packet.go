package astiavflow

import (
	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
)

type Packet struct {
	*astiav.Packet
	CodecParameters *astiav.CodecParameters
	MediaContext    MediaContext
	Noder           astiflow.Noder
}

type PacketHandler interface {
	HandlePacket(p Packet)
	NodeConnector() *astiflow.NodeConnector
}

type PacketSkipper func(pkt Packet) (skip bool)

func StreamPacketSkipper(s Stream) PacketSkipper {
	return func(pkt Packet) bool { return s.Index != pkt.StreamIndex() }
}
