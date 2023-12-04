package astiavflow

import (
	"sync/atomic"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

type packetDispatcher struct {
	cs *packetDispatcherCumulativeStats
	n  *astiflow.Node
}

type packetDispatcherCumulativeStats struct {
	outgoingBytes   uint64
	outgoingPackets uint64
}

func newPacketDispatcher() *packetDispatcher {
	return &packetDispatcher{cs: &packetDispatcherCumulativeStats{}}
}

func (pd *packetDispatcher) init(n *astiflow.Node) *packetDispatcher {
	pd.n = n
	return pd
}

type packetDispatcherSkipper func(h PacketHandler) (skip bool)

func (pd *packetDispatcher) dispatch(pkt Packet, skippers ...packetDispatcherSkipper) {
	// Update packet noder
	pkt.Noder = pd.n.Noder()

	// Increment stats
	atomic.AddUint64(&pd.cs.outgoingBytes, uint64(pkt.Size()))
	atomic.AddUint64(&pd.cs.outgoingPackets, 1)

	// Loop through children
	var hs []PacketHandler
	for _, n := range pd.n.Children() {
		// Assert
		h, ok := n.Noder().(PacketHandler)
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
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&pd.cs.outgoingPackets),
		},
		{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of bytes going out per second",
				Label:       "Outgoing byte rate",
				Name:        astiflow.DeltaStatNameOutgoingByteRate,
				Unit:        "Bps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&pd.cs.outgoingBytes),
		},
	}
}
