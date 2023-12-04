package astiavflow

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/stretchr/testify/require"
)

func TestFrameDescriptorNew(t *testing.T) {
	md := MediaDescriptor{
		FrameRate: astiav.NewRational(6, 7),
		TimeBase:  astiav.NewRational(8, 9),
	}
	fd1 := FrameDescriptor{
		ChannelLayout:   astiav.ChannelLayout21,
		MediaDescriptor: md,
		MediaType:       astiav.MediaTypeAudio,
		SampleFormat:    astiav.SampleFormatFlt,
		SampleRate:      1,
	}
	fd2 := FrameDescriptor{
		ColorRange:        astiav.ColorRangeJpeg,
		ColorSpace:        astiav.ColorSpaceBt709,
		Height:            2,
		MediaDescriptor:   md,
		MediaType:         astiav.MediaTypeVideo,
		PixelFormat:       astiav.PixelFormat(astiav.PixelFormatRgba),
		SampleAspectRatio: astiav.NewRational(3, 4),
		Width:             5,
	}

	require.Equal(t, fd1, newFrameDescriptorFromFrameFiltererBuffersinkContexter(&mockedFrameFiltererBuffersinkContexter{
		cl: fd1.ChannelLayout,
		fr: md.FrameRate,
		mt: fd1.MediaType,
		sf: fd1.SampleFormat,
		sr: fd1.SampleRate,
		tb: md.TimeBase,
	}))
	require.Equal(t, fd2, newFrameDescriptorFromFrameFiltererBuffersinkContexter(&mockedFrameFiltererBuffersinkContexter{
		cr:  fd2.ColorRange,
		cs:  fd2.ColorSpace,
		fr:  md.FrameRate,
		h:   fd2.Height,
		mt:  fd2.MediaType,
		pf:  fd2.PixelFormat,
		sar: fd2.SampleAspectRatio,
		tb:  md.TimeBase,
		w:   fd2.Width,
	}))

	md.Rotation = 1
	fd1.MediaDescriptor.Rotation = 1
	fd2.MediaDescriptor.Rotation = 1

	f1 := astiav.AllocFrame()
	defer f1.Free()
	f1.SetChannelLayout(fd1.ChannelLayout)
	f1.SetSampleFormat(fd1.SampleFormat)
	f1.SetSampleRate(fd1.SampleRate)
	require.Equal(t, fd1, newFrameDescriptorFromFrame(f1, md, astiav.MediaTypeAudio))

	f2 := astiav.AllocFrame()
	defer f2.Free()
	f2.SetColorRange(fd2.ColorRange)
	f2.SetColorSpace(fd2.ColorSpace)
	f2.SetHeight(fd2.Height)
	f2.SetPixelFormat(fd2.PixelFormat)
	f2.SetSampleAspectRatio(fd2.SampleAspectRatio)
	f2.SetWidth(fd2.Width)
	require.Equal(t, fd2, newFrameDescriptorFromFrame(f2, md, astiav.MediaTypeVideo))

	cp1 := astiav.AllocCodecParameters()
	defer cp1.Free()
	cp1.SetChannelLayout(fd1.ChannelLayout)
	cp1.SetMediaType(fd1.MediaType)
	cp1.SetSampleFormat(fd1.SampleFormat)
	cp1.SetSampleRate(fd1.SampleRate)
	require.Equal(t, fd1, newFrameDescriptorFromPacketDescriptor(PacketDescriptor{
		CodecParameters: cp1,
		MediaDescriptor: md,
	}))

	cp2 := astiav.AllocCodecParameters()
	defer cp2.Free()
	cp2.SetColorRange(fd2.ColorRange)
	cp2.SetColorSpace(fd2.ColorSpace)
	cp2.SetHeight(fd2.Height)
	cp2.SetMediaType(fd2.MediaType)
	cp2.SetPixelFormat(fd2.PixelFormat)
	cp2.SetSampleAspectRatio(fd2.SampleAspectRatio)
	cp2.SetWidth(fd2.Width)
	require.Equal(t, fd2, newFrameDescriptorFromPacketDescriptor(PacketDescriptor{
		CodecParameters: cp2,
		MediaDescriptor: md,
	}))
}

