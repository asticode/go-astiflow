package astiavflow

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestFramePool(t *testing.T) {
	c := astikit.NewCloser()
	fp := newFramePool().init(c)

	dss := fp.deltaStats()

	f1 := fp.get()
	require.Len(t, fp.fs, 0)
	require.NotNil(t, f1)
	f1.SetWidth(1)
	requireDeltaStats(t, map[string]interface{}{DeltaStatNameAllocatedFrames: uint64(1)}, dss)

	f2 := fp.get()
	require.Len(t, fp.fs, 0)
	require.NotNil(t, f2)
	require.NotSame(t, f1, f2)
	requireDeltaStats(t, map[string]interface{}{DeltaStatNameAllocatedFrames: uint64(2)}, dss)

	fp.put(f1)
	require.Len(t, fp.fs, 1)
	require.Equal(t, 0, f1.Width())
	requireDeltaStats(t, map[string]interface{}{DeltaStatNameAllocatedFrames: uint64(2)}, dss)

	f3 := fp.get()
	require.Len(t, fp.fs, 0)
	require.Equal(t, f1, f3)
	requireDeltaStats(t, map[string]interface{}{DeltaStatNameAllocatedFrames: uint64(2)}, dss)

	f4 := astiav.AllocFrame()
	defer f4.Free()
	f4.SetHeight(8)
	f4.SetPixelFormat(astiav.PixelFormatRgba)
	f4.SetWidth(8)
	require.NoError(t, f4.AllocBuffer(1))
	f5, err := fp.copy(f4)
	require.NoError(t, err)
	require.Equal(t, 8, f5.Width())

	c.Close()
	require.Panics(t, func() { f1.Width() })

	require.Equal(t, framePoolCumulativeStats{
		allocatedFrames: 3,
	}, *fp.cs)
}
