package psutil

import (
	"fmt"
	"os"
	"time"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/process"
)

func New() (astikit.DeltaStat, error) {
	// Create valuer
	vr, err := newValuer()
	if err != nil {
		return astikit.DeltaStat{}, fmt.Errorf("psutil: creating valuer failed: %w", err)
	}

	// Create delta stat
	return astikit.DeltaStat{
		Metadata: astikit.DeltaStatMetadata{Name: astiflow.DeltaStatNameHostUsage},
		Valuer:   vr,
	}, nil
}

var _ astikit.DeltaStatValuer = (*valuer)(nil)

type valuer struct {
	lastTimes *cpu.TimesStat
	p         *process.Process
}

func newValuer() (vr *valuer, err error) {
	// Create valuer
	vr = &valuer{}

	// Create process
	if vr.p, err = process.NewProcess(int32(os.Getpid())); err != nil {
		err = fmt.Errorf("psutil: creating process failed: %w", err)
		return
	}
	return
}

func (vr *valuer) Value(delta time.Duration) interface{} {
	// Get process CPU
	var v astiflow.DeltaStatHostUsageValue
	if t, err := vr.p.Times(); err == nil {
		if vr.lastTimes != nil {
			v.CPU.Process = astikit.Float64Ptr(((t.Total() - t.Idle) - (vr.lastTimes.Total() - vr.lastTimes.Idle)) / delta.Seconds() * 100)
		}
		vr.lastTimes = t
	}

	// Get global CPU
	if ps, err := cpu.Percent(0, true); err == nil {
		v.CPU.Individual = ps
	}
	if ps, err := cpu.Percent(0, false); err == nil && len(ps) > 0 {
		v.CPU.Total = ps[0]
	}

	// Get memory
	if i, err := vr.p.MemoryInfo(); err == nil {
		v.Memory.Resident = i.RSS
		v.Memory.Virtual = i.VMS
	}
	if s, err := mem.VirtualMemory(); err == nil {
		v.Memory.Total = s.Total
		v.Memory.Used = s.Used
	}
	return v
}