func TestFrameDescriptorEqual(t *testing.T) {
	fd1 := FrameDescriptor{
		ChannelLayout:   astiav.ChannelLayout21,
		MediaDescriptor: MediaDescriptor{Rotation: 1},
		MediaType:       astiav.MediaTypeAudio,
		SampleFormat:    astiav.SampleFormatFlt,
		SampleRate:      1,
	}
	fd2 := fd1
	require.True(t, fd1.equal(fd2))
	fd2.MediaDescriptor = MediaDescriptor{}
	require.False(t, fd1.equal(fd2))
	fd2.MediaDescriptor = fd1.MediaDescriptor
	fd2.MediaType = astiav.MediaTypeSubtitle
	require.False(t, fd1.equal(fd2))
	fd2.MediaType = fd1.MediaType
	fd2.ChannelLayout = astiav.ChannelLayout22
	require.False(t, fd1.equal(fd2))
	fd2.ChannelLayout = fd1.ChannelLayout
	fd2.SampleFormat = astiav.SampleFormatDbl
	require.False(t, fd1.equal(fd2))
	fd2.SampleFormat = fd1.SampleFormat
	fd2.SampleRate = 0
	require.False(t, fd1.equal(fd2))
	fd2.SampleRate = fd1.SampleRate

	fd1 = FrameDescriptor{
		ColorRange:        astiav.ColorRangeJpeg,
		ColorSpace:        astiav.ColorSpaceBt709,
		Height:            2,
		MediaDescriptor:   MediaDescriptor{Rotation: 1},
		MediaType:         astiav.MediaTypeVideo,
		PixelFormat:       astiav.PixelFormat0Bgr,
		SampleAspectRatio: astiav.NewRational(3, 4),
		Width:             5,
	}
	fd2 = fd1
	require.True(t, fd1.equal(fd2))
	fd2.ColorRange = astiav.ColorRangeMpeg
	require.False(t, fd1.equal(fd2))
	fd2.ColorRange = fd1.ColorRange
	fd2.ColorSpace = astiav.ColorSpaceBt470Bg
	require.False(t, fd1.equal(fd2))
	fd2.ColorSpace = fd1.ColorSpace
	fd2.Height = 0
	require.False(t, fd1.equal(fd2))
	fd2.Height = fd1.Height
	fd2.PixelFormat = astiav.PixelFormat0Rgb
	require.False(t, fd1.equal(fd2))
	fd2.PixelFormat = fd1.PixelFormat
	fd2.SampleAspectRatio = astiav.NewRational(0, 1)
	require.False(t, fd1.equal(fd2))
	fd2.SampleAspectRatio = fd1.SampleAspectRatio
	fd2.Width = 0
	require.False(t, fd1.equal(fd2))
	fd2.Width = fd1.Width
	require.True(t, fd1.equal(fd2))
}

func TestFrameDescriptorPool(t *testing.T) {
	p1 := newFrameDescriptorPool()
	fd1 := FrameDescriptor{Height: 1}
	n1 := mocks.NewMockedNoder()
	fd2 := FrameDescriptor{Height: 2}
	n2 := mocks.NewMockedNoder()
	p1.set(fd1, n1)
	p1.set(fd2, n2)
	require.Equal(t, 2, p1.len())
	d, ok := p1.get(n1)
	require.True(t, ok)
	require.Equal(t, fd1, d)
	d, ok = p1.one()
	require.True(t, ok)
	require.Contains(t, []FrameDescriptor{fd1, fd2}, d)
	p1.del(n1)
	require.Equal(t, 1, p1.len())
	_, ok = p1.get(n1)
	require.False(t, ok)
	d, ok = p1.one()
	require.True(t, ok)
	require.Equal(t, fd2, d)
	p1.del(n2)
	require.Equal(t, 0, p1.len())
	_, ok = p1.one()
	require.False(t, ok)

	p2 := newFrameDescriptorPool()
	require.True(t, p1.equal(p2))
	p1.set(fd1, n1)
	require.False(t, p1.equal(p2))
	p2.set(fd2, n1)
	require.False(t, p1.equal(p2))
	p2.set(fd1, n1)
	require.True(t, p1.equal(p2))

	p1.set(fd1, n1)
	p1.set(fd2, n2)
	p2.set(fd2, n1)
	p2.set(fd1, n2)
	require.Equal(t, FrameDescriptorsDelta{
		"n1": {
			After:  fd2,
			Before: fd1,
		},
		"n2": {
			After:  fd1,
			Before: fd2,
		},
	}, p1.delta(p2, map[astiflow.Noder]string{
		n1: "n1",
		n2: "n2",
	}))
}

