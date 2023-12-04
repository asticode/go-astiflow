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

type PacketHandler interface {
	HandlePacket(p Packet)
	NodeConnector() *astiflow.NodeConnector
	OnConnect(d PacketDescriptor, n astiflow.Noder) error
}

var _ PacketHandler = (*packetHandler)(nil)

type packetHandler struct {
	c                                 *astikit.Closer
	cpp                               *codecParametersPoolWithRereferenceCounter
	ch                                *astikit.Chan
	cs                                *packetHandlerCumulativeStats
	initialIncomingPacketDescriptors  *packetDescriptorPool
	n                                 *astiflow.Node
	onConnect                         packetHandlerOnConnect
	onPacket                          packetHandlerOnPacket
	pp                                *packetPool
	previousIncomingPacketDescriptors *packetDescriptorPool
}

type packetHandlerCumulativeStats struct {
	incomingBytes    uint64
	incomingPackets  uint64
	processedBytes   uint64
	processedPackets uint64
}

type packetHandlerOnConnect func(n astiflow.Noder) error

type packetHandlerOnPacket func(n astiflow.Noder, p *astiav.Packet, pd PacketDescriptor)

func newPacketHandler() *packetHandler {
	// Create packet handler
	h := &packetHandler{
		cpp: newCodecParametersPoolWithRereferenceCounter(),
		ch:  astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		cs:  &packetHandlerCumulativeStats{},
		pp:  newPacketPool(),
	}

	// Create packet descriptors
	h.initialIncomingPacketDescriptors = newPacketDescriptorPool(h.cpp)
	h.previousIncomingPacketDescriptors = newPacketDescriptorPool(h.cpp)
	return h
}

type packetHandlerInitOptions struct {
	c         *astikit.Closer
	n         *astiflow.Node
	onConnect packetHandlerOnConnect
	onPacket  packetHandlerOnPacket
}

func (h *packetHandler) init(o packetHandlerInitOptions) {
	// Update
	h.c = o.c
	h.cpp.init(o.c)
	h.n = o.n
	h.onConnect = o.onConnect
	h.onPacket = o.onPacket
	h.pp.init(h.c)

	// Make sure to close and remove initial descriptor when parent is removed
	h.n.On(astiflow.EventNameNodeParentRemoved, func(payload interface{}) (delete bool) {
		// Assert
		n, ok := payload.(*astiflow.Node)
		if !ok {
			return
		}

		// Remove descriptors
		h.initialIncomingPacketDescriptors.del(n.Noder())
		h.previousIncomingPacketDescriptors.del(n.Noder())
		return
	})
}

type PacketHandlerCumulativeStats struct {
	AllocatedCodecParameters uint64
	AllocatedPackets         uint64
	IncomingBytes            uint64
	IncomingPackets          uint64
	ProcessedBytes           uint64
	ProcessedPackets         uint64
	WorkedDuration           time.Duration
}

func (h *packetHandler) cumulativeStats() PacketHandlerCumulativeStats {
	return PacketHandlerCumulativeStats{
		AllocatedCodecParameters: atomic.LoadUint64(&h.cpp.cpp.cs.allocatedCodecParameters),
		AllocatedPackets:         atomic.LoadUint64(&h.pp.cs.allocatedPackets),
		IncomingBytes:            atomic.LoadUint64(&h.cs.incomingBytes),
		IncomingPackets:          atomic.LoadUint64(&h.cs.incomingPackets),
		ProcessedBytes:           atomic.LoadUint64(&h.cs.processedBytes),
		ProcessedPackets:         atomic.LoadUint64(&h.cs.processedPackets),
		WorkedDuration:           h.ch.CumulativeStats().WorkedDuration,
	}
}

