package astiavflow

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/stretchr/testify/require"
)

func TestPacketDispatcher(t *testing.T) {
	f, err := astiflow.NewFlow(astiflow.FlowOptions{})
	require.NoError(t, err)
	defer f.Close()
	g, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)

	n1 := mocks.NewMockedNoder()
	n1.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: n1})
	require.NoError(t, err)
	h1 := newMockedPacketHandler()
	ns1 := []Packet{}
	h1.onPacket = func(p Packet) { ns1 = append(ns1, p) }
	h1.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h1})
	require.NoError(t, err)
	n1.Node.Connector().Connect(h1.NodeConnector())
	h2 := newMockedPacketHandler()
	ns2 := []Packet{}
	h2.onPacket = func(p Packet) { ns2 = append(ns2, p) }
	h2.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h2})
	require.NoError(t, err)
	n1.Node.Connector().Connect(h2.NodeConnector())

	d := newPacketDispatcher().init(n1.Node)
	dss := d.deltaStats()

	pkt := Packet{Packet: astiav.AllocPacket()}
	pkt.SetStreamIndex(0)
	pkt.SetSize(2)
	defer pkt.Free()

	d.dispatch(pkt)
	requireDeltaStats(t, map[string]interface{}{
		astiflow.DeltaStatNameOutgoingByteRate: 2.0,
		astiflow.DeltaStatNameOutgoingRate:     1.0,
	}, dss)
	require.Equal(t, []Packet{{
		Noder:  n1,
		Packet: pkt.Packet,
	}}, ns1)
	require.Equal(t, []Packet{{
		Noder:  n1,
		Packet: pkt.Packet,
	}}, ns2)

	d.dispatch(pkt, func(h PacketHandler) (skip bool) { return h == h2 })
	require.Len(t, ns1, 2)
	require.Len(t, ns2, 1)

	n1.Node.Connector().Disconnect(h2.NodeConnector())

	d.dispatch(pkt)
	require.Len(t, ns1, 3)
	require.Len(t, ns2, 1)

	require.Equal(t, packetDispatcherCumulativeStats{
		outgoingBytes:   6,
		outgoingPackets: 3,
	}, *d.cs)
}
