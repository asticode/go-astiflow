package astiavflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

type mockedFrameHandler struct {
	*mocks.MockedNoder
	onConnect func(d FrameDescriptor, n astiflow.Noder) error
	onFrame   func(f Frame)
}

var _ FrameHandler = (*mockedFrameHandler)(nil)

func newMockedFrameHandler() *mockedFrameHandler {
	return &mockedFrameHandler{
		MockedNoder: mocks.NewMockedNoder(),
	}
}

func (h *mockedFrameHandler) HandleFrame(f Frame) {
	if h.onFrame != nil {
		h.onFrame(f)
	}
}

func (h *mockedFrameHandler) NodeConnector() *astiflow.NodeConnector {
	return h.Node.Connector()
}

func (h *mockedFrameHandler) OnConnect(d FrameDescriptor, n astiflow.Noder) error {
	if h.onConnect != nil {
		return h.onConnect(d, n)
	}
	return nil
}

func TestFrameHandler(t *testing.T) {
	// Connect works properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		nr1 := mocks.NewMockedNoder()
		n1, _, err := g.NewNode(astiflow.NodeOptions{Noder: nr1})
		require.NoError(t, err)
		nr2 := mocks.NewMockedNoder()
		n2, c, err := g.NewNode(astiflow.NodeOptions{Noder: nr2})
		require.NoError(t, err)

		count := 0
		fd := FrameDescriptor{Height: 1}
		var cn astiflow.Noder
		h := newFrameHandler()
		h.init(frameHandlerInitOptions{
			c: c,
			n: n2,
			onConnect: func(n astiflow.Noder) error {
				count++
				switch count {
				case 1:
					return errors.New("test")
				default:
					cn = n
				}
				return nil
			},
		})

		n1.Connector().Connect(h.NodeConnector())
		require.Error(t, h.OnConnect(fd, nr1))
		require.Equal(t, 0, h.initialIncomingFrameDescriptors.len())
		require.NoError(t, h.OnConnect(fd, nr1))
		require.Equal(t, nr1, cn)
		v, ok := h.initialIncomingFrameDescriptors.get(nr1)
		require.True(t, ok)
		require.Equal(t, fd, v)
		require.Error(t, h.OnConnect(fd, nr1))
		n1.Connector().Disconnect(h.NodeConnector())
		_, ok = h.initialIncomingFrameDescriptors.get(nr1)
		require.False(t, ok)
	})

	// Handling frames works properly, stats are correct, frame is copied and closed properly,
	// ignore incoming frames works properly, acquiring frame works properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		nr1 := mocks.NewMockedNoder()
		nr2 := mocks.NewMockedNoder()
		n, c, err := g.NewNode(astiflow.NodeOptions{Noder: nr2})
		require.NoError(t, err)

		fm := astiav.AllocFrame()
		defer fm.Free()
		fm.SetHeight(1)
		fm.SetPixelFormat(astiav.PixelFormatRgba)
		fm.SetWidth(1)
		require.NoError(t, fm.AllocBuffer(0))

		h := newFrameHandler()
		count := 0
		type frame struct {
			cd   FrameDescriptor
			n    astiflow.Noder
			pd   FrameDescriptor
			pts  int64
			same bool
		}
		fs := []frame{}
		var releaseFrameFunc func()
		h.init(frameHandlerInitOptions{
			c: c,
			n: n,
			onFrame: func(acquireFrameFunc frameHandlerAcquireFrameFunc, i *astiav.Frame, fd FrameDescriptor, n astiflow.Noder) {
				count++
				f := frame{
					n:    n,
					pts:  i.Pts(),
					same: fm == i,
				}
				f.cd = fd
				if fd, ok := h.previousIncomingFrameDescriptors.get(n); ok {
					f.pd = fd
				}
				fs = append(fs, f)
				switch count {
				case 2:
					releaseFrameFunc = acquireFrameFunc()
				case 3:
					require.Len(t, h.fp.fs, 1)
					releaseFrameFunc()
					require.Len(t, h.fp.fs, 2)
					require.NoError(t, g.Stop())
				}
			},
		})
		nr2.OnStart = func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
			tc().Do(func() { h.start(ctx) })
		}
		dss := h.deltaStats()

		fd1 := FrameDescriptor{Height: 1}
		fd2 := FrameDescriptor{Height: 2}
		for _, v := range []struct {
			fd                   FrameDescriptor
			ignoreIncomingFrames bool
			pts                  int64
		}{
			{
				ignoreIncomingFrames: true,
			},
			{
				fd:  fd1,
				pts: 1,
			},
			{
				fd:  fd2,
				pts: 2,
			},
			{
				ignoreIncomingFrames: true,
				pts:                  3,
			},
			{
				fd:  fd1,
				pts: 4,
			},
		} {
			h.IgnoreIncomingFrames(v.ignoreIncomingFrames)
			fm.SetPts(v.pts)
			h.HandleFrame(Frame{
				Frame:           fm,
				FrameDescriptor: v.fd,
				Noder:           nr1,
			})
		}

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		cs := h.cumulativeStats()
		require.Greater(t, cs.WorkedDuration, time.Duration(0))
		cs.WorkedDuration = 0
		require.Equal(t, FrameHandlerCumulativeStats{
			AllocatedFrames: 3,
			IncomingFrames:  5,
			ProcessedFrames: 3,
		}, cs)
		requireDeltaStats(t, map[string]interface{}{
			DeltaStatNameAllocatedFrames:        uint64(3),
			astiflow.DeltaStatNameIncomingRate:  float64(5),
			astiflow.DeltaStatNameProcessedRate: float64(3),
			astikit.StatNameWorkedRatio:         nil,
		}, dss)
		require.Equal(t, []frame{
			{
				cd:   fd1,
				n:    nr1,
				pts:  1,
				same: false,
			},
			{
				cd:   fd2,
				n:    nr1,
				pd:   fd1,
				pts:  2,
				same: false,
			},
			{
				cd:   fd1,
				n:    nr1,
				pd:   fd2,
				pts:  4,
				same: false,
			},
		}, fs)
	})

	// Handling frame is protected from the closer
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		nr1 := mocks.NewMockedNoder()
		n1, c1, err := g.NewNode(astiflow.NodeOptions{Noder: nr1})
		require.NoError(t, err)
		nr2 := mocks.NewMockedNoder()
		n2, c2, err := g.NewNode(astiflow.NodeOptions{Noder: nr2})
		require.NoError(t, err)
		nr3 := mocks.NewMockedNoder()
		n3, c3, err := g.NewNode(astiflow.NodeOptions{Noder: nr3})
		require.NoError(t, err)

		fm := astiav.AllocFrame()
		defer fm.Free()
		fm.SetHeight(1)
		fm.SetPixelFormat(astiav.PixelFormatRgba)
		fm.SetWidth(1)
		require.NoError(t, fm.AllocBuffer(0))

		h1 := newFrameHandler()
		count1 := 0
		h1.init(frameHandlerInitOptions{
			c: c1,
			n: n1,
			onFrame: func(acquireFrame frameHandlerAcquireFrameFunc, f *astiav.Frame, fd FrameDescriptor, n astiflow.Noder) {
				count1++
			},
		})
		nr1.OnStart = func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
			tc().Do(func() { h1.start(ctx) })
		}
		h2 := newFrameHandler()
		count2 := 0
		h2.init(frameHandlerInitOptions{
			c: c2,
			n: n2,
			onFrame: func(acquireFrame frameHandlerAcquireFrameFunc, f *astiav.Frame, fd FrameDescriptor, n astiflow.Noder) {
				count2++
			},
		})
		nr2.OnStart = func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
			tc().Do(func() { h2.start(ctx) })
		}
		h3 := newFrameHandler()
		count3 := 0
		h3.init(frameHandlerInitOptions{
			c: c3, n: n3,
			onFrame: func(acquireFrame frameHandlerAcquireFrameFunc, f *astiav.Frame, fd FrameDescriptor, n astiflow.Noder) {
				count3++
				require.NoError(t, g.Stop())
			},
		})
		nr3.OnStart = func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
			tc().Do(func() { h3.start(ctx) })
		}

		c1.Close()
		for _, h := range []*frameHandler{h1, h2, h3} {
			h.HandleFrame(Frame{Frame: fm})
		}

		c2.Close()
		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return n1.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Eventually(t, func() bool { return n2.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Eventually(t, func() bool { return n3.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Equal(t, 0, count1)
		require.Equal(t, 0, count2)
		require.Equal(t, 1, count3)
	})
}
