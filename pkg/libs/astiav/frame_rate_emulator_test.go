package astiavflow

import (
	"errors"
	"testing"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestNewFrameRateEmulator(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		countFrameRateEmulator = 0
		fre, err := NewFrameRateEmulator(FrameRateEmulatorOptions{Group: g})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{Name: "frame_rate_emulator_1", Tags: []string{"frame_rate_emulator"}}, fre.n.Metadata())
		var emitted bool
		fre.On(astiflow.EventNameNodeClosed, func(payload interface{}) (delete bool) {
			emitted = true
			return
		})
		g.Close()
		require.True(t, emitted)
	})

	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		fre, err := NewFrameRateEmulator(FrameRateEmulatorOptions{
			Group:    g,
			Metadata: astiflow.Metadata{Description: "d", Name: "n", Tags: []string{"t"}},
		})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{
			Description: "d",
			Name:        "n",
			Tags:        []string{"frame_rate_emulator", "t"},
		}, fre.n.Metadata())
	})
}

func TestFrameRateEmulatorOnConnect(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		fre, err := NewFrameRateEmulator(FrameRateEmulatorOptions{Group: g})
		require.NoError(t, err)
		require.NoError(t, fre.OnConnect(FrameDescriptor{}, nil))
	})
}

func TestFrameRateEmulatorConnect(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		h1 := newMockedFrameHandler()
		var err error
		h1.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h1})
		require.NoError(t, err)
		h2 := newMockedFrameHandler()
		h2.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h2})
		require.NoError(t, err)

		fre, err := NewFrameRateEmulator(FrameRateEmulatorOptions{Group: g})
		require.NoError(t, err)

		e := errors.New("test")
		h2.onConnect = func(d FrameDescriptor, n astiflow.Noder) error { return e }
		err = fre.Connect(h2, h1)
		require.Error(t, err)
		require.NotErrorIs(t, err, e)

		afd := FrameDescriptor{Height: 1}
		require.NoError(t, fre.OnConnect(afd, h1))
		require.ErrorIs(t, fre.Connect(h2, h1), e)

		var efd FrameDescriptor
		h2.onConnect = func(d FrameDescriptor, n astiflow.Noder) error {
			efd = d
			return nil
		}
		require.NoError(t, fre.Connect(h2, h1))
		require.Equal(t, 1, efd.Height)
		require.Equal(t, 1, len(fre.hns))
		fre.Disconnect(h2)
		require.Equal(t, 0, len(fre.hns))
	})
}

// TODO Test
