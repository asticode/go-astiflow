package replay_test

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/asticode/go-astiflow/pkg/plugins/monitor/replay"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestPlugin(t *testing.T) {
	count := uint64(1)
	defer astikit.MockNow(func() time.Time {
		return time.Unix(int64(atomic.LoadUint64(&count)), 0)
	}).Close()

	w := astikit.NewWorker(astikit.WorkerOptions{})
	const path = "testdata/monitorer-replay.txt"
	f, err := astiflow.NewFlow(astiflow.FlowOptions{
		Metadata: astiflow.Metadata{
			Description: "Description",
			Name:        "Name",
		},
		Plugins: []astiflow.Plugin{replay.New(replay.PluginOptions{
			DeltaPeriod: time.Millisecond,
			Path:        path,
		})},
		Worker: w,
	})
	require.NoError(t, err)
	defer f.Close()

	gm := astiflow.Metadata{Name: "g"}
	g, err := f.NewGroup(astiflow.GroupOptions{Metadata: gm})
	require.NoError(t, err)

	sm := astikit.DeltaStatMetadata{Name: "n"}
	n := mocks.NewMockedNoder()
	n.OnDeltaStats = []astikit.DeltaStat{{
		Metadata: sm,
		Valuer: astikit.DeltaStatValuerFunc(func(d time.Duration) interface{} {
			w.Stop()
			return 1
		}),
	}}
	nm := astiflow.Metadata{Name: "n"}
	n.Node, _, err = g.NewNode(astiflow.NodeOptions{
		Metadata: nm,
		Noder:    n,
	})
	require.NoError(t, err)

	require.NoError(t, f.Start(w.Context()))

	require.Eventually(t, func() bool { return f.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)

	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, `{"flow":{"description":"Description","id":1,"name":"Name"}}
{"at":1,"new_stats":[{"id":1,"metadata":{"name":"n"},"node_id":1}],"started_groups":[{"id":1,"metadata":{"name":"g"}}],"started_nodes":[{"group_id":1,"id":1,"metadata":{"name":"n"}}],"stat_values":{"1":1}}
`, string(b))
}
