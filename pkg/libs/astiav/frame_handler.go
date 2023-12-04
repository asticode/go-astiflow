package astiavflow

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

type FrameHandler interface {
	HandleFrame(p Frame)
	NodeConnector() *astiflow.NodeConnector
	OnConnect(d FrameDescriptor, n astiflow.Noder) error
}

var _ FrameHandler = (*frameHandler)(nil)

type frameHandler struct {
	c                                *astikit.Closer
	ch                               *astikit.Chan
	cs                               *frameHandlerCumulativeStats
	fp                               *framePool
	ignoreIncomingFrames             bool
	initialIncomingFrameDescriptors  *frameDescriptorPool
	onConnect                        frameHandlerOnConnect
	onFrame                          frameHandlerOnFrame
	previousIncomingFrameDescriptors *frameDescriptorPool
	n                                *astiflow.Node
}

type frameHandlerCumulativeStats struct {
	incomingFrames  uint64
	processedFrames uint64
}

type frameHandlerOnConnect func(n astiflow.Noder) error

type frameHandlerOnFrame func(acquireFrameFunc frameHandlerAcquireFrameFunc, f *astiav.Frame, fd FrameDescriptor, n astiflow.Noder)

type frameHandlerAcquireFrameFunc func() frameHandlerReleaseFrameFunc

type frameHandlerReleaseFrameFunc func()

func newFrameHandler() *frameHandler {
	h := &frameHandler{
		ch:                               astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		cs:                               &frameHandlerCumulativeStats{},
		fp:                               newFramePool(),
		initialIncomingFrameDescriptors:  newFrameDescriptorPool(),
		previousIncomingFrameDescriptors: newFrameDescriptorPool(),
	}
	return h
}

type frameHandlerInitOptions struct {
	c         *astikit.Closer
	n         *astiflow.Node
	onConnect frameHandlerOnConnect
	onFrame   frameHandlerOnFrame
}

func (h *frameHandler) init(o frameHandlerInitOptions) {
	// Update
	h.c = o.c
	h.n = o.n
	h.onConnect = o.onConnect
	h.onFrame = o.onFrame
	h.fp.init(h.c)

	// Make sure to close and remove initial descriptor when parent is removed
	h.n.On(astiflow.EventNameNodeParentRemoved, func(payload interface{}) (delete bool) {
		// Assert
		n, ok := payload.(*astiflow.Node)
		if !ok {
			return
		}

		// Remove descriptors
		h.initialIncomingFrameDescriptors.del(n.Noder())
		h.previousIncomingFrameDescriptors.del(n.Noder())
		return
	})
}

type FrameHandlerCumulativeStats struct {
	AllocatedFrames uint64
	IncomingFrames  uint64
	ProcessedFrames uint64
	WorkedDuration  time.Duration
}

func (h *frameHandler) cumulativeStats() FrameHandlerCumulativeStats {
	return FrameHandlerCumulativeStats{
		AllocatedFrames: atomic.LoadUint64(&h.fp.cs.allocatedFrames),
		IncomingFrames:  atomic.LoadUint64(&h.cs.incomingFrames),
		ProcessedFrames: atomic.LoadUint64(&h.cs.processedFrames),
		WorkedDuration:  h.ch.CumulativeStats().WorkedDuration,
	}
}

func (h *frameHandler) deltaStats() []astikit.DeltaStat {
	ss := h.ch.DeltaStats()
	ss = append(ss, h.fp.deltaStats()...)
	ss = append(ss,
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of frames coming in per second",
				Label:       "Incoming rate",
				Name:        astiflow.DeltaStatNameIncomingRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&h.cs.incomingFrames),
		},
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of frames processed per second",
				Label:       "Processed rate",
				Name:        astiflow.DeltaStatNameProcessedRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&h.cs.processedFrames),
		},
	)
	return ss
}

func (h *frameHandler) IgnoreIncomingFrames(v bool) {
	h.ignoreIncomingFrames = v
}

func (h *frameHandler) start(ctx context.Context) {
	// Make sure to stop the chan properly
	defer h.ch.Stop()

	// Start chan
	h.ch.Start(ctx)
}

func (h *frameHandler) HandleFrame(f Frame) {
	// Everything executed outside the main loop should be protected from the closer
	h.c.Do(func() {
		// Increment stats
		atomic.AddUint64(&h.cs.incomingFrames, 1)

		// Frame should be ignored
		if h.ignoreIncomingFrames {
			return
		}

		// Copy
		fd := f.FrameDescriptor
		fm, err := h.fp.copy(f.Frame)
		if err != nil {
			dispatchError(h.n, fmt.Errorf("astiavflow: copying frame failed: %w", err).Error())
			return
		}

		// Add to chan
		h.ch.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			h.c.Do(func() {
				// Make sure to release frame
				ignoreReleaseFrame := false
				releaseFrameFunc := func() { h.fp.put(fm) }
				defer func() {
					if !ignoreReleaseFrame {
						releaseFrameFunc()
					}
				}()

				// Increment stats
				atomic.AddUint64(&h.cs.processedFrames, 1)

				// Callback
				h.onFrame(func() frameHandlerReleaseFrameFunc {
					ignoreReleaseFrame = true
					return releaseFrameFunc
				}, fm, fd, f.Noder)

				// Store previous incoming frame descriptor
				h.previousIncomingFrameDescriptors.set(fd, f.Noder)
			})
		})
	})
}

func (h *frameHandler) NodeConnector() *astiflow.NodeConnector {
	return h.n.Connector()
}

func (h *frameHandler) OnConnect(d FrameDescriptor, n astiflow.Noder) (err error) {
	// Initial incoming frame descriptor already exists
	if _, ok := h.initialIncomingFrameDescriptors.get(n); ok {
		err = errors.New("astiavflow: initial incoming frame descriptor already exists for that noder")
		return
	}

	// Store initial incoming frame descriptor
	h.initialIncomingFrameDescriptors.set(d, n)

	// Make sure to remove initial incoming frame descriptor in case of error
	defer func() {
		if err != nil {
			h.initialIncomingFrameDescriptors.del(n)
		}
	}()

	// Callback
	if h.onConnect != nil {
		if err = h.onConnect(n); err != nil {
			err = fmt.Errorf("astiavflow: callback failed: %w", err)
			return
		}
	}
	return nil
}
