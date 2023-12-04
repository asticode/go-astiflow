package astiflow_test

import (
	"testing"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestPlugin(t *testing.T) {
	w := astikit.NewWorker(astikit.WorkerOptions{})
	defer w.Stop()
	p := mocks.NewMockedPlugin()
	f, err := astiflow.NewFlow(astiflow.FlowOptions{
		Plugins: []astiflow.Plugin{p},
		Worker:  w,
	})
	require.NoError(t, err)
	defer f.Close()
	require.True(t, p.Initialized)
	require.False(t, p.Started)
	require.NoError(t, f.Start(w.Context()))
	require.True(t, p.Started)

	_, err = astiflow.NewFlow(astiflow.FlowOptions{
		Plugins: []astiflow.Plugin{p},
		Worker:  w,
	})
	require.Error(t, err)
}
