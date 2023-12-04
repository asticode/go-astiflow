package astiavflow

import (
	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
)

type Packet struct {
	*astiav.Packet
	PacketDescriptor
	Noder astiflow.Noder
}
