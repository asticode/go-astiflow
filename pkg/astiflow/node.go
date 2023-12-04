package astiflow

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astikit"
)

// Noder should be the simplest interface possible, hopefuly only describing the action
// a node can do
// Internal node shouldn't be exposed here since we don't want other nodes/packages to be able to
// manipulate it since only the flow owner should be able to do it
type Noder interface {
	DeltaStats() []astikit.DeltaStat
	Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator)
}

// Node is purposefuly not an interface
type Node struct {
	children map[uint64]*Node // Indexed by node id
	ctx      context.Context
	e        *astikit.EventManager
	f        *Flow
	id       uint64
	m        *sync.Mutex // Locks all maps
	o        NodeOptions
	parents  map[uint64]*Node // Indexed by node id
	t        *task
}

type NodeOptions struct {
	Metadata Metadata
	Noder    Noder
	Stop     *NodeStopOptions
}

type NodeStopOptions struct {
	WhenAllChildrenAreDone bool // Default is false
	WhenGroupStops         bool // Default is true
	WhenAllParentsAreDone  bool // Default is false
}

func (g *Group) NewNode(o NodeOptions) (n *Node, c *astikit.Closer, err error) {
	// Invalid group status
	if g.Status() != StatusCreated {
		err = fmt.Errorf("astiflow: invalid group status %s", g.Status())
		return
	}

	// Create node
	n = &Node{
		children: make(map[uint64]*Node),
		ctx:      context.Background(),
		e:        astikit.NewEventManager(),
		f:        g.f,
		id:       atomic.AddUint64(&g.f.nodeCount, 1),
		m:        &sync.Mutex{},
		o:        o,
		parents:  make(map[uint64]*Node),
		t:        newTask(g.t.c.NewChild(), o.Noder.Start, nil),
	}

	// Adapt context
	if n.f.o.ContextAdapters.Node != nil {
		n.ctx = n.f.o.ContextAdapters.Node(n.ctx, n.f, g, n)
	}

	// Listen to task events
	n.t.e.On(eventNameTaskClosed, func(payload interface{}) (delete bool) {
		// Log
		n.f.l.InfoC(n.ctx, "astiflow: node is closed")

		// Emit
		n.Emit(EventNameNodeClosed, nil)
		return true
	})
	n.t.e.On(eventNameTaskDone, func(payload interface{}) (delete bool) {
		// Log
		n.f.l.InfoC(n.ctx, "astiflow: node is done")

		// Emit
		n.Emit(EventNameNodeDone, nil)
		return
	})
	n.t.e.On(eventNameTaskRunning, func(payload interface{}) (delete bool) {
		// Log
		n.f.l.InfoC(n.ctx, "astiflow: node is running")

		// Emit
		n.Emit(EventNameNodeRunning, nil)
		return
	})
	n.t.e.On(eventNameTaskStarting, func(payload interface{}) (delete bool) {
		// Log
		n.f.l.InfoC(n.ctx, "astiflow: node is starting")

		// Emit
		n.Emit(EventNameNodeStarting, nil)
		return
	})
	n.t.e.On(eventNameTaskStopping, func(payload interface{}) (delete bool) {
		// Log
		n.f.l.InfoC(n.ctx, "astiflow: node is stopping")

		// Emit
		n.Emit(EventNameNodeStopping, nil)
		return
	})

	// Create closer
	c = n.t.c.NewChild()

	// Emit created node
	g.Emit(EventNameNodeCreated, n)
	return
}

func (n *Node) ID() uint64 {
	return n.id
}

func (n *Node) String() string {
	if n.Metadata().Name != "" {
		return fmt.Sprintf("%s (node_%d)", n.Metadata().Name, n.id)
	}
	return fmt.Sprintf("node_%d", n.id)
}

func (n *Node) Metadata() Metadata {
	return n.o.Metadata
}

func (n *Node) Logger() astikit.CompleteLogger {
	return n.f.l
}

func (n *Node) Context() context.Context {
	return n.ctx
}

func (n *Node) Noder() Noder {
	return n.o.Noder
}

func (n *Node) Status() Status {
	return n.t.status()
}

func (n *Node) Emit(nm astikit.EventName, payload interface{}) {
	n.e.Emit(nm, payload)
}

func (n *Node) On(nm astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return n.e.On(nm, h)
}

