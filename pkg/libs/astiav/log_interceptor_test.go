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

func TestLogInterceptorShouldHandleDefaultLevelsProperly(t *testing.T) {
	li := NewLogInterceptor(LogInterceptorOptions{Level: astiav.LogLevelDebug})
	l := astikit.NewMockedLogger()
	l.SkipFunc = func(msg string) (skip bool) { return !strings.HasPrefix(msg, "libav: ") }
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()
	f, err := astiflow.NewFlow(astiflow.FlowOptions{
		Logger:  l,
		Plugins: []astiflow.Plugin{li},
		Worker:  w,
	})
	require.NoError(t, err)

	astiav.SetLogLevel(astiav.LogLevelInfo)
	require.NoError(t, f.Start(w.Context()))
	astiav.Log(nil, astiav.LogLevelDebug, "")
	astiav.Log(nil, astiav.LogLevelDebug, "0")
	astiav.Log(nil, astiav.LogLevelVerbose, "1")
	astiav.Log(nil, astiav.LogLevelInfo, "2")
	astiav.Log(nil, astiav.LogLevelWarning, "3")
	astiav.Log(nil, astiav.LogLevelError, "4")
	astiav.Log(nil, astiav.LogLevelFatal, "5")
	astiav.Log(nil, astiav.LogLevelPanic, "6")
	require.Equal(t, []astikit.MockedLoggerItem{
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelDebug,
			Message:     "libav: 0",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelDebug,
			Message:     "libav: 1",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "libav: 2",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelWarn,
			Message:     "libav: 3",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelError,
			Message:     "libav: 4",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelError,
			Message:     "libav: FATAL! 5",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelError,
			Message:     "libav: PANIC! 6",
		},
	}, l.Items)

	require.NoError(t, f.Close())
	require.Equal(t, astiav.LogLevelInfo, astiav.GetLogLevel())
	l.Items = []astikit.MockedLoggerItem{}
	astiav.Log(nil, astiav.LogLevelInfo, "uncatched\n")
	require.Empty(t, l.Items)
}

func TestLogInterceptorShouldHandleLevelFuncProperly(t *testing.T) {
	li := NewLogInterceptor(LogInterceptorOptions{
		Level: astiav.LogLevelWarning,
		LevelFunc: func(l astiav.LogLevel) (ll astikit.LoggerLevel, processed bool, stop bool) {
			switch l {
			case astiav.LogLevelError:
				ll = astikit.LoggerLevelWarn
				processed = true
			case astiav.LogLevelFatal:
				stop = true
			}
			return
		},
	})
	l := astikit.NewMockedLogger()
	l.SkipFunc = func(msg string) (skip bool) { return !strings.HasPrefix(msg, "libav: ") }
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()
	f, err := astiflow.NewFlow(astiflow.FlowOptions{
		Logger:  l,
		Plugins: []astiflow.Plugin{li},
		Worker:  w,
	})
	require.NoError(t, err)
	defer f.Close()

	astiav.Log(nil, astiav.LogLevelWarning, "1\n")
	require.NoError(t, f.Start(w.Context()))
	astiav.Log(nil, astiav.LogLevelInfo, "2")
	astiav.Log(nil, astiav.LogLevelWarning, "3")
	astiav.Log(nil, astiav.LogLevelError, "4")
	astiav.Log(nil, astiav.LogLevelFatal, "5")
	require.Equal(t, []astikit.MockedLoggerItem{
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelWarn,
			Message:     "libav: 3",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelWarn,
			Message:     "libav: 4",
		},
	}, l.Items)
}

