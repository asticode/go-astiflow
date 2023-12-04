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

func (fd *frameDispatcher) dispatch(f Frame) {
	// Update frame noder
	f.Noder = fd.n.Noder()

	// Increment stats
	atomic.AddUint64(&fd.cs.outgoingFrames, 1)

	// Loop through children
	for _, n := range fd.n.Children() {
		// Assert
		h, ok := n.Noder().(FrameHandler)
		if !ok {
			continue
		}

		// Handle f
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
