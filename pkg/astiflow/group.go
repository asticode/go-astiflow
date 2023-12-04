package astiflow

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astikit"
)

type Group struct {
	ctx context.Context
	e   *astikit.EventManager
	f   *Flow
	id  uint64
	mn  sync.Mutex // Locks ns
	ns  []*Node
	o   GroupOptions
	t   *task
}

type GroupOptions struct {
	Metadata Metadata
}

func (f *Flow) NewGroup(o GroupOptions) (g *Group, err error) {
	// Invalid flow status
	if f.Status() > StatusRunning {
		err = fmt.Errorf("astiflow: invalid flow status %s", f.Status())
		return
	}

	// Create group
	g = &Group{
		ctx: context.Background(),
		e:   astikit.NewEventManager(),
		f:   f,
		id:  atomic.AddUint64(&f.groupCount, 1),
		o:   o,
	}

	// Adapt context
	if f.o.ContextAdapters.Group != nil {
		g.ctx = f.o.ContextAdapters.Group(g.ctx, f, g)
	}

	// Create task
	g.t = newTask(f.t.c.NewChild(), g.onTaskStart, g.onTaskStop)

	// Listen to nodes events
	g.On(EventNameNodeCreated, func(payload interface{}) bool {
		// Assert payload
		n, ok := payload.(*Node)
		if !ok {
			return false
		}

		// Store node
		g.mn.Lock()
		g.ns = append(g.ns, n)
		g.mn.Unlock()

		// Listen to nodes events
		n.On(EventNameNodeDone, func(payload interface{}) bool {
			// Disconnect node
			g.disconnectNode(n)

			// Remove node
			g.mn.Lock()
			for idx := 0; idx < len(g.ns); idx++ {
				if n.id == g.ns[idx].id {
					g.ns = append(g.ns[:idx], g.ns[idx+1:]...)
					idx--
				}
			}
			allNodesAreDone := len(g.ns) == 0
			g.mn.Unlock()

			// All nodes are done
			if allNodesAreDone {
				// Stop group
				g.Stop() //nolint: errcheck
			}
			return false
		})
		return false
	})

	// Listen to task events
	g.t.e.On(eventNameTaskClosed, func(payload interface{}) (delete bool) {
		// Log
		f.l.InfoC(g.ctx, "astiflow: group is closed")

		// Emit
		g.Emit(EventNameGroupClosed, nil)
		return true
	})
	g.t.e.On(eventNameTaskDone, func(payload interface{}) (delete bool) {
		// Log
		f.l.InfoC(g.ctx, "astiflow: group is done")

		// Emit
		g.Emit(EventNameGroupDone, nil)
		return
	})
	g.t.e.On(eventNameTaskRunning, func(payload interface{}) (delete bool) {
		// Log
		f.l.InfoC(g.ctx, "astiflow: group is running")

		// Emit
		g.Emit(EventNameGroupRunning, nil)
		return
	})
	g.t.e.On(eventNameTaskStarting, func(payload interface{}) (delete bool) {
		// Log
		f.l.InfoC(g.ctx, "astiflow: group is starting")

		// Emit
		g.Emit(EventNameGroupStarting, nil)
		return
	})
	g.t.e.On(eventNameTaskStopping, func(payload interface{}) (delete bool) {
		// Log
		f.l.InfoC(g.ctx, "astiflow: group is stopping")

		// Emit
		g.Emit(EventNameGroupStopping, nil)
		return
	})

	// Emit created group
	f.Emit(EventNameGroupCreated, g)
	return
}

func (g *Group) ID() uint64 {
	return g.id
}

func (g *Group) String() string {
	if g.Metadata().Name != "" {
		return fmt.Sprintf("%s (group_%d)", g.Metadata().Name, g.id)
	}
	return fmt.Sprintf("group_%d", g.id)
}

func (g *Group) Metadata() Metadata {
	return g.o.Metadata
}

func (g *Group) Logger() astikit.CompleteLogger {
	return g.f.l
}

func (g *Group) Context() context.Context {
	return g.ctx
}

func (g *Group) Status() Status {
	return g.t.status()
}

func (g *Group) Emit(nm astikit.EventName, payload interface{}) {
	g.e.Emit(nm, payload)
}

func (g *Group) On(nm astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return g.e.On(nm, h)
}

func (g *Group) Nodes() (nodes []*Node) {
	g.mn.Lock()
	defer g.mn.Unlock()
	nodes = make([]*Node, len(g.ns))
	copy(nodes, g.ns)
	return
}

func (g *Group) DisconnectNodes() {
	// Loop through nodes
	for _, n := range g.Nodes() {
		// Disconnect node
		g.disconnectNode(n)
	}
}

func (g *Group) disconnectNode(n *Node) {
	// Disconnect parents
	for _, p := range n.Parents() {
		p.disconnect(n)
	}

	// Disconnect children
	for _, c := range n.Children() {
		n.disconnect(c)
	}
}

func (g *Group) Closer() *astikit.ProtectedCloser {
	return astikit.NewProtectedCloser(g.t.c)
}

func (g *Group) Close() error {
	return g.t.c.Close()
}

func (g *Group) Start() error {
	// Groups can only be started if flow is either starting or running
	if s := g.f.Status(); s != StatusStarting && s != StatusRunning {
		return fmt.Errorf("astiflow: invalid flow status %s", s)
	}

	// Start task
	// We don't use flow context here since we want to stop groups using their .Stop() method
	if err := g.t.start(context.Background(), g.f.t.t.NewSubTask); err != nil {
		return fmt.Errorf("astiflow: starting task failed: %w", err)
	}
	return nil
}

func (g *Group) onTaskStart(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	// Loop through nodes
	for _, n := range g.Nodes() {
		// Start node
		if err := n.start(tc); err != nil {
			g.f.l.WarnC(n.ctx, fmt.Errorf("astiflow: starting node failed: %w", err))
		}
	}
}

func (g *Group) onTaskStop() {
	// Loop through nodes
	for _, n := range g.Nodes() {
		// Node should be stopped
		if n.o.Stop == nil || n.o.Stop.WhenGroupStops {
			// Stop node
			if err := n.stop(); err != nil {
				g.f.l.WarnC(n.ctx, fmt.Errorf("astiflow: stopping node failed: %w", err))
			}
		}
	}
}

func (g *Group) Stop() error {
	// Stop task
	if err := g.t.stop(); err != nil {
		return fmt.Errorf("astiflow: stopping task failed: %w", err)
	}
	return nil
}
