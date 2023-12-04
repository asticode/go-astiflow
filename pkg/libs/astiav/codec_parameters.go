package astiavflow

import (
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
)

type codecParametersPool struct {
	c   *astikit.Closer
	cps []*astiav.CodecParameters
	cs  *codecParametersPoolCumulativeStats
	m   sync.Mutex // Locks cps
}

type codecParametersPoolCumulativeStats struct {
	allocatedCodecParameters uint64
}

func newCodecParametersPool() *codecParametersPool {
	return &codecParametersPool{cs: &codecParametersPoolCumulativeStats{}}
}

func (cpp *codecParametersPool) init(c *astikit.Closer) {
	cpp.c = c
}

func (cpp *codecParametersPool) get() (cp *astiav.CodecParameters) {
	// Lock
	cpp.m.Lock()
	defer cpp.m.Unlock()

	// Pool is empty
	if len(cpp.cps) == 0 {
		// Allocate codec parameters
		cp = astiav.AllocCodecParameters()

		// Increment allocated codec parameters
		atomic.AddUint64(&cpp.cs.allocatedCodecParameters, 1)

		// Make sure codec parameters is freed properly
		cpp.c.Add(cp.Free)
		return
	}

	// Use first codec parameters in pool
	cp = cpp.cps[0]
	cpp.cps = cpp.cps[1:]
	return
}

func (cpp *codecParametersPool) put(cp *astiav.CodecParameters) {
	// Lock
	cpp.m.Lock()
	defer cpp.m.Unlock()

	// Append
	cpp.cps = append(cpp.cps, cp)
}

func (cpp *codecParametersPool) deltaStats() []astikit.DeltaStat {
	return []astikit.DeltaStat{
		{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of allocated codec parameterss",
				Label:       "Allocated codec parameterss",
				Name:        DeltaStatNameAllocatedCodecParameters,
				Unit:        "cp",
			},
			Valuer: astikit.NewAtomicUint64CumulativeDeltaStat(&cpp.cs.allocatedCodecParameters),
		},
	}
}

type codecParametersPoolWithRereferenceCounter struct {
	cpp *codecParametersPool
	m   sync.Mutex
	p   map[*astiav.CodecParameters]int
}

func newCodecParametersPoolWithRereferenceCounter() *codecParametersPoolWithRereferenceCounter {
	return &codecParametersPoolWithRereferenceCounter{
		cpp: newCodecParametersPool(),
		p:   make(map[*astiav.CodecParameters]int),
	}
}

func (p *codecParametersPoolWithRereferenceCounter) init(c *astikit.Closer) {
	p.cpp.init(c)
}

func (p *codecParametersPoolWithRereferenceCounter) copy(src *astiav.CodecParameters) (cp *astiav.CodecParameters, err error) {
	// Get codec parameters
	cp = p.cpp.get()

	// Acquire
	p.acquire(cp)

	// Make sure codec parameters is released in case of error
	defer func() {
		if err != nil {
			p.release(cp)
		}
	}()

	// Copy
	if err = src.Copy(cp); err != nil {
		return
	}
	return
}

func (p *codecParametersPoolWithRereferenceCounter) acquire(cp *astiav.CodecParameters) {
	// Lock
	p.m.Lock()
	defer p.m.Unlock()

	// Increment
	p.p[cp]++
}

func (p *codecParametersPoolWithRereferenceCounter) release(cp *astiav.CodecParameters) {
	// Lock
	p.m.Lock()
	defer p.m.Unlock()

	// Decrement
	p.p[cp]--

	// Put
	if count, ok := p.p[cp]; ok && count <= 0 {
		p.cpp.put(cp)
	}
}

func (p *codecParametersPoolWithRereferenceCounter) deltaStats() []astikit.DeltaStat {
	return p.cpp.deltaStats()
}
