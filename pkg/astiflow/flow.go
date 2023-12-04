package astiflow

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astikit"
)

var flowCount uint64

type Flow struct {
	ctx        context.Context
	dss        []astikit.DeltaStat
	e          *astikit.EventManager
	groupCount uint64
	gs         []*Group
	id         uint64
	l          astikit.CompleteLogger
	mg         sync.Mutex // Locks gs
	nodeCount  uint64
	o          FlowOptions
	ps         []Plugin
	t          *task
}

type FlowOptions struct {
	ContextAdapters FlowContextAdaptersOptions
	DeltaStats      []astikit.DeltaStat
	Logger          astikit.StdLogger
	Metadata        Metadata
	Plugins         []Plugin
	Stop            *FlowStopOptions
	Worker          *astikit.Worker
}

type FlowContextAdaptersOptions struct {
	Flow   func(context.Context, *Flow) context.Context
	Group  func(context.Context, *Flow, *Group) context.Context
	Node   func(context.Context, *Flow, *Group, *Node) context.Context
	Plugin func(context.Context, *Flow, Plugin) context.Context
}

type FlowStopOptions struct {
	WhenAllGroupsAreDone bool // Default is false
}

func NewFlow(o FlowOptions) (f *Flow, err error) {
	// Create flow
	f = &Flow{
		ctx: context.Background(),
		dss: make([]astikit.DeltaStat, len(o.DeltaStats)),
		e:   astikit.NewEventManager(),
		id:  atomic.AddUint64(&flowCount, 1),
		l:   astikit.AdaptStdLogger(o.Logger),
		o:   o,
		ps:  make([]Plugin, len(o.Plugins)),
	}

	// Adapt context
	if f.o.ContextAdapters.Flow != nil {
		f.ctx = f.o.ContextAdapters.Flow(f.ctx, f)
	}

	// Copy stats
	copy(f.dss, o.DeltaStats)

	// Copy plugins
	copy(f.ps, o.Plugins)

	// Create task
	f.t = newTask(astikit.NewCloser(), f.onTaskStart, f.onTaskStop)

	// Listen to group events
	f.On(EventNameGroupCreated, func(payload interface{}) bool {
		// Assert payload
		g, ok := payload.(*Group)
		if !ok {
			return false
		}

		// Store group
		f.mg.Lock()
		f.gs = append(f.gs, g)
		f.mg.Unlock()

		// Listen to group events
		g.On(EventNameGroupDone, func(payload interface{}) bool {
			// Remove group
			f.mg.Lock()
			for idx := 0; idx < len(f.gs); idx++ {
				if g.id == f.gs[idx].id {
					f.gs = append(f.gs[:idx], f.gs[idx+1:]...)
					idx--
				}
			}
			allGroupsAreDone := len(f.gs) == 0
			f.mg.Unlock()

			// All groups are done
			if allGroupsAreDone && f.o.Stop != nil && f.o.Stop.WhenAllGroupsAreDone {
				// Stop flow
				f.Stop() //nolint: errcheck
			}
			return false
		})
		return false
	})

	// Listen to task events
	f.t.e.On(eventNameTaskClosed, func(payload interface{}) (delete bool) {
		// Log
		f.l.InfoC(f.ctx, "astiflow: flow is closed")

		// Emit
		f.Emit(EventNameFlowClosed, nil)
		return true
	})
	f.t.e.On(eventNameTaskDone, func(payload interface{}) (delete bool) {
		// Log
		f.l.InfoC(f.ctx, "astiflow: flow is done")

		// Emit
		f.Emit(EventNameFlowDone, nil)
		return
	})
	f.t.e.On(eventNameTaskRunning, func(payload interface{}) (delete bool) {
		// Log
		f.l.InfoC(f.ctx, "astiflow: flow is running")

		// Emit
		f.Emit(EventNameFlowRunning, nil)
		return
	})
	f.t.e.On(eventNameTaskStarting, func(payload interface{}) (delete bool) {
		// Log
		f.l.InfoC(f.ctx, "astiflow: flow is starting")

		// Emit
		f.Emit(EventNameFlowStarting, nil)
		return
	})
	f.t.e.On(eventNameTaskStopping, func(payload interface{}) (delete bool) {
		// Log
		f.l.InfoC(f.ctx, "astiflow: flow is stopping")

		// Emit
		f.Emit(EventNameFlowStopping, nil)
		return
	})

	// Loop through plugins
	for idx, p := range f.ps {
		// Create context
		ctx := context.Background()
		if f.o.ContextAdapters.Plugin != nil {
			ctx = f.o.ContextAdapters.Plugin(ctx, f, p)
		}

		// Initialize plugin
		if err = p.Init(ctx, f.t.c.NewChild(), f); err != nil {
			err = fmt.Errorf("astiflow: initializing plugin #%d failed: %w", idx, err)
			return
		}
	}
	return
}

func (f *Flow) ID() uint64 {
	return f.id
}

func (f *Flow) String() string {
	if f.Metadata().Name != "" {
		return fmt.Sprintf("%s (flow_%d)", f.Metadata().Name, f.id)
	}
	return fmt.Sprintf("flow_%d", f.id)
}

func (f *Flow) DeltaStats() []astikit.DeltaStat {
	dst := make([]astikit.DeltaStat, len(f.dss))
	copy(dst, f.dss)
	return dst
}

func (f *Flow) Metadata() Metadata {
	return f.o.Metadata
}

func (f *Flow) Logger() astikit.CompleteLogger {
	return f.l
}

func (f *Flow) Context() context.Context {
	return f.ctx
}

func (f *Flow) Close() error {
	return f.t.c.Close()
}

func (f *Flow) Status() Status {
	return f.t.status()
}

func (f *Flow) Emit(n astikit.EventName, payload interface{}) {
	f.e.Emit(n, payload)
}

func (f *Flow) On(n astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return f.e.On(n, h)
}

func (f *Flow) Groups() (groups []*Group) {
	f.mg.Lock()
	defer f.mg.Unlock()
	groups = make([]*Group, len(f.gs))
	copy(groups, f.gs)
	return
}

func (f *Flow) Start(ctx context.Context) error {
	// Start task
	if err := f.t.start(ctx, f.o.Worker.NewTask); err != nil {
		return fmt.Errorf("astiflow: starting task failed: %w", err)
	}
	return nil
}

func (f *Flow) onTaskStart(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	// Loop through plugins
	for _, p := range f.ps {
		// Start plugin
		p.Start(ctx, tc)
	}

	// Loop through groups
	for _, g := range f.Groups() {
		// Start group
		if err := g.Start(); err != nil {
			f.l.WarnC(g.ctx, fmt.Errorf("astiflow: starting group failed: %w", err))
		}
	}
}

func (f *Flow) onTaskStop() {
	// Loop through groups
	for _, g := range f.Groups() {
		// Stop group
		if err := g.Stop(); err != nil {
			f.l.WarnC(g.ctx, fmt.Errorf("astiflow: stopping group failed: %w", err))
		}
	}
}

func (f *Flow) Stop() error {
	// Stop task
	if err := f.t.stop(); err != nil {
		return fmt.Errorf("astiflow: stopping task failed: %w", err)
	}
	return nil
}
