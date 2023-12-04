package astiavflow

import (
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
)

type packetPool struct {
	c  *astikit.Closer
	cs *packetPoolCumulativeStats
	mp sync.Mutex // Locks ps
	ps []*astiav.Packet
}

type packetPoolCumulativeStats struct {
	allocatedPackets uint64
}

func newPacketPool() *packetPool {
	return &packetPool{cs: &packetPoolCumulativeStats{}}
}

func (pp *packetPool) init(c *astikit.Closer) *packetPool {
	pp.c = c
	return pp
}

func (pp *packetPool) get() (pkt *astiav.Packet) {
	// Lock
	pp.mp.Lock()
	defer pp.mp.Unlock()

	// Pool is empty
	if len(pp.ps) == 0 {
		// Allocate packet
		pkt = astiav.AllocPacket()

		// Increment allocated packets
		atomic.AddUint64(&pp.cs.allocatedPackets, 1)

		// Make sure packet is freed properly
		pp.c.Add(pkt.Free)
		return
	}

	// Use first packet in pool
	pkt = pp.ps[0]
	pp.ps = pp.ps[1:]
	return
}

func (pp *packetPool) copy(src *astiav.Packet) (pkt *astiav.Packet, err error) {
	pkt = pp.get()
	if err = pkt.Ref(src); err != nil {
		return
	}
	return
}

func (pp *packetPool) put(pkt *astiav.Packet) {
	// Lock
	pp.mp.Lock()
	defer pp.mp.Unlock()

	// Unref
	pkt.Unref()

	// Append
	pp.ps = append(pp.ps, pkt)
}

func (pp *packetPool) deltaStats() []astikit.DeltaStat {
	return []astikit.DeltaStat{
		{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of allocated packets",
				Label:       "Allocated packets",
				Name:        DeltaStatNameAllocatedPackets,
				Unit:        "p",
			},
			Valuer: astikit.NewAtomicUint64CumulativeDeltaStat(&pp.cs.allocatedPackets),
		},
	}
}
