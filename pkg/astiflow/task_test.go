package astiflow

import (
	"context"
	"testing"
	"time"

	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestTaskShouldRunProperly(t *testing.T) {
	c := astikit.NewCloser()
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := astikit.NewWorker(astikit.WorkerOptions{})
	var stopped bool
	tk := newTask(c, func(_ context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
		tc().Do(func() { <-ctx.Done() })
	}, func() { stopped = true })

	var eventNames []astikit.EventName
	for _, n := range []astikit.EventName{
		eventNameTaskClosed,
		eventNameTaskDone,
		eventNameTaskRunning,
		eventNameTaskStarting,
		eventNameTaskStopping,
	} {
		ln := n
		tk.e.On(ln, func(payload interface{}) (delete bool) {
			eventNames = append(eventNames, ln)
			return
		})
	}

	require.NoError(t, tk.start(context.Background(), w.NewTask))
	require.Equal(t, StatusRunning, tk.status())
	require.Equal(t, []astikit.EventName{
		eventNameTaskStarting,
		eventNameTaskRunning,
	}, eventNames)
	eventNames = []astikit.EventName{}
	require.Error(t, tk.start(ctx, w.NewTask))

	require.NoError(t, tk.stop())
	require.Equal(t, StatusStopping, tk.status())
	require.Equal(t, []astikit.EventName{eventNameTaskStopping}, eventNames)
	eventNames = []astikit.EventName{}
	require.True(t, stopped)
	require.NoError(t, tk.stop())

	cancel()
	w.Stop()

	require.Eventually(t, func() bool { return tk.status() == StatusDone }, time.Second, 10*time.Millisecond)
	require.True(t, c.IsClosed())
	require.Equal(t, []astikit.EventName{
		eventNameTaskClosed,
		eventNameTaskDone,
	}, eventNames)
}

func TestTaskShouldBeProperlyStoppedEvenIfStopIsNotCalled(t *testing.T) {
	c := astikit.NewCloser()
	defer c.Close()
	w := astikit.NewWorker(astikit.WorkerOptions{})
	tk := newTask(c, func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
		tc().Do(func() { <-ctx.Done() })
	}, nil)

	var eventNames []astikit.EventName
	for _, n := range []astikit.EventName{
		eventNameTaskClosed,
		eventNameTaskDone,
		eventNameTaskRunning,
		eventNameTaskStarting,
		eventNameTaskStopping,
	} {
		ln := n
		tk.e.On(ln, func(payload interface{}) (delete bool) {
			eventNames = append(eventNames, ln)
			return
		})
	}

	require.NoError(t, tk.start(w.Context(), w.NewTask))
	w.Stop()

	require.Eventually(t, func() bool { return tk.status() == StatusDone }, time.Second, 10*time.Millisecond)
	require.Equal(t, []astikit.EventName{
		eventNameTaskStarting,
		eventNameTaskRunning,
		eventNameTaskStopping,
		eventNameTaskClosed,
		eventNameTaskDone,
	}, eventNames)
}

func TestTaskShouldNotStartIfContextIsDone(t *testing.T) {
	c := astikit.NewCloser()
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()
	tk := newTask(c, nil, nil)
	require.Error(t, tk.start(ctx, w.NewTask))
}

func TestTaskContextShouldBeCancelledEvenIfTaskIsNotStarted(t *testing.T) {
	c := astikit.NewCloser()
	defer c.Close()
	tk := newTask(c, nil, nil)
	c.Close()
	require.Nil(t, tk.cancel)
}

func TestTaskShouldStopWhenDoneIsCalled(t *testing.T) {
	c := astikit.NewCloser()
	defer c.Close()
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	var stopped bool
	tk := newTask(c, func(_ context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
		tc().Do(func() {
			<-ctx1.Done()
			cancel()
		})
	}, func() { stopped = true })

	w := astikit.NewWorker(astikit.WorkerOptions{})

	require.NoError(t, tk.start(context.Background(), w.NewTask))
	require.Equal(t, StatusRunning, tk.status())

	cancel1()
	w.Stop()

	require.Eventually(t, func() bool { return tk.status() == StatusDone }, time.Second, 10*time.Millisecond)
	require.True(t, stopped)
}
