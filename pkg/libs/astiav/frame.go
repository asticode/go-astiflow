package astiavflow

import (
	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
)

type Frame struct {
	*astiav.Frame
	MediaContext MediaContext
	Noder        astiflow.Noder
}

type FrameHandler interface {
	HandleFrame(p Frame)
	NodeConnector() *astiflow.NodeConnector
}

// TODO Rename ?
// TODO Test
type frameProperties struct {
	// Audio
	channelLayout astiav.ChannelLayout
	sampleFormat  astiav.SampleFormat
	sampleRate    int

	// Video
	height            int
	pixelFormat       astiav.PixelFormat
	sampleAspectRatio astiav.Rational
	width             int
}

func newFrameProperties(f *astiav.Frame) *frameProperties {
	return &frameProperties{
		// Audio
		channelLayout: f.ChannelLayout(),
		sampleFormat:  f.SampleFormat(),
		sampleRate:    f.SampleRate(),

		// Video
		height:            f.Height(),
		pixelFormat:       f.PixelFormat(),
		sampleAspectRatio: f.SampleAspectRatio(),
		width:             f.Width(),
	}
}

func (p *frameProperties) equal(i *frameProperties) bool {
	return p.channelLayout.Equal(i.channelLayout) &&
		p.sampleFormat == i.sampleFormat &&
		p.sampleRate == i.sampleRate &&

		p.height == i.height &&
		p.pixelFormat == i.pixelFormat &&
		p.sampleAspectRatio == i.sampleAspectRatio &&
		p.width == i.width
}
