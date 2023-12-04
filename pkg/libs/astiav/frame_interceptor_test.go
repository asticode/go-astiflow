package astiavflow

import (
	"errors"
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestNewFrameInterceptor(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		countFrameInterceptor = 0
		i, err := NewFrameInterceptor(FrameInterceptorOptions{Group: g})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{Name: "frame_interceptor_1", Tags: []string{"frame_interceptor"}}, i.n.Metadata())
		var emitted bool
		i.On(astiflow.EventNameNodeClosed, func(payload interface{}) (delete bool) {
			emitted = true
			return
		})
		g.Close()
		require.True(t, emitted)
	})

	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		i, err := NewFrameInterceptor(FrameInterceptorOptions{
			Group:    g,
			Metadata: astiflow.Metadata{Description: "d", Name: "n", Tags: []string{"t"}},
		})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{
			Description: "d",
			Name:        "n",
			Tags:        []string{"frame_interceptor", "t"},
		}, i.n.Metadata())
	})
}

func TestFrameInterceptorOnConnect(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		i, err := NewFrameInterceptor(FrameInterceptorOptions{Group: g})
		require.NoError(t, err)

		fd := FrameDescriptor{}
		require.NoError(t, i.OnConnect(fd, nil))
	})
}

func TestFrameInterceptorConnect(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		h := newMockedFrameHandler()
		var err error
		h.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h})
		require.NoError(t, err)

		i, err := NewFrameInterceptor(FrameInterceptorOptions{Group: g})
		require.NoError(t, err)

		e := errors.New("test")
		h.onConnect = func(d FrameDescriptor, n astiflow.Noder) error { return e }
		err = i.Connect(h)
		require.Error(t, err)
		require.NotErrorIs(t, err, e)

		afd := FrameDescriptor{Height: 1}
		require.NoError(t, i.OnConnect(afd, nil))
		require.ErrorIs(t, i.Connect(h), e)

		var efd FrameDescriptor
		h.onConnect = func(d FrameDescriptor, n astiflow.Noder) error {
			efd = d
			return nil
		}
		require.NoError(t, i.Connect(h))
		require.Equal(t, 1, efd.Height)
	})
}

func TestFrameInterceptorStart(t *testing.T) {
	// Callback should be handled properly, stats should be correct
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		type frame struct {
			fd  FrameDescriptor
			n   astiflow.Noder
			pts int64
		}
		fs1 := []frame{}
		fh1 := newMockedFrameHandler()
		fh1.onFrame = func(f Frame) {
			fs1 = append(fs1, frame{
				fd:  f.FrameDescriptor,
				n:   f.Noder,
				pts: f.Pts(),
			})
		}
		var err error
		fh1.Node, _, err = g.NewNode(astiflow.NodeOptions{
			Noder: fh1,
			Stop:  &astiflow.NodeStopOptions{WhenAllParentsAreDone: true},
		})
		require.NoError(t, err)
		fs2 := []frame{}
		fh2 := newMockedFrameHandler()
		fh2.onFrame = func(f Frame) {
			fs2 = append(fs2, frame{
				fd:  f.FrameDescriptor,
				n:   f.Noder,
				pts: f.Pts(),
			})
		}
		fh2.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: fh2})
		require.NoError(t, err)

		count := 0
		var i *FrameInterceptor
		i, err = NewFrameInterceptor(FrameInterceptorOptions{
			Group: g,
			OnFrame: func(d FrameDescriptor, f *astiav.Frame) (dispatch bool, err error) {
				count++
				f.SetPts(int64(count))
				switch count {
				case 1:
					return false, nil
				case 2:
					return true, errors.New("test")
				case 3:
					return true, nil
				case 4:
					i.Disconnect(fh2)
					return true, nil
				default:
					require.NoError(t, g.Stop())
					return true, nil
				}
			},
		})
		require.NoError(t, err)

		dss := i.DeltaStats()

		md := MediaDescriptor{
			FrameRate: astiav.NewRational(2, 1),
			TimeBase:  astiav.NewRational(1, 10),
		}
		fm := astiav.AllocFrame()
		defer fm.Free()
		fm.SetHeight(2)
		fm.SetPixelFormat(astiav.PixelFormatRgba)
		fm.SetWidth(2)
		require.NoError(t, fm.AllocBuffer(0))
		fd := newFrameDescriptorFromFrame(fm, md, astiav.MediaTypeVideo)
		require.NoError(t, i.OnConnect(fd, nil))
		require.NoError(t, i.Connect(fh1))
		require.NoError(t, i.Connect(fh2))

		for idx := 0; idx < 5; idx++ {
			i.HandleFrame(Frame{
				Frame:           fm,
				FrameDescriptor: fd,
			})
		}

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return i.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		cs := i.CumulativeStats()
		require.Greater(t, cs.WorkedDuration, time.Duration(0))
		cs.WorkedDuration = 0
		require.Equal(t, FrameInterceptorCumulativeStats{
			FrameHandlerCumulativeStats: FrameHandlerCumulativeStats{
				AllocatedFrames: 5,
				IncomingFrames:  5,
				ProcessedFrames: 5,
			},
			OutgoingFrames: 3,
		}, cs)
		requireDeltaStats(t, map[string]interface{}{
			DeltaStatNameAllocatedFrames:        uint64(5),
			astiflow.DeltaStatNameIncomingRate:  float64(5),
			astiflow.DeltaStatNameOutgoingRate:  float64(3),
			astiflow.DeltaStatNameProcessedRate: float64(5),
			astikit.StatNameWorkedRatio:         nil,
		}, dss)
		require.Equal(t, []frame{
			{
				fd:  fd,
				n:   i,
				pts: 3,
			},
			{
				fd:  fd,
				n:   i,
				pts: 4,
			},
			{
				fd:  fd,
				n:   i,
				pts: 5,
			},
		}, fs1)
		require.Equal(t, []frame{
			{
				fd:  fd,
				n:   i,
				pts: 3,
			},
		}, fs2)
	})
}