func TestFrameDescriptorDelta(t *testing.T) {
	fd1 := FrameDescriptor{
		ChannelLayout: astiav.ChannelLayout21,
		ColorRange:    astiav.ColorRangeJpeg,
		ColorSpace:    astiav.ColorSpaceBt709,
		Height:        1,
		MediaDescriptor: MediaDescriptor{
			FrameRate: astiav.NewRational(2, 3),
			Rotation:  4,
			TimeBase:  astiav.NewRational(5, 6),
		},
		MediaType:         astiav.MediaTypeAttachment,
		PixelFormat:       astiav.PixelFormatRgba,
		SampleAspectRatio: astiav.NewRational(7, 8),
		SampleFormat:      astiav.SampleFormatFlt,
		SampleRate:        9,
		Width:             10,
	}
	fd2 := FrameDescriptor{
		ChannelLayout: astiav.ChannelLayout22,
		ColorRange:    astiav.ColorRangeMpeg,
		ColorSpace:    astiav.ColorSpaceRgb,
		Height:        11,
		MediaDescriptor: MediaDescriptor{
			FrameRate: astiav.NewRational(12, 13),
			Rotation:  14,
			TimeBase:  astiav.NewRational(15, 16),
		},
		MediaType:         astiav.MediaTypeAudio,
		PixelFormat:       astiav.PixelFormatYuv420P,
		SampleAspectRatio: astiav.NewRational(17, 18),
		SampleFormat:      astiav.SampleFormatFltp,
		SampleRate:        19,
		Width:             20,
	}
	require.Equal(t, "", FrameDescriptorDelta{
		After:  fd1,
		Before: fd1,
	}.String())
	require.Equal(t, "frame rate changed: 2/3 --> 12/13 && rotation changed: 4.000 --> 14.000 && time base changed: 5/6 --> 15/16 && media type changed: attachment --> audio", FrameDescriptorDelta{
		After:  fd2,
		Before: fd1,
	}.String())
	fd1.MediaType = astiav.MediaTypeAudio
	require.Equal(t, "frame rate changed: 2/3 --> 12/13 && rotation changed: 4.000 --> 14.000 && time base changed: 5/6 --> 15/16 && channel layout changed: 3.0(back) --> quad(side) && sample format changed: flt --> fltp && sample rate changed: 9 --> 19", FrameDescriptorDelta{
		After:  fd2,
		Before: fd1,
	}.String())
	fd1.MediaType = astiav.MediaTypeVideo
	fd2.MediaType = astiav.MediaTypeVideo
	require.Equal(t, "frame rate changed: 2/3 --> 12/13 && rotation changed: 4.000 --> 14.000 && time base changed: 5/6 --> 15/16 && color range changed: pc --> tv && color space changed: bt709 --> gbr && height changed: 1 --> 11 && pixel format changed: rgba --> yuv420p && sample aspect ratio changed: 7/8 --> 17/18 && width changed: 10 --> 20", FrameDescriptorDelta{
		After:  fd2,
		Before: fd1,
	}.String())

	fd1 = FrameDescriptor{MediaType: astiav.MediaTypeAttachment}
	fd2 = FrameDescriptor{MediaType: astiav.MediaTypeAudio}
	require.Equal(t, "", FrameDescriptorsDelta{
		"n1": {
			After:  fd1,
			Before: fd1,
		},
		"n2": {
			After:  fd2,
			Before: fd2,
		},
	}.String())
	require.Contains(t, []string{
		"n1: media type changed: attachment --> audio | n2: media type changed: audio --> attachment",
		"n2: media type changed: audio --> attachment | n1: media type changed: attachment --> audio",
	}, FrameDescriptorsDelta{
		"n1": {
			After:  fd2,
			Before: fd1,
		},
		"n2": {
			After:  fd1,
			Before: fd2,
		},
	}.String())
}
