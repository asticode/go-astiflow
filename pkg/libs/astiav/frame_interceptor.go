package astiavflow

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

var _ FrameHandler = (*FrameInterceptor)(nil)

var (
	countFrameInterceptor uint64
)

type FrameInterceptor struct {
	*frameHandler
	c       *astikit.Closer
	fd      *frameDispatcher
	n       *astiflow.Node
	onFrame FrameInterceptorOnFrame
}

type FrameInterceptorOnFrame func(d FrameDescriptor, f *astiav.Frame) (dispatch bool, err error)

type FrameInterceptorOptions struct {
	Group    *astiflow.Group
	Metadata astiflow.Metadata
	OnFrame  FrameInterceptorOnFrame
	Stop     *astiflow.NodeStopOptions
}

func NewFrameInterceptor(o FrameInterceptorOptions) (i *FrameInterceptor, err error) {
	// Create frame interceptor
	i = &FrameInterceptor{
		fd:           newFrameDispatcher(),
		frameHandler: newFrameHandler(),
		onFrame:      o.OnFrame,
	}

	// Make sure on frame is always set
	if i.onFrame == nil {
		i.onFrame = func(d FrameDescriptor, f *astiav.Frame) (dispatch bool, err error) { return }
	}

	// Create node
	if i.n, i.c, err = o.Group.NewNode(astiflow.NodeOptions{
		Metadata: (&astiflow.Metadata{
			Name: fmt.Sprintf("frame_interceptor_%d", atomic.AddUint64(&countFrameInterceptor, uint64(1))),
			Tags: []string{"frame_interceptor"},
		}).Merge(o.Metadata),
		Noder: i,
		Stop:  o.Stop,
	}); err != nil {
		err = fmt.Errorf("astiavflow: creating node failed: %w", err)
		return
	}

	// Initialize dispatchers and handlers
	i.fd.init(i.n)
	i.frameHandler.init(frameHandlerInitOptions{
		c:       i.c,
		n:       i.n,
		onFrame: i.onFrame_,
	})
	return
}

type FrameInterceptorCumulativeStats struct {
	FrameHandlerCumulativeStats
	OutgoingFrames uint64
}

func (i *FrameInterceptor) CumulativeStats() FrameInterceptorCumulativeStats {
	return FrameInterceptorCumulativeStats{
		FrameHandlerCumulativeStats: i.frameHandler.cumulativeStats(),
		OutgoingFrames:              atomic.LoadUint64(&i.fd.cs.outgoingFrames),
	}
}

func (i *FrameInterceptor) DeltaStats() []astikit.DeltaStat {
	ss := i.frameHandler.deltaStats()
	ss = append(ss, i.fd.deltaStats()...)
	return ss
}

func (i *FrameInterceptor) On(n astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return i.n.On(n, h)
}

func (i *FrameInterceptor) Connect(h FrameHandler) error {
	// Get initial outgoing frame descriptor
	fd, err := i.initialOutgoingFrameDescriptor()
	if err != nil {
		return fmt.Errorf("astiavflow: getting initial outgoing frame descriptor failed: %w: frame interceptor needs at least one parent before adding children", err)
	}

	// Callback
	if err := h.OnConnect(fd, i); err != nil {
		return fmt.Errorf("astiavflow: callback failed: %w", err)
	}

	// Connect
	i.n.Connector().Connect(h.NodeConnector())
	return nil
}

func (i *FrameInterceptor) initialOutgoingFrameDescriptor() (FrameDescriptor, error) {
	// Get one initial incoming frame descriptor
	fd, ok := i.initialIncomingFrameDescriptors.one()
	if !ok {
		return FrameDescriptor{}, errors.New("astiavflow: no initial incoming frame descriptor")
	}
	return fd, nil
}

func (i *FrameInterceptor) Disconnect(h FrameHandler) {
	// Disconnect
	i.n.Connector().Disconnect(h.NodeConnector())
}

func (i *FrameInterceptor) Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	tc().Do(func() {
		// Start frame handler
		i.frameHandler.start(ctx)
	})
}

func (i *FrameInterceptor) onFrame_(acquireFrameFunc frameHandlerAcquireFrameFunc, f *astiav.Frame, fd FrameDescriptor, n astiflow.Noder) {
	// Callback
	dispatch, err := i.onFrame(fd, f)
	if err != nil {
		dispatchError(i.n, fmt.Errorf("astiavflow: callback failed: %w", err).Error())
		return
	}

	// Dispatch frame
	if dispatch {
		i.fd.dispatch(Frame{
			Frame:           f,
			FrameDescriptor: fd,
		})
	}
}
