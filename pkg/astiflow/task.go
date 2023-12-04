package astiflow

import (
	"context"
	"fmt"
	"sync"

	"github.com/asticode/go-astikit"
)

type task struct {
	c       *astikit.Closer
	cancel  context.CancelFunc
	ctx     context.Context
	e       *astikit.EventManager
	m       *sync.Mutex // Locks status
	onStart onTaskStart
	onStop  onTaskStop
	s       Status
	t       *astikit.Task
}

type onTaskStart func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator)

type onTaskStop func()

func newTask(c *astikit.Closer, onStart onTaskStart, onStop onTaskStop) *task {
	// Create task
	t := &task{
		c:       c,
		e:       astikit.NewEventManager(),
		m:       &sync.Mutex{},
		onStart: onStart,
		onStop:  onStop,
		s:       StatusCreated,
	}

	// Make sure context is cancelled
	t.c.Add(func() {
		if t.cancel != nil {
			t.cancel()
		}
	})

	// Emit closed event when task closes
	t.c.OnClosed(func(err error) { t.e.Emit(eventNameTaskClosed, nil) })
	return t
}

func (t *task) status() Status {
	t.m.Lock()
	defer t.m.Unlock()
	return t.s
}

func (t *task) start(ctx context.Context, tc astikit.TaskCreator) error {
	// Lock
	t.m.Lock()

	// Invalid status
	if t.s != StatusCreated {
		t.m.Unlock()
		return fmt.Errorf("astiflow: invalid status %s", t.s)
	}

	// Check context
	if ctx.Err() != nil {
		t.m.Unlock()
		return ctx.Err()
	}

	// Create task
	t.t = tc()

	// Create context
	t.ctx, t.cancel = context.WithCancel(ctx)

	// Update status
	t.s = StatusStarting

	// Unlock
	t.m.Unlock()

	//!\\ Mutex should be unlocked at this point

	// Emit event
	t.e.Emit(eventNameTaskStarting, nil)

	// Callback
	t.onStart(t.ctx, t.cancel, t.t.NewSubTask)

	// Update status
	t.m.Lock()
	t.s = StatusRunning
	t.m.Unlock()

	// Emit event
	t.e.Emit(eventNameTaskRunning, nil)

	// Execute the rest in a goroutine
	// We can't use t.Do() since we want stopped status to be updated after task is done waiting
	go func() {
		// Wait for context
		<-t.ctx.Done()

		// Make sure task is properly stopped
		t.m.Lock()
		if t.s == StatusRunning {
			t.stopUnsafe()
		} else {
			t.m.Unlock()
		}

		// Wait for task to be done
		t.t.Wait()

		// Close task
		t.c.Close()

		// Update status
		t.m.Lock()
		t.s = StatusDone
		t.m.Unlock()

		// Emit event
		t.e.Emit(eventNameTaskDone, nil)

		// Task is done
		t.t.Done()
	}()
	return nil
}

func (t *task) stop() error {
	// Lock
	t.m.Lock()

	// Invalid status
	if s := t.s; s != StatusRunning {
		t.m.Unlock()
		if s == StatusStopping || s == StatusDone {
			return nil
		}
		return fmt.Errorf("astiflow: invalid status %s", s)
	}

	// Stop
	t.stopUnsafe()
	return nil
}

// Mutex should be locked
func (t *task) stopUnsafe() {
	// Update status
	t.s = StatusStopping

	// Unlock
	t.m.Unlock()

	//!\\ Mutex should be unlocked at this point

	// Emit event
	t.e.Emit(eventNameTaskStopping, nil)

	// Cancel context
	if t.cancel != nil {
		t.cancel()
	}

	// Callback
	if t.onStop != nil {
		t.onStop()
	}
}
