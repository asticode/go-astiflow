package astiavflow

import (
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestFramePool(t *testing.T) {
	c := astikit.NewCloser()
	fp := newFramePool().init(c)

	dss := fp.deltaStats()
	require.Len(t, dss, 1)
	require.Equal(t, uint64(0), dss[0].Valuer.Value(time.Second))

	f1 := fp.get()
	require.Len(t, fp.fs, 0)
	require.NotNil(t, f1)
	f1.SetWidth(1)
	require.Equal(t, uint64(1), dss[0].Valuer.Value(time.Second))

	f2 := fp.get()
	require.Len(t, fp.fs, 0)
	require.NotNil(t, f2)
	require.NotEqual(t, f1, f2)
	require.Equal(t, uint64(2), dss[0].Valuer.Value(time.Second))

	fp.put(f1)
	require.Len(t, fp.fs, 1)
	require.Equal(t, 0, f1.Width())
	require.Equal(t, uint64(2), dss[0].Valuer.Value(time.Second))

	f3 := fp.get()
	require.Len(t, fp.fs, 0)
	require.Equal(t, f1, f3)
	require.Equal(t, uint64(2), dss[0].Valuer.Value(time.Second))

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
	}, *fp.cumulativeStats)
}
