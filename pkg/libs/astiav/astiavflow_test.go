package astiavflow

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func withGroup(t *testing.T, fn func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker)) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()
	f, err := astiflow.NewFlow(astiflow.FlowOptions{Worker: w})
	require.NoError(t, err)
	defer f.Close()
	g, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)

	fn(f, g, w)
}

func requireDeltaStats(t *testing.T, expected map[string]interface{}, ss []astikit.DeltaStat) {
	require.Len(t, ss, len(expected))
	for _, s := range ss {
		if s.Metadata.Name == astikit.StatNameWorkedRatio {
			require.Greater(t, s.Valuer.Value(time.Second), 0.0)
			continue
		}

		v, ok := expected[s.Metadata.Name]
		if !ok {
			require.Fail(t, fmt.Sprintf("delta stat %s shouldn't be here", s.Metadata.Name))
		}
		require.Equal(t, v, s.Valuer.Value(time.Second))
	}
}

func TestDispatchError(t *testing.T) {
	var seconds int64
	defer astikit.MockNow(func() time.Time {
		return time.Unix(seconds, 0)
	}).Close()
	tk := astikit.MockTickers()
	defer tk.Close()

	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()

	l1 := astikit.NewMockedLogger()
	l1.SkipFunc = func(msg string) (skip bool) {
		return !strings.HasPrefix(msg, "astiavflow: ")
	}
	li := NewLogInterceptor(LogInterceptorOptions{
		Level: astiav.LogLevelInfo,
		Merge: LogInterceptorMergeOptions{
			AllowedCount: 1,
			Buffer:       2 * time.Second,
		},
	})
	f1, err := astiflow.NewFlow(astiflow.FlowOptions{
		Logger:  l1,
		Plugins: []astiflow.Plugin{li},
		Worker:  w,
	})
	require.NoError(t, err)
	defer f1.Close()
	g1, err := f1.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	n1, _, err := g1.NewNode(astiflow.NodeOptions{Noder: mocks.NewMockedNoder()})
	require.NoError(t, err)

	l2 := astikit.NewMockedLogger()
	l2.SkipFunc = func(msg string) (skip bool) {
		return !strings.HasPrefix(msg, "astiavflow: ")
	}
	f2, err := astiflow.NewFlow(astiflow.FlowOptions{
		Logger: l2,
		Worker: w,
	})
	require.NoError(t, err)
	defer f2.Close()
	g2, err := f2.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	n2, _, err := g2.NewNode(astiflow.NodeOptions{Noder: mocks.NewMockedNoder()})
	require.NoError(t, err)

	require.NoError(t, f1.Start(w.Context()))
	require.NoError(t, f2.Start(w.Context()))
	require.NoError(t, tk.Wait(time.Second))

	seconds = 1
	dispatchError(n1, "astiavflow: fmt %d", 1)
	dispatchError(n2, "astiavflow: fmt %d", 1)
	seconds = 2
	dispatchError(n1, "astiavflow: fmt %d", 2)
	dispatchError(n2, "astiavflow: fmt %d", 2)
	tk.Tick(time.Unix(3, 0))

	require.Equal(t, []astikit.MockedLoggerItem{
		{
			Context:     n1.Context(),
			LoggerLevel: astikit.LoggerLevelWarn,
			Message:     "astiavflow: fmt 1",
		},
		{
			Context:     n1.Context(),
			LoggerLevel: astikit.LoggerLevelWarn,
			Message:     "astiavflow: pattern repeated once: astiavflow: fmt %d",
		},
	}, l1.Items)

	require.Equal(t, []astikit.MockedLoggerItem{
		{
			Context:     n2.Context(),
			LoggerLevel: astikit.LoggerLevelWarn,
			Message:     "astiavflow: fmt 1",
		},
		{
			Context:     n2.Context(),
			LoggerLevel: astikit.LoggerLevelWarn,
			Message:     "astiavflow: fmt 2",
		},
	}, l2.Items)

	w.Stop()
}

func TestDurationToTimeBase(t *testing.T) {
	i, r := durationToTimeBase(time.Second, astiav.NewRational(1, 2))
	require.Equal(t, int64(2), i)
	require.Equal(t, time.Duration(0), r)
	i, r = durationToTimeBase(time.Second+450*time.Millisecond, astiav.NewRational(1, 2))
	require.Equal(t, int64(2), i)
	require.Equal(t, 450*time.Millisecond, r)
}
