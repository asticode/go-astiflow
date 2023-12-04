package astiavflow

import (
	"sync/atomic"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

type frameDispatcher struct {
	cs *frameDispatcherCumulativeStats
	n  *astiflow.Node
}

type frameDispatcherCumulativeStats struct {
	outgoingFrames uint64
}

func newFrameDispatcher() *frameDispatcher {
	return &frameDispatcher{cs: &frameDispatcherCumulativeStats{}}
}

func (fd *frameDispatcher) init(n *astiflow.Node) *frameDispatcher {
	fd.n = n
	return fd
}

type frameDispatcherSkipper func(h FrameHandler) (skip bool)

func (fd *frameDispatcher) dispatch(f Frame, skippers ...frameDispatcherSkipper) {
	// Update frame noder
	f.Noder = fd.n.Noder()

	// Increment stats
	atomic.AddUint64(&fd.cs.outgoingFrames, 1)

	// Loop through children
	var hs []FrameHandler
	for _, n := range fd.n.Children() {
		// Assert
		h, ok := n.Noder().(FrameHandler)
		if !ok {
			continue
		}

		// Skip
		if len(skippers) > 0 {
			skip := false
			for _, f := range skippers {
				if skip = f(h); skip {
					break
				}
			}
			if skip {
				continue
			}
		}

		// Append
		hs = append(hs, h)
	}

	// No handlers
	if len(hs) == 0 {
		return
	}

	// Loop through handlers
	for _, h := range hs {
		// Handle frame
		h.HandleFrame(f)
	}
}

func (fd *frameDispatcher) deltaStats() []astikit.DeltaStat {
	return []astikit.DeltaStat{
		{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of frames going out per second",
				Label:       "Outgoing rate",
				Name:        astiflow.DeltaStatNameOutgoingRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&fd.cs.outgoingFrames),
		},
	}
}
