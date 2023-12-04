package astiavflow

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/stretchr/testify/require"
)

func TestMediaContext(t *testing.T) {
	fc := astiav.AllocFormatContext()
	defer fc.Free()

	s1 := fc.NewStream(nil)
	s1.SetAvgFrameRate(astiav.NewRational(2, 1))
	s1.SetRFrameRate(astiav.NewRational(3, 1))
	require.NoError(t, s1.AddSideData(astiav.PacketSideDataTypeDisplaymatrix, astiav.NewDisplayMatrixFromRotation(90).Bytes()))
	s1.SetTimeBase(astiav.NewRational(6, 1))
	require.Equal(t, MediaContext{
		FrameRate: astiav.NewRational(2, 1),
		Rotation:  90,
		TimeBase:  astiav.NewRational(6, 1),
	}, newMediaContextFromStream(s1))

	s1.SetAvgFrameRate(astiav.NewRational(0, 1))
	require.Equal(t, astiav.NewRational(3, 1), newMediaContextFromStream(s1).FrameRate)
}
