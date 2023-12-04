package astiavflow

import (
	"sync"
	"time"

	"github.com/asticode/go-astiav"
)

type TimeReference struct {
	m sync.Mutex
	// We use a timestamp in nanosecond timebase so that when sharing a time reference between streams with different
	// timebase there's no RescaleQ rounding issues
	t         time.Time
	timestamp int64
}

func NewTimeReference() *TimeReference {
	return &TimeReference{}
}

func (r *TimeReference) do(fn func()) {
	r.m.Lock()
	defer r.m.Unlock()
	fn()
}

func (r *TimeReference) isZeroUnsafe() bool {
	return r.t.IsZero()
}

func (r *TimeReference) timestampFromTimeUnsafe(t time.Time, timeBase astiav.Rational) int64 {
	return astiav.RescaleQ(int64(t.Sub(r.t))+r.timestamp, NanosecondRational, timeBase)
}

func (r *TimeReference) TimestampFromTime(t time.Time, timeBase astiav.Rational) int64 {
	r.m.Lock()
	defer r.m.Unlock()
	return r.timestampFromTimeUnsafe(t, timeBase)
}

func (r *TimeReference) timeFromTimestampUnsafe(timestamp int64, timeBase astiav.Rational) time.Time {
	return r.t.Add(time.Duration(astiav.RescaleQ(timestamp, timeBase, NanosecondRational) - r.timestamp))
}

func (r *TimeReference) TimeFromTimestamp(timestamp int64, timeBase astiav.Rational) time.Time {
	r.m.Lock()
	defer r.m.Unlock()
	return r.timeFromTimestampUnsafe(timestamp, timeBase)
}

func (r *TimeReference) updateUnsafe(timestamp int64, t time.Time, timeBase astiav.Rational) *TimeReference {
	r.timestamp = astiav.RescaleQ(timestamp, timeBase, NanosecondRational)
	r.t = t
	return r
}

func (r *TimeReference) Update(timestamp int64, t time.Time, timeBase astiav.Rational) *TimeReference {
	r.m.Lock()
	defer r.m.Unlock()
	return r.updateUnsafe(timestamp, t, timeBase)
}

func (r *TimeReference) AddToTime(d time.Duration) *TimeReference {
	r.m.Lock()
	defer r.m.Unlock()
	r.t = r.t.Add(d)
	return r
}
