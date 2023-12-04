package astiavflow

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/stretchr/testify/require"
)

func TestFrameDispatcher(t *testing.T) {
	f, err := astiflow.NewFlow(astiflow.FlowOptions{})
	require.NoError(t, err)
	defer f.Close()
	g, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)

	n1 := mocks.NewMockedNoder()
	n1.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: n1})
	require.NoError(t, err)
	h1 := newMockedFrameHandler()
	ns1 := []Frame{}
	h1.onFrame = func(p Frame) { ns1 = append(ns1, p) }
	h1.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h1})
	require.NoError(t, err)
	n1.Node.Connector().Connect(h1.NodeConnector())
	h2 := newMockedFrameHandler()
	ns2 := []Frame{}
	h2.onFrame = func(p Frame) { ns2 = append(ns2, p) }
	h2.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h2})
	require.NoError(t, err)
	n1.Node.Connector().Connect(h2.NodeConnector())

	d := newFrameDispatcher().init(n1.Node)
	dss := d.deltaStats()

	fm := Frame{Frame: astiav.AllocFrame()}
	defer fm.Free()

	d.dispatch(fm)
	requireDeltaStats(t, map[string]interface{}{astiflow.DeltaStatNameOutgoingRate: 1.0}, dss)
	require.Equal(t, []Frame{{
		Frame: fm.Frame,
		Noder: n1,
	}}, ns1)
	require.Equal(t, []Frame{{
		Frame: fm.Frame,
		Noder: n1,
	}}, ns2)

	n1.Node.Connector().Disconnect(h2.NodeConnector())

	d.dispatch(fm)
	requireDeltaStats(t, map[string]interface{}{astiflow.DeltaStatNameOutgoingRate: 1.0}, dss)
	require.Len(t, ns1, 2)
	require.Len(t, ns2, 1)

	require.Equal(t, frameDispatcherCumulativeStats{
		outgoingFrames: 2,
	}, *d.cs)
}