func (h *packetHandler) deltaStats() []astikit.DeltaStat {
	ss := h.cpp.deltaStats()
	ss = append(ss, h.ch.DeltaStats()...)
	ss = append(ss, h.pp.deltaStats()...)
	ss = append(ss,
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of packets processed per second",
				Label:       "Processed rate",
				Name:        astiflow.DeltaStatNameProcessedRate,
				Unit:        "pps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&h.cs.processedPackets),
		},
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of bytes processed per second",
				Label:       "Processed byte rate",
				Name:        astiflow.DeltaStatNameProcessedByteRate,
				Unit:        "Bps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&h.cs.processedBytes),
		},
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of bytes coming in per second",
				Label:       "Incoming byte rate",
				Name:        astiflow.DeltaStatNameIncomingByteRate,
				Unit:        "Bps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&h.cs.incomingBytes),
		},
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of packets coming in per second",
				Label:       "Incoming rate",
				Name:        astiflow.DeltaStatNameIncomingRate,
				Unit:        "pps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&h.cs.incomingPackets),
		},
	)
	return ss
}

func (h *packetHandler) start(ctx context.Context) {
	// Make sure to stop the chan properly
	defer h.ch.Stop()

	// Start chan
	h.ch.Start(ctx)
}

func (h *packetHandler) HandlePacket(p Packet) {
	// Everything executed outside the main loop should be protected from the closer
	h.c.Do(func() {
		// Increment stats
		atomic.AddUint64(&h.cs.incomingBytes, uint64(p.Size()))
		atomic.AddUint64(&h.cs.incomingPackets, 1)

		// Copy packet descriptor
		pd, err := h.copyPacketDescriptor(p.PacketDescriptor)
		if err != nil {
			dispatchError(h.n, fmt.Errorf("astiavflow: copying packet descriptor failed: %w", err).Error())
			return
		}

		// Copy packet
		var pkt *astiav.Packet
		if pkt, err = h.pp.copy(p.Packet); err != nil {
			h.releasePacketDescriptor(pd)
			dispatchError(h.n, fmt.Errorf("astiavflow: copying packet failed: %w", err).Error())
			return
		}

		// Add to chan
		h.ch.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			h.c.Do(func() {
				// Make sure to close packet and release packet descriptor
				defer h.releasePacketDescriptor(pd)
				defer h.pp.put(pkt)

				// Increment stats
				atomic.AddUint64(&h.cs.processedBytes, uint64(pkt.Size()))
				atomic.AddUint64(&h.cs.processedPackets, 1)

				// Callback
				h.onPacket(p.Noder, pkt, pd)

				// Store descriptor
				h.previousIncomingPacketDescriptors.set(pd, p.Noder)
			})
		})
	})
}

func (h *packetHandler) copyPacketDescriptor(d PacketDescriptor) (PacketDescriptor, error) {
	cp, err := h.cpp.copy(d.CodecParameters)
	if err != nil {
		return PacketDescriptor{}, fmt.Errorf("astiavflow: copying codec parameter failed: %w", err)
	}
	return PacketDescriptor{
		CodecParameters: cp,
		MediaDescriptor: d.MediaDescriptor,
	}, nil
}

func (h *packetHandler) releasePacketDescriptor(d PacketDescriptor) {
	h.cpp.release(d.CodecParameters)
}

func (h *packetHandler) NodeConnector() *astiflow.NodeConnector {
	return h.n.Connector()
}

func (h *packetHandler) OnConnect(d PacketDescriptor, n astiflow.Noder) (err error) {
	// Initial incoming packet descriptor already exists
	if _, ok := h.initialIncomingPacketDescriptors.get(n); ok {
		err = errors.New("astiavflow: initial incoming packet descriptor already exists for that noder")
		return
	}

	// Copy packet descriptor
	var cd PacketDescriptor
	if cd, err = h.copyPacketDescriptor(d); err != nil {
		err = fmt.Errorf("astiavflow: copying packet descriptor failed: %w", err)
		return
	}

	// Store initial incoming packet descriptor
	h.initialIncomingPacketDescriptors.set(cd, n)

	// We can release the packet descriptor now it's been acquired by the pool
	h.releasePacketDescriptor(cd)

	// Make sure to remove initial incoming packet descriptor in case of error
	defer func() {
		if err != nil {
			h.initialIncomingPacketDescriptors.del(n)
		}
	}()

	// Callback
	if h.onConnect != nil {
		if err = h.onConnect(n); err != nil {
			err = fmt.Errorf("astiavflow: callback failed: %w", err)
			return
		}
	}
	return
}
