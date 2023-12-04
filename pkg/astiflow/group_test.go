package astiflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestGroupShouldRunProperly(t *testing.T) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()
	events := astikit.NewEventInterceptor()
	f, err := astiflow.NewFlow(astiflow.FlowOptions{Worker: w})
	require.NoError(t, err)
	defer f.Close()

	g, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	events.Intercept(
		g,
		astiflow.EventNameGroupClosed,
		astiflow.EventNameGroupDone,
		astiflow.EventNameGroupRunning,
		astiflow.EventNameGroupStarting,
		astiflow.EventNameGroupStopping,
	)

	n1 := mocks.NewMockedNoder()
	n1.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: n1})
	require.NoError(t, err)
	n2 := mocks.NewMockedNoder()
	n2.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: n2})
	require.NoError(t, err)
	n1.Node.Connector().Connect(n2.Node.Connector())

	require.Error(t, g.Start())
	require.NoError(t, f.Start(w.Context()))
	require.Equal(t, astiflow.StatusRunning, g.Status())
	require.Equal(t, astiflow.StatusRunning, n1.Node.Status())
	require.Equal(t, astiflow.StatusRunning, n2.Node.Status())
	require.Equal(t, map[astikit.EventProcesser][]astikit.Event{g: {
		{EventName: astiflow.EventNameGroupStarting},
		{EventName: astiflow.EventNameGroupRunning},
	}}, events.Pool())
	events.Reset()

	require.NoError(t, g.Stop())
	require.True(t, g.Status() >= astiflow.StatusStopping)

	require.Eventually(t, func() bool { return g.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)

	require.Equal(t, astiflow.StatusDone, g.Status())
	require.Equal(t, astiflow.StatusDone, n1.Node.Status())
	require.Equal(t, astiflow.StatusDone, n2.Node.Status())
	require.Equal(t, map[astikit.EventProcesser][]astikit.Event{g: {
		{EventName: astiflow.EventNameGroupStopping},
		{EventName: astiflow.EventNameGroupClosed},
		{EventName: astiflow.EventNameGroupDone},
	}}, events.Pool())
	require.Equal(t, astiflow.StatusRunning, f.Status())
	require.Len(t, n1.Node.Children(), 0)
	require.Len(t, n2.Node.Parents(), 0)

	_, err = f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	require.NoError(t, f.Stop())
	_, err = f.NewGroup(astiflow.GroupOptions{})
	require.Error(t, err)
}

func TestGroupShouldApplyNodeStopOptionsProperly(t *testing.T) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()
	f, err := astiflow.NewFlow(astiflow.FlowOptions{Worker: w})
	require.NoError(t, err)
	defer f.Close()

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	g1, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	n1 := mocks.NewMockedNoder()
	n1.Node, _, err = g1.NewNode(astiflow.NodeOptions{Noder: n1})
	require.NoError(t, err)
	n2 := mocks.NewMockedNoder()
	n2.Node, _, err = g1.NewNode(astiflow.NodeOptions{Noder: n2})
	require.NoError(t, err)
	n1.Node.Connector().Connect(n2.Node.Connector())

	g2, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	n3 := mocks.NewMockedNoder()
	n3.Node, _, err = g2.NewNode(astiflow.NodeOptions{Noder: n3, Stop: &astiflow.NodeStopOptions{WhenAllChildrenAreDone: true}})
	require.NoError(t, err)
	n4 := mocks.NewMockedNoder()
	n4.Node, _, err = g2.NewNode(astiflow.NodeOptions{Noder: n4})
	require.NoError(t, err)
	n4.OnStart = func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
		tc().Do(func() {
			<-ctx.Done()
			<-ctx1.Done()
		})
	}
	n3.Node.Connector().Connect(n4.Node.Connector())

	g3, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	n5 := mocks.NewMockedNoder()
	n5.Node, _, err = g3.NewNode(astiflow.NodeOptions{Noder: n5})
	require.NoError(t, err)
	n5.OnStart = func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
		tc().Do(func() {
			<-ctx.Done()
			<-ctx1.Done()
		})
	}
	n6 := mocks.NewMockedNoder()
	n6.Node, _, err = g3.NewNode(astiflow.NodeOptions{Noder: n6, Stop: &astiflow.NodeStopOptions{WhenAllParentsAreDone: true}})
	require.NoError(t, err)
	n5.Node.Connector().Connect(n6.Node.Connector())

	require.NoError(t, f.Start(w.Context()))

	require.NoError(t, g1.Stop())
	require.True(t, n1.Node.Status() > astiflow.StatusRunning)
	require.True(t, n2.Node.Status() > astiflow.StatusRunning)

	require.NoError(t, g2.Stop())
	require.Equal(t, astiflow.StatusRunning, n3.Node.Status())
	require.True(t, n4.Node.Status() > astiflow.StatusRunning)

	require.NoError(t, g3.Stop())
	require.True(t, n5.Node.Status() > astiflow.StatusRunning)
	require.Equal(t, astiflow.StatusRunning, n6.Node.Status())

	cancel1()

	require.Eventually(t, func() bool { return n3.Node.Status() > astiflow.StatusRunning }, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return n4.Node.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return n5.Node.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return n6.Node.Status() > astiflow.StatusRunning }, time.Second, 10*time.Millisecond)

	require.NoError(t, f.Stop())
}

func TestGroupShouldHandleNodesProperly(t *testing.T) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()
	f, err := astiflow.NewFlow(astiflow.FlowOptions{Worker: w})
	require.NoError(t, err)
	defer f.Close()

	g, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)

	nd1 := mocks.NewMockedNoder()
	n1, _, err := g.NewNode(astiflow.NodeOptions{Noder: nd1})
	require.NoError(t, err)
	nd1.Node = n1
	nd2 := mocks.NewMockedNoder()
	n2, _, err := g.NewNode(astiflow.NodeOptions{Noder: nd2})
	require.NoError(t, err)
	nd2.Node = n2

	ns := map[*astiflow.Node]bool{}
	for _, n := range g.Nodes() {
		ns[n] = true
	}

	require.Equal(t, map[*astiflow.Node]bool{
		n1: true,
		n2: true,
	}, ns)

	n1.Connector().Connect(n2.Connector())
	require.Len(t, n1.Children(), 1)
	g.DisconnectNodes()
	require.Len(t, n1.Children(), 0)
}

func TestGroupShouldStopWhenAllNodesAreDone(t *testing.T) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()
	f, err := astiflow.NewFlow(astiflow.FlowOptions{Worker: w})
	require.NoError(t, err)
	defer f.Close()

	g, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)

	n := mocks.NewMockedNoder()
	n.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: n})
	require.NoError(t, err)
	n.OnStart = func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
		cancel()
	}

	require.NoError(t, f.Start(w.Context()))

	require.Eventually(t, func() bool { return g.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)

	require.NoError(t, f.Stop())
}
