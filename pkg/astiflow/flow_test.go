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

func TestFlowShouldRunProperly(t *testing.T) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	events := astikit.NewEventInterceptor()
	l := astikit.NewMockedLogger()

	f, err := astiflow.NewFlow(astiflow.FlowOptions{
		Logger:   l,
		Metadata: astiflow.Metadata{Name: "fn"},
		Worker:   w,
	})
	require.NoError(t, err)
	defer f.Close()
	events.Intercept(
		f,
		astiflow.EventNameGroupCreated,
		astiflow.EventNameFlowClosed,
		astiflow.EventNameFlowDone,
		astiflow.EventNameFlowRunning,
		astiflow.EventNameFlowStarting,
		astiflow.EventNameFlowStopping,
	)

	require.Equal(t, uint64(1), f.ID())
	require.Equal(t, "fn (flow_1)", f.String())

	g, err := f.NewGroup(astiflow.GroupOptions{Metadata: astiflow.Metadata{Name: "gn"}})
	require.NoError(t, err)
	n := mocks.NewMockedNoder()
	n.Node, _, err = g.NewNode(astiflow.NodeOptions{Metadata: astiflow.Metadata{Name: "nn"}, Noder: n})
	require.NoError(t, err)
	require.Equal(t, map[astikit.EventProcesser][]astikit.Event{f: {{
		EventName: astiflow.EventNameGroupCreated,
		Payload:   g,
	}}}, events.Pool())
	events.Reset()

	require.Equal(t, uint64(1), g.ID())
	require.Equal(t, "gn (group_1)", g.String())
	require.Equal(t, uint64(1), n.Node.ID())
	require.Equal(t, "nn (node_1)", n.Node.String())
	require.Equal(t, n, n.Node.Noder())

	require.NoError(t, f.Start(w.Context()))
	require.Equal(t, astiflow.StatusRunning, f.Status())
	require.Equal(t, map[astikit.EventProcesser][]astikit.Event{f: {
		{EventName: astiflow.EventNameFlowStarting},
		{EventName: astiflow.EventNameFlowRunning},
	}}, events.Pool())
	events.Reset()
	require.Error(t, f.Start(context.Background()))

	require.NoError(t, f.Stop())
	require.True(t, f.Status() >= astiflow.StatusStopping)
	require.NoError(t, f.Stop())

	w.Stop()

	require.Eventually(t, func() bool { return f.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
	require.Equal(t, astiflow.StatusDone, g.Status())
	require.Equal(t, astiflow.StatusDone, n.Node.Status())
	require.Equal(t, map[astikit.EventProcesser][]astikit.Event{f: {
		{EventName: astiflow.EventNameFlowStopping},
		{EventName: astiflow.EventNameFlowClosed},
		{EventName: astiflow.EventNameFlowDone},
	}}, events.Pool())
	require.Equal(t, []astikit.MockedLoggerItem{
		{Context: f.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: flow is starting"},
		{Context: g.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: group is starting"},
		{Context: n.Node.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: node is starting"},
		{Context: n.Node.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: node is running"},
		{Context: g.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: group is running"},
		{Context: f.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: flow is running"},
		{Context: f.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: flow is stopping"},
		{Context: g.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: group is stopping"},
		{Context: n.Node.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: node is stopping"},
		{Context: n.Node.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: node is closed"},
		{Context: n.Node.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: node is done"},
		{Context: g.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: group is closed"},
		{Context: g.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: group is done"},
		{Context: f.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: flow is closed"},
		{Context: f.Context(), LoggerLevel: astikit.LoggerLevelInfo, Message: "astiflow: flow is done"},
	}, l.Items)
}

func TestFlowShouldStopWhenContextIsCanceled(t *testing.T) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	f, err := astiflow.NewFlow(astiflow.FlowOptions{Worker: w})
	require.NoError(t, err)
	defer f.Close()

	g, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)

	_, _, err = g.NewNode(astiflow.NodeOptions{
		Noder: mocks.NewMockedNoder(),
	})
	require.NoError(t, err)

	require.NoError(t, f.Start(w.Context()))
	w.Stop()

	require.Eventually(t, func() bool { return f.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
}

func TestFlowShouldHandleGroupsProperly(t *testing.T) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()
	f, err := astiflow.NewFlow(astiflow.FlowOptions{Worker: w})
	require.NoError(t, err)
	defer f.Close()

	g1, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	g2, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)

	gs := map[*astiflow.Group]bool{}
	for _, g := range f.Groups() {
		gs[g] = true
	}

	require.Equal(t, map[*astiflow.Group]bool{
		g1: true,
		g2: true,
	}, gs)
}

func TestFlowShouldHandleDeltaStatsProperly(t *testing.T) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()
	ds := astikit.DeltaStat{}
	f, err := astiflow.NewFlow(astiflow.FlowOptions{
		DeltaStats: []astikit.DeltaStat{ds},
		Worker:     w,
	})
	require.NoError(t, err)
	defer f.Close()

	require.Equal(t, []astikit.DeltaStat{ds}, f.DeltaStats())
}

func TestFlowShouldHandleContextAdaptersProperly(t *testing.T) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()

	type contextKey string
	k := contextKey("v")
	ctxf := context.WithValue(context.Background(), k, "f")
	ctxg := context.WithValue(context.Background(), k, "g")
	ctxn := context.WithValue(context.Background(), k, "n")
	ctxp := context.WithValue(context.Background(), k, "p")

	p := mocks.NewMockedPlugin()

	f, err := astiflow.NewFlow(astiflow.FlowOptions{
		ContextAdapters: astiflow.FlowContextAdaptersOptions{
			Flow:  func(ctx context.Context, f *astiflow.Flow) context.Context { return ctxf },
			Group: func(ctx context.Context, f *astiflow.Flow, g *astiflow.Group) context.Context { return ctxg },
			Node: func(ctx context.Context, f *astiflow.Flow, g *astiflow.Group, n *astiflow.Node) context.Context {
				return ctxn
			},
			Plugin: func(ctx context.Context, f *astiflow.Flow, p astiflow.Plugin) context.Context { return ctxp },
		},
		Plugins: []astiflow.Plugin{p},
		Worker:  w,
	})
	require.NoError(t, err)
	defer f.Close()
	require.Equal(t, "f", f.Context().Value(k))
	require.Equal(t, "p", p.Context.Value(k))

	g, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	require.Equal(t, "g", g.Context().Value(k))

	n, _, err := g.NewNode(astiflow.NodeOptions{Noder: mocks.NewMockedNoder()})
	require.NoError(t, err)
	require.Equal(t, "n", n.Context().Value(k))
}

func TestFlowShouldHandleStopOptionsProperly(t *testing.T) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()

	f1, err := astiflow.NewFlow(astiflow.FlowOptions{Worker: w})
	require.NoError(t, err)
	defer f1.Close()
	f2, err := astiflow.NewFlow(astiflow.FlowOptions{
		Stop:   &astiflow.FlowStopOptions{WhenAllGroupsAreDone: true},
		Worker: w,
	})
	require.NoError(t, err)
	defer f2.Close()

	var s1, s2 astiflow.Status
	g1, err := f1.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	g1.On(astiflow.EventNameGroupDone, func(payload interface{}) (delete bool) {
		s1 = f1.Status()
		return
	})
	g2, err := f2.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	g2.On(astiflow.EventNameGroupDone, func(payload interface{}) (delete bool) {
		s2 = f2.Status()
		return
	})

	require.NoError(t, f1.Start(w.Context()))
	require.NoError(t, f2.Start(w.Context()))
	require.NoError(t, g1.Stop())
	require.NoError(t, g2.Stop())

	require.Eventually(t, func() bool { return s1 == astiflow.StatusRunning }, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return s2 > astiflow.StatusRunning }, time.Second, 10*time.Millisecond)

	require.NoError(t, f1.Stop())
	require.NoError(t, f2.Stop())
}
