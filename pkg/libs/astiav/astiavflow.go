package astiavflow

import (
	"fmt"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

var (
	NanosecondRational = astiav.NewRational(1, 1e9)
)

const (
	DeltaStatNameAllocatedCodecParameters = "astiavflow.allocated.codec.parameters"
	DeltaStatNameAllocatedFrames          = "astiavflow.allocated.frames"
	DeltaStatNameAllocatedPackets         = "astiavflow.allocated.packets"
)

func dispatchError(n *astiflow.Node, format string, args ...interface{}) {
	// Create message
	msg := fmt.Sprintf(format, args...)

	// Get log interceptor
	if li, ok := logInterceptors.get(n); ok {
		li.write(n.Context(), astikit.LoggerLevelWarn, format, msg)
		return
	}

	// Log
	n.Logger().WarnC(n.Context(), msg)
}

func durationToTimeBase(d time.Duration, t astiav.Rational) (i int64, r time.Duration) {
	// Get duration expressed in stream timebase
	// We need to make sure it's rounded to the nearest smaller int
	i = astiav.RescaleQRnd(d.Nanoseconds(), NanosecondRational, t, astiav.RoundingDown)

	// Update remainder
	r = d - time.Duration(astiav.RescaleQ(i, t, NanosecondRational))
	return
}
