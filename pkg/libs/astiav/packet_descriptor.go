package astiavflow

import (
	"sync"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
)

type PacketDescriptor struct {
	CodecParameters *astiav.CodecParameters
	MediaDescriptor MediaDescriptor
}

type packetDescriptorPool struct {
	cpp *codecParametersPoolWithRereferenceCounter
	m   sync.Mutex
	p   map[astiflow.Noder]PacketDescriptor
}

func newPacketDescriptorPool(cpp *codecParametersPoolWithRereferenceCounter) *packetDescriptorPool {
	return &packetDescriptorPool{
		cpp: cpp,
		p:   make(map[astiflow.Noder]PacketDescriptor),
	}
}

func (p *packetDescriptorPool) set(d PacketDescriptor, n astiflow.Noder) {
	// Lock
	p.m.Lock()
	defer p.m.Unlock()

	// Release previous codec parameters
	if v, ok := p.p[n]; ok {
		p.cpp.release(v.CodecParameters)
	}

	// Store
	p.p[n] = d

	// Acquire codec parameters
	p.cpp.acquire(d.CodecParameters)
}

func (p *packetDescriptorPool) get(n astiflow.Noder) (PacketDescriptor, bool) {
	// Lock
	p.m.Lock()
	defer p.m.Unlock()

	// Get
	d, ok := p.p[n]
	return d, ok
}

func (p *packetDescriptorPool) del(n astiflow.Noder) {
	// Lock
	p.m.Lock()
	defer p.m.Unlock()

	// Release codec parameters
	if v, ok := p.p[n]; ok {
		p.cpp.release(v.CodecParameters)
	}

	// Delete
	delete(p.p, n)
}

func (p *packetDescriptorPool) one() (PacketDescriptor, bool) {
	// Lock
	p.m.Lock()
	defer p.m.Unlock()

	// Get first descriptor found in map
	for _, d := range p.p {
		return d, true
	}
	return PacketDescriptor{}, false
}