func TestLogInterceptorShouldFormatLogsProperly(t *testing.T) {
	li := NewLogInterceptor(LogInterceptorOptions{Level: astiav.LogLevelInfo})
	l := astikit.NewMockedLogger()
	l.SkipFunc = func(msg string) (skip bool) { return !strings.HasPrefix(msg, "libav: ") }
	w := astikit.NewWorker(astikit.WorkerOptions{})
	f, err := astiflow.NewFlow(astiflow.FlowOptions{
		Logger:  l,
		Plugins: []astiflow.Plugin{li},
		Worker:  w,
	})
	require.NoError(t, err)
	defer f.Close()
	g, err := f.NewGroup(astiflow.GroupOptions{})
	require.NoError(t, err)
	n, _, err := g.NewNode(astiflow.NodeOptions{Noder: mocks.NewMockedNoder()})
	require.NoError(t, err)
	_, ok := logInterceptors.get(n)
	require.True(t, ok)
	f1 := astiav.AllocFilterGraph()
	defer f1.Free()
	classers.set(f1, n)
	defer classers.del(f1)
	f2 := astiav.AllocFilterGraph()
	defer f2.Free()

	require.NoError(t, f.Start(w.Context()))
	astiav.Log(nil, astiav.LogLevelInfo, " 1 ")
	astiav.Log(nil, astiav.LogLevelInfo, " %s ", "2")
	astiav.Log(nil, astiav.LogLevelInfo, " 3 %s ", "arg")
	astiav.Log(f1, astiav.LogLevelInfo, "4")
	astiav.Log(f2, astiav.LogLevelInfo, "5")
	require.Equal(t, []astikit.MockedLoggerItem{
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "libav: 1",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "libav: 2",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "libav: 3 arg",
		},
		{
			Context:     n.Context(),
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     fmt.Sprintf("libav: 4: %s", f1.Class()),
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     fmt.Sprintf("libav: 5: %s", f2.Class()),
		},
	}, l.Items)

	w.Stop()

	require.Eventually(t, func() bool { return f.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
	_, ok = logInterceptors.get(n)
	require.False(t, ok)
}

func TestLogInterceptorShouldMergeLogsProperly(t *testing.T) {
	var seconds int64
	defer astikit.MockNow(func() time.Time {
		return time.Unix(seconds, 0)
	}).Close()
	tk := astikit.MockTickers()
	defer tk.Close()

	li := NewLogInterceptor(LogInterceptorOptions{
		Level: astiav.LogLevelInfo,
		Merge: LogInterceptorMergeOptions{
			AllowedCount: 2,
			Buffer:       6 * time.Second,
		},
	})
	l := astikit.NewMockedLogger()
	l.SkipFunc = func(msg string) (skip bool) {
		return !strings.HasPrefix(msg, "libav: ") && !strings.HasPrefix(msg, "astiavflow: pattern repeated")
	}
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()
	f, err := astiflow.NewFlow(astiflow.FlowOptions{
		Logger:  l,
		Plugins: []astiflow.Plugin{li},
		Worker:  w,
	})
	require.NoError(t, err)

	require.NoError(t, f.Start(w.Context()))
	require.NoError(t, tk.Wait(time.Second))
	seconds = 1
	astiav.Log(nil, astiav.LogLevelInfo, "%s", "1")
	astiav.Log(nil, astiav.LogLevelInfo, "fmt1 %s", "1")
	astiav.Log(nil, astiav.LogLevelInfo, "fmt2 %s", "1")
	tk.Tick(time.Unix(2, 0))
	require.Equal(t, []astikit.MockedLoggerItem{
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "libav: 1",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "libav: fmt1 1",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "libav: fmt2 1",
		},
	}, l.Items)
	l.Items = []astikit.MockedLoggerItem{}
	seconds = 3
	astiav.Log(nil, astiav.LogLevelInfo, "%s", "2")
	astiav.Log(nil, astiav.LogLevelInfo, "fmt1 %s", "2")
	astiav.Log(nil, astiav.LogLevelInfo, "fmt2 %s", "2")
	astiav.Log(nil, astiav.LogLevelInfo, "fmt3 %s", "1")
	tk.Tick(time.Unix(4, 0))
	require.Equal(t, []astikit.MockedLoggerItem{
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "libav: 2",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "libav: fmt1 2",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "libav: fmt2 2",
		},
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "libav: fmt3 1",
		},
	}, l.Items)
	l.Items = []astikit.MockedLoggerItem{}
	seconds = 5
	astiav.Log(nil, astiav.LogLevelInfo, "fmt1 %s", "3")
	astiav.Log(nil, astiav.LogLevelInfo, "fmt2 %s", "3")
	astiav.Log(nil, astiav.LogLevelInfo, "fmt3 %s", "2")
	tk.Tick(time.Unix(6, 0))
	require.Equal(t, []astikit.MockedLoggerItem{
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "libav: fmt3 2",
		},
	}, l.Items)
	l.Items = []astikit.MockedLoggerItem{}
	seconds = 7
	astiav.Log(nil, astiav.LogLevelInfo, "fmt2 %s", "4")
	astiav.Log(nil, astiav.LogLevelInfo, "fmt3 %s", "3")
	tk.Tick(time.Unix(8, 0))
	require.Len(t, l.Items, 2)
	require.Contains(t, l.Items, astikit.MockedLoggerItem{
		Context:     li.ctx,
		LoggerLevel: astikit.LoggerLevelInfo,
		Message:     "astiavflow: pattern repeated once: libav: fmt1 %s",
	})
	require.Contains(t, l.Items, astikit.MockedLoggerItem{
		Context:     li.ctx,
		LoggerLevel: astikit.LoggerLevelInfo,
		Message:     "astiavflow: pattern repeated 2 times: libav: fmt2 %s",
	})
	l.Items = []astikit.MockedLoggerItem{}
	require.NoError(t, f.Close())
	require.Equal(t, []astikit.MockedLoggerItem{
		{
			Context:     li.ctx,
			LoggerLevel: astikit.LoggerLevelInfo,
			Message:     "astiavflow: pattern repeated once: libav: fmt3 %s",
		},
	}, l.Items)
}