func (n *Node) addNode(i *Node, m map[uint64]*Node, onAddedEventName, onRemovedEventName astikit.EventName, stopWhenAllNodesAreDone bool) {
	// Lock
	n.m.Lock()

	// Node already exists
	if _, ok := m[i.id]; ok {
		n.m.Unlock()
		return
	}

	// Handle done node
	off := i.On(EventNameNodeDone, func(payload interface{}) (delete bool) {
		// We should check whether we need to stop node
		if stopWhenAllNodesAreDone {
			// Lock
			n.m.Lock()

			// Get number of stopped nodes
			l := len(m)
			var count int
			for _, v := range m {
				if v.Status() == StatusDone {
					count++
				}
			}

			// Unlock
			n.m.Unlock()

			// All nodes are stopped
			if count == l {
				if err := n.stop(); err != nil {
					n.f.l.WarnC(n.ctx, fmt.Errorf("astiflow: stopping node failed: %w", err))
				}
			}
		}
		return false
	})

	// Make sure not to listen to stopped node once node has disconnected
	n.On(onRemovedEventName, func(payload interface{}) (delete bool) {
		// Assert payload
		n, ok := payload.(*Node)
		if !ok {
			return
		}

		// Invalid node
		if n != i {
			return
		}

		// Unlisten
		off()
		return true
	})

	// Add node
	m[i.id] = i

	// Unlock
	n.m.Unlock()

	//!\\ Mutex should be unlocked at this point

	// Emit event
	n.Emit(onAddedEventName, i)
}

func (n *Node) removeNode(i *Node, m map[uint64]*Node, onRemovedEventName astikit.EventName) {
	// Lock
	n.m.Lock()

	// Node doesn't exist
	if _, ok := m[i.id]; !ok {
		n.m.Unlock()
		return
	}

	// Delete node
	delete(m, i.id)

	// Unlock
	n.m.Unlock()

	//!\\ Mutex should be unlocked at this point

	// Emit event
	n.Emit(onRemovedEventName, i)
}

func (n *Node) listNodes(m map[uint64]*Node) (ns []*Node) {
	// Lock
	n.m.Lock()
	defer n.m.Unlock()

	// Get ids
	var ids []uint64
	for id := range m {
		ids = append(ids, id)
	}

	// Sort ids
	slices.Sort(ids)

	// Loop through ids
	ns = []*Node{}
	for _, id := range ids {
		ns = append(ns, m[id])
	}
	return
}

func (n *Node) addChild(i *Node) {
	n.addNode(i, n.children, EventNameNodeChildAdded, EventNameNodeChildRemoved, n.o.Stop != nil && n.o.Stop.WhenAllChildrenAreDone)
}

func (n *Node) removeChild(i *Node) {
	n.removeNode(i, n.children, EventNameNodeChildRemoved)
}

func (n *Node) addParent(i *Node) {
	n.addNode(i, n.parents, EventNameNodeParentAdded, EventNameNodeParentRemoved, n.o.Stop != nil && n.o.Stop.WhenAllParentsAreDone)
}

func (n *Node) removeParent(i *Node) {
	n.removeNode(i, n.parents, EventNameNodeParentRemoved)
}

func (n *Node) Children() []*Node {
	return n.listNodes(n.children)
}

func (n *Node) Parents() []*Node {
	return n.listNodes(n.parents)
}

func (n *Node) connect(dst *Node) {
	n.addChild(dst)
	dst.addParent(n)
}

func (n *Node) disconnect(dst *Node) {
	n.removeChild(dst)
	dst.removeParent(n)
}

func (n *Node) start(tc astikit.TaskCreator) error {
	// Start task
	// We don't want to use group context here since we don't always want node to stop when group stops
	if err := n.t.start(context.Background(), tc); err != nil {
		return fmt.Errorf("astiflow: starting task failed: %w", err)
	}
	return nil
}

func (n *Node) stop() error {
	// Stop task
	if err := n.t.stop(); err != nil {
		return fmt.Errorf("astiflow: stopping task failed: %w", err)
	}
	return nil
}

// NodeConnector's purpose is to provide a way to connect noders belonging to different external packages without
// exposing the node itself
type NodeConnector struct {
	n *Node
}

func (n *Node) Connector() *NodeConnector {
	return &NodeConnector{n: n}
}

func (src *NodeConnector) Connect(dst *NodeConnector) {
	src.n.connect(dst.n)
}

func (src *NodeConnector) Disconnect(dst *NodeConnector) {
	src.n.disconnect(dst.n)
}
