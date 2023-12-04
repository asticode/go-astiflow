package monitorer

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestMonitorer(t *testing.T) {
	count := uint64(1)
	defer astikit.MockNow(func() time.Time {
		return time.Unix(int64(atomic.LoadUint64(&count)), 0)
	}).Close()

	w := astikit.NewWorker(astikit.WorkerOptions{})
	sm1 := astikit.DeltaStatMetadata{Name: "n1"}
	f, err := astiflow.NewFlow(astiflow.FlowOptions{
		DeltaStats: []astikit.DeltaStat{{
			Metadata: sm1,
			Valuer:   astikit.DeltaStatValuerFunc(func(d time.Duration) interface{} { return int(atomic.LoadUint64(&count)) }),
		}},
		Worker: w,
	})
	require.NoError(t, err)
	defer f.Close()

	var deltas []Delta
	var catchupDeltas []Delta
	var g *astiflow.Group
	gm := astiflow.Metadata{Name: "g"}
	sm2 := astikit.DeltaStatMetadata{Name: "n2"}
	n1 := mocks.NewMockedNoder()
	n1.OnDeltaStats = []astikit.DeltaStat{{
		Metadata: sm2,
		Valuer:   astikit.DeltaStatValuerFunc(func(d time.Duration) interface{} { return int(atomic.LoadUint64(&count)) + 1 }),
	}}
	nm1 := astiflow.Metadata{Name: "n1"}
	sm3 := astikit.DeltaStatMetadata{Name: "n3"}
	n2 := mocks.NewMockedNoder()
	n2.OnDeltaStats = []astikit.DeltaStat{{
		Metadata: sm3,
		Valuer:   astikit.DeltaStatValuerFunc(func(d time.Duration) interface{} { return int(atomic.LoadUint64(&count)) + 2 }),
	}}
	nm2 := astiflow.Metadata{Name: "n2"}
	var m *Monitorer
	fn := func(d Delta) {
		// Sort
		astikit.SortUint64(d.DoneGroups)
		astikit.SortUint64(d.DoneNodes)

		// Store
		catchupDeltas = append(catchupDeltas, m.CatchUp())
		deltas = append(deltas, d)

		// Switch
		switch atomic.AddUint64(&count, 1) {
		case 2:
		case 3:
			g, err = f.NewGroup(astiflow.GroupOptions{Metadata: gm})
			require.NoError(t, err)
			n1.Node, _, err = g.NewNode(astiflow.NodeOptions{
				Metadata: nm1,
				Noder:    n1,
			})
			require.NoError(t, err)
			n2.Node, _, err = g.NewNode(astiflow.NodeOptions{
				Metadata: nm2,
				Noder:    n2,
			})
			require.NoError(t, err)
			n1.Node.Connector().Connect(n2.Node.Connector())
			require.NoError(t, f.Start(w.Context()))
		case 4:
			require.NoError(t, g.Stop())
			require.Eventually(t, func() bool { return g.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		default:
			w.Stop()
		}
	}
	m = New(MonitorerOptions{
		Flow:    f,
		OnDelta: fn,
		Period:  time.Millisecond,
	})
	defer m.Close()

	m.Start(w.Context())

	require.Eventually(t, func() bool { return f.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
	require.Equal(t, []Delta{
		{
			At: *astikit.NewTimestamp(time.Unix(1, 0)),
			NewStats: []DeltaStat{{
				ID:       1,
				Metadata: newDeltaStatMetadata(sm1),
			}},
			StatValues: map[uint64]interface{}{1: 1},
		},
		{
			At:         *astikit.NewTimestamp(time.Unix(2, 0)),
			StatValues: map[uint64]interface{}{1: 2},
		},
		{
			At: *astikit.NewTimestamp(time.Unix(3, 0)),
			ConnectedNodes: []DeltaConnection{{
				From: 1,
				To:   2,
			}},
			NewStats: []DeltaStat{
				{
					ID:       2,
					Metadata: newDeltaStatMetadata(sm2),
					NodeID:   astikit.UInt64Ptr(1),
				},
				{
					ID:       3,
					Metadata: newDeltaStatMetadata(sm3),
					NodeID:   astikit.UInt64Ptr(2),
				},
			},
			StartedGroups: []DeltaGroup{{
				ID:       1,
				Metadata: gm,
			}},
			StartedNodes: []DeltaNode{
				{
					ID:       1,
					GroupID:  1,
					Metadata: nm1,
				},
				{
					ID:       2,
					GroupID:  1,
					Metadata: nm2,
				},
			},
			StatValues: map[uint64]interface{}{
				1: 3,
				2: 4,
				3: 5,
			},
		},
		{
			At: *astikit.NewTimestamp(time.Unix(4, 0)),
			DisconnectedNodes: []DeltaConnection{{
				From: 1,
				To:   2,
			}},
			DoneGroups: []uint64{1},
			DoneNodes:  []uint64{1, 2},
			StatValues: map[uint64]interface{}{1: 4},
		},
	}, deltas)
	require.Equal(t, []Delta{
		{
			At: *astikit.NewTimestamp(time.Unix(1, 0)),
			NewStats: []DeltaStat{{
				ID:       1,
				Metadata: newDeltaStatMetadata(sm1),
			}},
			StatValues: map[uint64]interface{}{1: 1},
		},
		{
			At: *astikit.NewTimestamp(time.Unix(2, 0)),
			NewStats: []DeltaStat{{
				ID:       1,
				Metadata: newDeltaStatMetadata(sm1),
			}},
			StatValues: map[uint64]interface{}{1: 2},
		},
		{
			At: *astikit.NewTimestamp(time.Unix(3, 0)),
			ConnectedNodes: []DeltaConnection{{
				From: 1,
				To:   2,
			}},
			NewStats: []DeltaStat{
				{
					ID:       1,
					Metadata: newDeltaStatMetadata(sm1),
				},
				{
					ID:       2,
					Metadata: newDeltaStatMetadata(sm2),
					NodeID:   astikit.UInt64Ptr(1),
				},
				{
					ID:       3,
					Metadata: newDeltaStatMetadata(sm3),
					NodeID:   astikit.UInt64Ptr(2),
				},
			},
			StartedGroups: []DeltaGroup{{
				ID:       1,
				Metadata: gm,
			}},
			StartedNodes: []DeltaNode{
				{
					ID:       1,
					GroupID:  1,
					Metadata: nm1,
				},
				{
					ID:       2,
					GroupID:  1,
					Metadata: nm2,
				},
			},
			StatValues: map[uint64]interface{}{
				1: 3,
				2: 4,
				3: 5,
			},
		},
		{
			At: *astikit.NewTimestamp(time.Unix(4, 0)),
			NewStats: []DeltaStat{{
				ID:       1,
				Metadata: newDeltaStatMetadata(sm1),
			}},
			StatValues: map[uint64]interface{}{1: 4},
		},
	}, catchupDeltas)
}
