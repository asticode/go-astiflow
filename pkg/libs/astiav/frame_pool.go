package astiavflow

import (
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
)

type framePool struct {
	c  *astikit.Closer
	cs *framePoolCumulativeStats
	fs []*astiav.Frame
	mp sync.Mutex // Locks ps
}

type framePoolCumulativeStats struct {
	allocatedFrames uint64
}

func newFramePool() *framePool {
	return &framePool{cs: &framePoolCumulativeStats{}}
}

func (fp *framePool) init(c *astikit.Closer) *framePool {
	fp.c = c
	return fp
}

func (fp *framePool) get() (f *astiav.Frame) {
	// Lock
	fp.mp.Lock()
	defer fp.mp.Unlock()

	// Pool is empty
	if len(fp.fs) == 0 {
		// Allocate frame
		f = astiav.AllocFrame()

		// Increment allocated frames
		atomic.AddUint64(&fp.cs.allocatedFrames, 1)

		// Make sure frame is freed properly
		fp.c.Add(f.Free)
		return
	}

	// Use first frame in pool
	f = fp.fs[0]
	fp.fs = fp.fs[1:]
	return
}

func (fp *framePool) put(f *astiav.Frame) {
	// Lock
	fp.mp.Lock()
	defer fp.mp.Unlock()

	// Unref
	f.Unref()

	// Append
	fp.fs = append(fp.fs, f)
}

func (fp *framePool) copy(src *astiav.Frame) (f *astiav.Frame, err error) {
	f = fp.get()
	if err = f.Ref(src); err != nil {
		return
	}
	return
}

func (fp *framePool) deltaStats() []astikit.DeltaStat {
	return []astikit.DeltaStat{
		{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of allocated frames",
				Label:       "Allocated frames",
				Name:        DeltaStatNameAllocatedFrames,
				Unit:        "f",
			},
			Valuer: astikit.NewAtomicUint64CumulativeDeltaStat(&fp.cs.allocatedFrames),
		},
	}
}
