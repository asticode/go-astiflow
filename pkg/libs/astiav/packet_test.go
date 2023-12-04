package astiavflow

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/stretchr/testify/require"
)

type mockedPacketHandler struct {
	*mocks.MockedNoder
	handlePacketFunc func(p Packet)
}

var _ PacketHandler = (*mockedPacketHandler)(nil)

func newMockedPacketHandler() *mockedPacketHandler {
	return &mockedPacketHandler{
		MockedNoder: mocks.NewMockedNoder(),
	}
}

func (h *mockedPacketHandler) HandlePacket(p Packet) {
	if h.handlePacketFunc != nil {
		h.handlePacketFunc(p)
	}
}

func (h *mockedPacketHandler) NodeConnector() *astiflow.NodeConnector {
	return h.Node.Connector()
}

func TestStreamPacketSkipper(t *testing.T) {
	sk := StreamPacketSkipper(Stream{Index: 1})
	pkt := astiav.AllocPacket()
	defer pkt.Free()
	pkt.SetStreamIndex(1)
	require.False(t, sk(Packet{Packet: pkt}))
	pkt.SetStreamIndex(2)
	require.True(t, sk(Packet{Packet: pkt}))
}
