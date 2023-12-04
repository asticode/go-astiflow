package astiavflow

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestCodecParametersPool(t *testing.T) {
	c := astikit.NewCloser()
	defer c.Close()

	cpp := newCodecParametersPool()
	cpp.init(c)

	dss := cpp.deltaStats()

	cp1 := cpp.get()
	require.Len(t, cpp.cps, 0)
	require.NotNil(t, cp1)
	requireDeltaStats(t, map[string]interface{}{DeltaStatNameAllocatedCodecParameters: uint64(1)}, dss)

	cp2 := cpp.get()
	require.Len(t, cpp.cps, 0)
	require.NotNil(t, cp2)
	require.NotSame(t, cp1, cp2)
	requireDeltaStats(t, map[string]interface{}{DeltaStatNameAllocatedCodecParameters: uint64(2)}, dss)

	cpp.put(cp1)
	require.Len(t, cpp.cps, 1)
	requireDeltaStats(t, map[string]interface{}{DeltaStatNameAllocatedCodecParameters: uint64(2)}, dss)

	cp3 := cpp.get()
	require.Len(t, cpp.cps, 0)
	require.Equal(t, cp1, cp3)
	requireDeltaStats(t, map[string]interface{}{DeltaStatNameAllocatedCodecParameters: uint64(2)}, dss)

	c.Close()
	require.Panics(t, func() { cp1.CodecID() })

	require.Equal(t, codecParametersPoolCumulativeStats{
		allocatedCodecParameters: 2,
	}, *cpp.cs)
}

func TestCodecParametersPoolWithRereferenceCounter(t *testing.T) {
	c := astikit.NewCloser()
	defer c.Close()

	cpp := newCodecParametersPoolWithRereferenceCounter()
	cpp.init(c)

	cp1 := astiav.AllocCodecParameters()
	defer cp1.Free()
	cp1.SetCodecID(astiav.CodecIDMjpeg)
	cp2, err := cpp.copy(cp1)
	require.NoError(t, err)
	require.Equal(t, astiav.CodecIDMjpeg, cp2.CodecID())

	cpp.acquire(cp2)
	cpp.release(cp2)
	require.Len(t, cpp.cpp.cps, 0)
	cpp.release(cp2)
	require.Len(t, cpp.cpp.cps, 1)
}
