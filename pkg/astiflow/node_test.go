package astiflow_test

import (
	"testing"
	"time"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestNodeShouldRunProperly(t *testing.T) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	events := astikit.NewEventInterceptor()
	f, err := astiflow.NewFlow(astiflow.FlowOptions{Worker: w})
	require.NoError(t, err)
	defer f.Close()
	g, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)

	n := mocks.NewMockedNoder()
	n.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: n})
	require.NoError(t, err)
	events.Intercept(
		n.Node,
		astiflow.EventNameNodeClosed,
		astiflow.EventNameNodeDone,
		astiflow.EventNameNodeRunning,
		astiflow.EventNameNodeStarting,
		astiflow.EventNameNodeStopping,
	)

	require.NoError(t, f.Start(w.Context()))
	require.Equal(t, astiflow.StatusRunning, n.Node.Status())
	require.Equal(t, map[astikit.EventProcesser][]astikit.Event{n.Node: {
		{EventName: astiflow.EventNameNodeStarting},
		{EventName: astiflow.EventNameNodeRunning},
	}}, events.Pool())
	events.Reset()

	_, _, err = g.NewNode(astiflow.NodeOptions{Noder: n})
	require.Error(t, err)

	require.NoError(t, f.Stop())
	require.True(t, n.Node.Status() >= astiflow.StatusStopping)
	w.Stop()

	require.Eventually(t, func() bool { return f.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)

	require.Equal(t, astiflow.StatusDone, n.Node.Status())
	require.Equal(t, map[astikit.EventProcesser][]astikit.Event{n.Node: {
		{EventName: astiflow.EventNameNodeStopping},
		{EventName: astiflow.EventNameNodeClosed},
		{EventName: astiflow.EventNameNodeDone},
	}}, events.Pool())
}

func TestNodeShouldConnectProperly(t *testing.T) {
	c := astikit.NewCloser()
	defer c.Close()
	f, err := astiflow.NewFlow(astiflow.FlowOptions{})
	require.NoError(t, err)
	defer f.Close()
	g, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	events := astikit.NewEventInterceptor()

	n1 := mocks.NewMockedNoder()
	n2 := mocks.NewMockedNoder()
	n1.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: n1})
	require.NoError(t, err)
	n2.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: n2})
	require.NoError(t, err)
	for _, n := range []*astiflow.Node{n1.Node, n2.Node} {
		events.Intercept(
			n,
			astiflow.EventNameNodeChildAdded,
			astiflow.EventNameNodeChildRemoved,
			astiflow.EventNameNodeParentAdded,
			astiflow.EventNameNodeParentRemoved,
		)
	}

	n1.Node.Connector().Connect(n2.Node.Connector())
	require.Equal(t, map[astikit.EventProcesser][]astikit.Event{
		n1.Node: {{
			EventName: astiflow.EventNameNodeChildAdded,
			Payload:   n2.Node,
		}},
		n2.Node: {{
			EventName: astiflow.EventNameNodeParentAdded,
			Payload:   n1.Node,
		}},
	}, events.Pool())
	events.Reset()
	require.Equal(t, []*astiflow.Node{n2.Node}, n1.Node.Children())
	require.Equal(t, []*astiflow.Node{n1.Node}, n2.Node.Parents())

	n1.Node.Connector().Disconnect(n2.Node.Connector())
	require.Equal(t, map[astikit.EventProcesser][]astikit.Event{
		n1.Node: {{
			EventName: astiflow.EventNameNodeChildRemoved,
			Payload:   n2.Node,
		}},
		n2.Node: {{
			EventName: astiflow.EventNameNodeParentRemoved,
			Payload:   n1.Node,
		}},
	}, events.Pool())
	require.Equal(t, []*astiflow.Node{}, n1.Node.Children())
	require.Equal(t, []*astiflow.Node{}, n2.Node.Parents())
}
