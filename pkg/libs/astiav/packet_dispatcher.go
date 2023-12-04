package astiavflow

import (
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

type packetDispatcher struct {
	cumulativeStats *packetDispatcherCumulativeStats
	ms              sync.Mutex // Locks skippers
	n               *astiflow.Node
	skippers        map[PacketHandler]PacketSkipper
}

type packetDispatcherCumulativeStats struct {
	outgoingBytes   uint64
	outgoingPackets uint64
}

func newPacketDispatcher() *packetDispatcher {
	return &packetDispatcher{
		cumulativeStats: &packetDispatcherCumulativeStats{},
		skippers:        make(map[PacketHandler]PacketSkipper),
	}
}

func (pd *packetDispatcher) init(n *astiflow.Node) *packetDispatcher {
	pd.n = n
	return pd
}

func (pd *packetDispatcher) setSkipper(h PacketHandler, s PacketSkipper) {
	// Lock
	pd.ms.Lock()
	defer pd.ms.Unlock()

	// Set
	pd.skippers[h] = s
}

func (pd *packetDispatcher) delSkipper(h PacketHandler) {
	// Lock
	pd.ms.Lock()
	defer pd.ms.Unlock()

	// Remove
	delete(pd.skippers, h)
}

func (pd *packetDispatcher) dispatch(pkt Packet) {
	// Update packet noder
	pkt.Noder = pd.n.Noder()

	// Increment stats
	atomic.AddUint64(&pd.cumulativeStats.outgoingBytes, uint64(pkt.Size()))
	atomic.AddUint64(&pd.cumulativeStats.outgoingPackets, 1)

	// Loop through children
	var hs []PacketHandler
	for _, n := range pd.n.Children() {
		// Assert
		h, ok := n.Noder().(PacketHandler)
		if !ok {
			continue
		}

		// Skip
		pd.ms.Lock()
		skip := pd.skippers[h]
		pd.ms.Unlock()
		if skip != nil && skip(pkt) {
			continue
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
		// Handle pkt
		h.HandlePacket(pkt)
	}
}

func (pd *packetDispatcher) deltaStats() []astikit.DeltaStat {
	return []astikit.DeltaStat{
		{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of packets going out per second",
				Label:       "Outgoing rate",
				Name:        astiflow.DeltaStatNameOutgoingRate,
				Unit:        "pps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&pd.cumulativeStats.outgoingPackets),
		},
		{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of bytes going out per second",
				Label:       "Outgoing byte rate",
				Name:        astiflow.DeltaStatNameOutgoingByteRate,
				Unit:        "Bps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&pd.cumulativeStats.outgoingBytes),
		},
	}
}
