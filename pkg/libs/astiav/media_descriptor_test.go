package astiavflow

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/stretchr/testify/require"
)

func TestMediaDescriptor(t *testing.T) {
	fc := astiav.AllocFormatContext()
	defer fc.Free()

	s1 := fc.NewStream(nil)
	s1.SetAvgFrameRate(astiav.NewRational(2, 1))
	s1.SetRFrameRate(astiav.NewRational(3, 1))
	require.NoError(t, s1.CodecParameters().SideData().Add(astiav.PacketSideDataTypeDisplaymatrix, astiav.NewDisplayMatrixFromRotation(90).Bytes()))
	s1.SetTimeBase(astiav.NewRational(6, 1))
	require.Equal(t, MediaDescriptor{
		FrameRate: astiav.NewRational(2, 1),
		Rotation:  90,
		TimeBase:  astiav.NewRational(6, 1),
	}, newMediaDescriptorFromStream(s1))

	s1.SetAvgFrameRate(astiav.NewRational(0, 1))
	require.Equal(t, astiav.NewRational(3, 1), newMediaDescriptorFromStream(s1).FrameRate)
	s1.CodecParameters().SetMediaType(astiav.MediaTypeAudio)
	s1.CodecParameters().SetFrameSize(1)
	s1.CodecParameters().SetSampleRate(4)
	require.Equal(t, astiav.NewRational(3, 1), newMediaDescriptorFromStream(s1).FrameRate)
	s1.SetRFrameRate(astiav.NewRational(0, 1))
	require.Equal(t, astiav.NewRational(4, 1), newMediaDescriptorFromStream(s1).FrameRate)

	md1 := MediaDescriptor{
		FrameRate: astiav.NewRational(1, 2),
		Rotation:  1,
		TimeBase:  astiav.NewRational(3, 4),
	}
	md2 := md1
	require.True(t, md1.equal(md2))
	md2.FrameRate = astiav.NewRational(3, 4)
	require.False(t, md1.equal(md2))
	md2.FrameRate = md1.FrameRate
	md2.Rotation = 2
	require.False(t, md1.equal(md2))
	md2.Rotation = md1.Rotation
	md2.TimeBase = astiav.NewRational(1, 2)
	require.False(t, md1.equal(md2))
	md2.TimeBase = md1.TimeBase
	require.True(t, md1.equal(md2))
}
