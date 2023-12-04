package astiavflow

import (
	"testing"
	"time"

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
	h1.handlePacketFunc = func(p Packet) { ns1 = append(ns1, p) }
	h1.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h1})
	require.NoError(t, err)
	n1.Node.Connector().Connect(h1.NodeConnector())
	h2 := newMockedPacketHandler()
	ns2 := []Packet{}
	h2.handlePacketFunc = func(p Packet) { ns2 = append(ns2, p) }
	h2.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h2})
	require.NoError(t, err)
	n1.Node.Connector().Connect(h2.NodeConnector())

	d := newPacketDispatcher().init(n1.Node)
	dss := d.deltaStats()
	require.Len(t, dss, 2)
	require.Equal(t, 0.0, dss[0].Valuer.Value(time.Second))
	require.Equal(t, 0.0, dss[1].Valuer.Value(time.Second))

	pkt := Packet{Packet: astiav.AllocPacket()}
	pkt.SetSize(2)
	defer pkt.Free()

	d.dispatch(pkt)
	require.Equal(t, 1.0, dss[0].Valuer.Value(time.Second))
	require.Equal(t, 2.0, dss[1].Valuer.Value(time.Second))
	require.Equal(t, []Packet{{
		Noder:  n1,
		Packet: pkt.Packet,
	}}, ns1)
	require.Equal(t, []Packet{{
		Noder:  n1,
		Packet: pkt.Packet,
	}}, ns2)

	d.setSkipper(h1, func(pkt Packet) (skip bool) { return true })
	d.dispatch(pkt)
	require.Len(t, ns1, 1)
	require.Len(t, ns2, 2)

	d.delSkipper(h1)
	d.dispatch(pkt)
	require.Len(t, ns1, 2)
	require.Len(t, ns2, 3)

	require.Equal(t, packetDispatcherCumulativeStats{
		outgoingBytes:   6,
		outgoingPackets: 3,
	}, *d.cumulativeStats)
}
