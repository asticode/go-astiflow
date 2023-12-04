package monitorer

import (
	"context"
	"sync"
	"time"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

type Delta struct {
	At                astikit.Timestamp      `json:"at"`
	ConnectedNodes    []DeltaConnection      `json:"connected_nodes,omitempty"`
	DisconnectedNodes []DeltaConnection      `json:"disconnected_nodes,omitempty"`
	DoneGroups        []uint64               `json:"done_groups,omitempty"`
	DoneNodes         []uint64               `json:"done_nodes,omitempty"`
	NewStats          []DeltaStat            `json:"new_stats,omitempty"`
	StartedGroups     []DeltaGroup           `json:"started_groups,omitempty"`
	StartedNodes      []DeltaNode            `json:"started_nodes,omitempty"`
	StatValues        map[uint64]interface{} `json:"stat_values,omitempty"`
}

func newDelta() *Delta {
	return &Delta{StatValues: make(map[uint64]interface{})}
}

func (d Delta) empty() bool {
	return len(d.ConnectedNodes) == 0 && len(d.DisconnectedNodes) == 0 &&
		len(d.DoneGroups) == 0 && len(d.DoneNodes) == 0 &&
		len(d.NewStats) == 0 && len(d.StartedGroups) == 0 &&
		len(d.StartedGroups) == 0 && len(d.StatValues) == 0
}

func (d Delta) copy() *Delta {
	dst := newDelta()
	dst.At = d.At
	if len(d.ConnectedNodes) > 0 {
		dst.ConnectedNodes = make([]DeltaConnection, len(d.ConnectedNodes))
		copy(dst.ConnectedNodes, d.ConnectedNodes)
	}
	if len(d.DisconnectedNodes) > 0 {
		dst.DisconnectedNodes = make([]DeltaConnection, len(d.DisconnectedNodes))
		copy(dst.DisconnectedNodes, d.DisconnectedNodes)
	}
	if len(d.DoneGroups) > 0 {
		dst.DoneGroups = make([]uint64, len(d.DoneGroups))
		copy(dst.DoneGroups, d.DoneGroups)
	}
	if len(d.DoneNodes) > 0 {
		dst.DoneNodes = make([]uint64, len(d.DoneNodes))
		copy(dst.DoneNodes, d.DoneNodes)
	}
	if len(d.NewStats) > 0 {
		dst.NewStats = make([]DeltaStat, len(d.NewStats))
		copy(dst.NewStats, d.NewStats)
	}
	if len(d.StartedGroups) > 0 {
		dst.StartedGroups = make([]DeltaGroup, len(d.StartedGroups))
		copy(dst.StartedGroups, d.StartedGroups)
	}
	if len(d.StartedNodes) > 0 {
		dst.StartedNodes = make([]DeltaNode, len(d.StartedNodes))
		copy(dst.StartedNodes, d.StartedNodes)
	}
	if len(d.StatValues) > 0 {
		dst.StatValues = make(map[uint64]interface{}, len(d.StatValues))
		for k, v := range d.StatValues {
			dst.StatValues[k] = v
		}
	}
	return dst
}

type DeltaConnection struct {
	From uint64 `json:"from"`
	To   uint64 `json:"to"`
}

type DeltaGroup struct {
	ID       uint64            `json:"id"`
	Metadata astiflow.Metadata `json:"metadata"`
}

type DeltaNode struct {
	GroupID  uint64            `json:"group_id"`
	ID       uint64            `json:"id"`
	Metadata astiflow.Metadata `json:"metadata"`
}

type DeltaStat struct {
	ID       uint64            `json:"id"`
	Metadata DeltaStatMetadata `json:"metadata"`
	NodeID   *uint64           `json:"node_id,omitempty"`
}

type DeltaStatMetadata struct {
	Description string `json:"description,omitempty"`
	Label       string `json:"label,omitempty"`
	Name        string `json:"name,omitempty"`
	Unit        string `json:"unit,omitempty"`
}

func newDeltaStatMetadata(i astikit.DeltaStatMetadata) DeltaStatMetadata {
	return DeltaStatMetadata{
		Description: i.Description,
		Label:       i.Label,
		Name:        i.Name,
		Unit:        i.Unit,
	}
}

type DeltaStatValue struct {
	StatID uint64      `json:"stat_id"`
	Value  interface{} `json:"value"`
}

type Monitorer struct {
	cd *Delta // Catchup Delta
	d  *Delta
	ds *astikit.DeltaStater
	mc *sync.Mutex // Locks cd
	md *sync.Mutex // Locks d
	o  MonitorerOptions
}

type OnDelta func(d Delta)

type MonitorerOptions struct {
	Flow    *astiflow.Flow
	OnDelta OnDelta
	Period  time.Duration
}

func New(o MonitorerOptions) *Monitorer {
	// Create monitorer
	m := &Monitorer{
		cd: newDelta(),
		d:  newDelta(),
		mc: &sync.Mutex{},
		md: &sync.Mutex{},
		o:  o,
	}

	// Create Delta stater
	m.ds = astikit.NewDeltaStater(astikit.DeltaStaterOptions{
		OnStats: m.onStats,
		Period:  o.Period,
	})

	// Monitor flow
	m.monitorFlow()
	return m
}

func (m *Monitorer) monitorFlow() {
	// Loop through flow delta stats
	for _, ds := range m.o.Flow.DeltaStats() {
		// Add to stater
		id := m.ds.Add(ds.Valuer)

		// Create Delta stat
		s := DeltaStat{
			ID:       id,
			Metadata: newDeltaStatMetadata(ds.Metadata),
		}

		// Store stat
		m.mc.Lock()
		m.cd.NewStats = append(m.cd.NewStats, s)
		m.mc.Unlock()
		m.md.Lock()
		m.d.NewStats = append(m.d.NewStats, s)
		m.md.Unlock()
	}

	// Listen to flow
	m.o.Flow.On(astiflow.EventNameGroupCreated, func(payload interface{}) (delete bool) {
		// Assert payload
		g, ok := payload.(*astiflow.Group)
		if !ok {
			return
		}

		// Monitor group
		m.monitorGroup(g)
		return
	})
}

func (m *Monitorer) monitorGroup(g *astiflow.Group) {
	// Listen to group
	g.On(astiflow.EventNameGroupRunning, func(payload interface{}) (delete bool) {
		// Create Delta group
		dg := DeltaGroup{
			ID:       g.ID(),
			Metadata: g.Metadata(),
		}

		// Store group
		m.mc.Lock()
		m.cd.StartedGroups = append(m.cd.StartedGroups, dg)
		m.mc.Unlock()
		m.md.Lock()
		m.d.StartedGroups = append(m.d.StartedGroups, dg)
		m.md.Unlock()
		return
	})
	g.On(astiflow.EventNameGroupDone, func(payload interface{}) (delete bool) {
		// Store group
		m.mc.Lock()
		for idx := 0; idx < len(m.cd.StartedGroups); idx++ {
			if m.cd.StartedGroups[idx].ID == g.ID() {
				m.cd.StartedGroups = append(m.cd.StartedGroups[:idx], m.cd.StartedGroups[idx+1:]...)
				idx--
			}
		}
		m.mc.Unlock()
		m.md.Lock()
		m.d.DoneGroups = append(m.d.DoneGroups, g.ID())
		m.md.Unlock()
		return
	})
	g.On(astiflow.EventNameNodeCreated, func(payload interface{}) (delete bool) {
		// Assert payload
		n, ok := payload.(*astiflow.Node)
		if !ok {
			return
		}

		// Monitor node
		m.monitorNode(g, n)
		return
	})
}

func (m *Monitorer) monitorNode(g *astiflow.Group, n *astiflow.Node) {
	// Loop through delta stats
	var statIDs []uint64
	for _, ds := range n.Noder().DeltaStats() {
		// Add to stater
		statID := m.ds.Add(ds.Valuer)

		// Store stat id
		statIDs = append(statIDs, statID)

		// Create Delta stat
		s := DeltaStat{
			ID:       statID,
			Metadata: newDeltaStatMetadata(ds.Metadata),
			NodeID:   astikit.UInt64Ptr(n.ID()),
		}

		// Store stat
		m.mc.Lock()
		m.cd.NewStats = append(m.cd.NewStats, s)
		m.mc.Unlock()
		m.md.Lock()
		m.d.NewStats = append(m.d.NewStats, s)
		m.md.Unlock()
	}

	// Loop through children
	for _, c := range n.Children() {
		// Store node connection
		m.storeNodeConnection(n, c)
	}

	// Listen to node
	n.On(astiflow.EventNameNodeRunning, func(payload interface{}) (delete bool) {
		// Create Delta node
		dn := DeltaNode{
			ID:       n.ID(),
			GroupID:  g.ID(),
			Metadata: n.Metadata(),
		}

		// Store node
		m.mc.Lock()
		m.cd.StartedNodes = append(m.cd.StartedNodes, dn)
		m.mc.Unlock()
		m.md.Lock()
		m.d.StartedNodes = append(m.d.StartedNodes, dn)
		m.md.Unlock()
		return
	})
	n.On(astiflow.EventNameNodeDone, func(payload interface{}) (delete bool) {
		// Remove stats
		m.mc.Lock()
		for _, statID := range statIDs {
			for idx := 0; idx < len(m.cd.NewStats); idx++ {
				if m.cd.NewStats[idx].ID == statID {
					m.cd.NewStats = append(m.cd.NewStats[:idx], m.cd.NewStats[idx+1:]...)
					idx--
				}
			}
		}
		m.mc.Unlock()
		m.ds.Remove(statIDs...)

		// Store node
		m.mc.Lock()
		for idx := 0; idx < len(m.cd.StartedNodes); idx++ {
			if m.cd.StartedNodes[idx].ID == n.ID() {
				m.cd.StartedNodes = append(m.cd.StartedNodes[:idx], m.cd.StartedNodes[idx+1:]...)
				idx--
			}
		}
		m.mc.Unlock()
		m.md.Lock()
		m.d.DoneNodes = append(m.d.DoneNodes, n.ID())
		m.md.Unlock()
		return
	})
	n.On(astiflow.EventNameNodeChildAdded, func(payload interface{}) (delete bool) {
		// Assert payload
		to, ok := payload.(*astiflow.Node)
		if !ok {
			return
		}

		// Store node connection
		m.storeNodeConnection(n, to)
		return
	})
	n.On(astiflow.EventNameNodeChildRemoved, func(payload interface{}) (delete bool) {
		// Assert payload
		to, ok := payload.(*astiflow.Node)
		if !ok {
			return
		}

		// Store disconnection
		m.mc.Lock()
		for idx := 0; idx < len(m.cd.ConnectedNodes); idx++ {
			if m.cd.ConnectedNodes[idx].From == n.ID() && m.cd.ConnectedNodes[idx].To == to.ID() {
				m.cd.ConnectedNodes = append(m.cd.ConnectedNodes[:idx], m.cd.ConnectedNodes[idx+1:]...)
				idx--
			}
		}
		m.mc.Unlock()
		m.md.Lock()
		m.d.DisconnectedNodes = append(m.d.DisconnectedNodes, DeltaConnection{
			From: n.ID(),
			To:   to.ID(),
		})
		m.md.Unlock()
		return
	})
}

func (m *Monitorer) storeNodeConnection(from, to *astiflow.Node) {
	// Create Delta connection
	dc := DeltaConnection{
		From: from.ID(),
		To:   to.ID(),
	}

	// Store connection
	m.mc.Lock()
	m.cd.ConnectedNodes = append(m.cd.ConnectedNodes, dc)
	m.mc.Unlock()
	m.md.Lock()
	m.d.ConnectedNodes = append(m.d.ConnectedNodes, dc)
	m.md.Unlock()
}

func (m *Monitorer) Start(ctx context.Context) {
	// Start stater
	m.ds.Start(ctx)
}

func (m *Monitorer) Close() {
	// Stop stater
	m.ds.Stop()
}

func (m *Monitorer) onStats(stats []astikit.DeltaStatValue) {
	// Swap Delta
	m.md.Lock()
	d := *m.d
	m.d = newDelta()
	m.md.Unlock()

	// Update at
	d.At = *astikit.NewTimestamp(astikit.Now())

	// Loop through stats
	m.cd.StatValues = map[uint64]interface{}{}
	for _, s := range stats {
		// Add
		d.StatValues[s.ID] = s.Value
		m.cd.StatValues[s.ID] = s.Value
	}

	// Callback
	if !d.empty() {
		m.o.OnDelta(d)
	}
}

func (m *Monitorer) CatchUp() Delta {
	// Lock
	m.mc.Lock()
	defer m.mc.Unlock()

	// Copy Delta
	d := m.cd.copy()

	// Update at
	d.At = *astikit.NewTimestamp(astikit.Now())
	return *d
}
