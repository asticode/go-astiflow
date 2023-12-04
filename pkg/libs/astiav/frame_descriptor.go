package astiavflow

import (
	"fmt"
	"strings"
	"sync"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
)

type FrameDescriptor struct {
	MediaDescriptor MediaDescriptor
	MediaType       astiav.MediaType

	// Audio
	ChannelLayout astiav.ChannelLayout
	SampleFormat  astiav.SampleFormat
	SampleRate    int

	// Video
	ColorRange        astiav.ColorRange
	ColorSpace        astiav.ColorSpace
	Height            int
	PixelFormat       astiav.PixelFormat
	SampleAspectRatio astiav.Rational
	Width             int
}

func newFrameDescriptorFromFrameFiltererBuffersinkContexter(l frameFiltererBuffersinkContexter) FrameDescriptor {
	fd := FrameDescriptor{
		MediaDescriptor: MediaDescriptor{
			FrameRate: l.FrameRate(),
			TimeBase:  l.TimeBase(),
		},
		MediaType: l.MediaType(),
	}
	switch fd.MediaType {
	case astiav.MediaTypeAudio:
		fd.ChannelLayout = l.ChannelLayout()
		fd.SampleFormat = l.SampleFormat()
		fd.SampleRate = l.SampleRate()
	case astiav.MediaTypeVideo:
		fd.ColorRange = l.ColorRange()
		fd.ColorSpace = l.ColorSpace()
		fd.Height = l.Height()
		fd.PixelFormat = l.PixelFormat()
		fd.SampleAspectRatio = l.SampleAspectRatio()
		fd.Width = l.Width()
	}
	return fd
}

func newFrameDescriptorFromFrame(f *astiav.Frame, md MediaDescriptor, mt astiav.MediaType) FrameDescriptor {
	fd := FrameDescriptor{
		MediaDescriptor: md,
		MediaType:       mt,
	}
	switch fd.MediaType {
	case astiav.MediaTypeAudio:
		fd.ChannelLayout = f.ChannelLayout()
		fd.SampleFormat = f.SampleFormat()
		fd.SampleRate = f.SampleRate()
	case astiav.MediaTypeVideo:
		fd.ColorRange = f.ColorRange()
		fd.ColorSpace = f.ColorSpace()
		fd.Height = f.Height()
		fd.PixelFormat = f.PixelFormat()
		fd.SampleAspectRatio = f.SampleAspectRatio()
		fd.Width = f.Width()
	}
	return fd
}

func newFrameDescriptorFromPacketDescriptor(pd PacketDescriptor) FrameDescriptor {
	fd := FrameDescriptor{
		MediaDescriptor: pd.MediaDescriptor,
		MediaType:       pd.CodecParameters.MediaType(),
	}
	switch fd.MediaType {
	case astiav.MediaTypeAudio:
		fd.ChannelLayout = pd.CodecParameters.ChannelLayout()
		fd.SampleFormat = pd.CodecParameters.SampleFormat()
		fd.SampleRate = pd.CodecParameters.SampleRate()
	case astiav.MediaTypeVideo:
		fd.ColorRange = pd.CodecParameters.ColorRange()
		fd.ColorSpace = pd.CodecParameters.ColorSpace()
		fd.Height = pd.CodecParameters.Height()
		fd.PixelFormat = pd.CodecParameters.PixelFormat()
		fd.SampleAspectRatio = pd.CodecParameters.SampleAspectRatio()
		fd.Width = pd.CodecParameters.Width()
	}
	return fd
}

func (p FrameDescriptor) equal(i FrameDescriptor) bool {
	if !p.MediaDescriptor.equal(i.MediaDescriptor) || p.MediaType != i.MediaType {
		return false
	}
	switch p.MediaType {
	case astiav.MediaTypeAudio:
		return p.ChannelLayout.Equal(i.ChannelLayout) &&
			p.SampleFormat == i.SampleFormat &&
			p.SampleRate == i.SampleRate
	case astiav.MediaTypeVideo:
		return p.ColorRange == i.ColorRange &&
			p.ColorSpace == i.ColorSpace &&
			p.Height == i.Height &&
			p.PixelFormat == i.PixelFormat &&
			p.SampleAspectRatio == i.SampleAspectRatio &&
			p.Width == i.Width
	default:
		return true
	}
}

type frameDescriptorPool struct {
	m sync.Mutex
	p map[astiflow.Noder]FrameDescriptor
}

func newFrameDescriptorPool() *frameDescriptorPool {
	return &frameDescriptorPool{p: make(map[astiflow.Noder]FrameDescriptor)}
}

func (p *frameDescriptorPool) set(d FrameDescriptor, n astiflow.Noder) {
	p.m.Lock()
	defer p.m.Unlock()
	p.p[n] = d
}

func (p *frameDescriptorPool) get(n astiflow.Noder) (FrameDescriptor, bool) {
	p.m.Lock()
	defer p.m.Unlock()
	d, ok := p.p[n]
	return d, ok
}

func (p *frameDescriptorPool) del(n astiflow.Noder) {
	p.m.Lock()
	defer p.m.Unlock()
	delete(p.p, n)
}

func (p *frameDescriptorPool) one() (FrameDescriptor, bool) {
	p.m.Lock()
	defer p.m.Unlock()
	for _, d := range p.p {
		return d, true
	}
	return FrameDescriptor{}, false
}

func (p *frameDescriptorPool) len() int {
	p.m.Lock()
	defer p.m.Unlock()
	return len(p.p)
}

func (a *frameDescriptorPool) equal(b *frameDescriptorPool) bool {
	a.m.Lock()
	defer a.m.Unlock()
	b.m.Lock()
	defer b.m.Unlock()
	for n, afd := range a.p {
		bfd, ok := b.p[n]
		if !ok {
			return false
		}
		if !afd.equal(bfd) {
			return false
		}
	}
	return true
}

func (before *frameDescriptorPool) delta(after *frameDescriptorPool, aliases map[astiflow.Noder]string) FrameDescriptorsDelta {
	before.m.Lock()
	defer before.m.Unlock()
	after.m.Lock()
	defer after.m.Unlock()
	fdds := make(FrameDescriptorsDelta)
	for n, bfd := range before.p {
		name, ok := aliases[n]
		if !ok {
			continue
		}
		afd, ok := after.p[n]
		if !ok {
			continue
		}
		fdds[name] = FrameDescriptorDelta{
			After:  afd,
			Before: bfd,
		}
	}
	return fdds
}

type FrameDescriptorsDelta map[string]FrameDescriptorDelta

func (fdds FrameDescriptorsDelta) String() string {
	var ss []string
	for n, fdd := range fdds {
		if s := fdd.String(); s != "" {
			ss = append(ss, fmt.Sprintf("%s: %s", n, s))
		}
	}
	return strings.Join(ss, " | ")
}

type FrameDescriptorDelta struct {
	After  FrameDescriptor
	Before FrameDescriptor
}

func (fdd FrameDescriptorDelta) String() string {
	var ss []string
	if fdd.Before.MediaDescriptor.FrameRate.Float64() != fdd.After.MediaDescriptor.FrameRate.Float64() {
		ss = append(ss, fmt.Sprintf("frame rate changed: %s --> %s", fdd.Before.MediaDescriptor.FrameRate, fdd.After.MediaDescriptor.FrameRate))
	}
	if fdd.Before.MediaDescriptor.Rotation != fdd.After.MediaDescriptor.Rotation {
		ss = append(ss, fmt.Sprintf("rotation changed: %.3f --> %.3f", fdd.Before.MediaDescriptor.Rotation, fdd.After.MediaDescriptor.Rotation))
	}
	if fdd.Before.MediaDescriptor.TimeBase.Float64() != fdd.After.MediaDescriptor.TimeBase.Float64() {
		ss = append(ss, fmt.Sprintf("time base changed: %s --> %s", fdd.Before.MediaDescriptor.TimeBase, fdd.After.MediaDescriptor.TimeBase))
	}
	if fdd.Before.MediaType != fdd.After.MediaType {
		ss = append(ss, fmt.Sprintf("media type changed: %s --> %s", fdd.Before.MediaType, fdd.After.MediaType))
	} else {
		switch fdd.Before.MediaType {
		case astiav.MediaTypeAudio:
			if !fdd.Before.ChannelLayout.Equal(fdd.After.ChannelLayout) {
				ss = append(ss, fmt.Sprintf("channel layout changed: %s --> %s", fdd.Before.ChannelLayout, fdd.After.ChannelLayout))
			}
			if fdd.Before.SampleFormat != fdd.After.SampleFormat {
				ss = append(ss, fmt.Sprintf("sample format changed: %s --> %s", fdd.Before.SampleFormat, fdd.After.SampleFormat))
			}
			if fdd.Before.SampleRate != fdd.After.SampleRate {
				ss = append(ss, fmt.Sprintf("sample rate changed: %d --> %d", fdd.Before.SampleRate, fdd.After.SampleRate))
			}
		case astiav.MediaTypeVideo:
			if fdd.Before.ColorRange != fdd.After.ColorRange {
				ss = append(ss, fmt.Sprintf("color range changed: %s --> %s", fdd.Before.ColorRange, fdd.After.ColorRange))
			}
			if fdd.Before.ColorSpace != fdd.After.ColorSpace {
				ss = append(ss, fmt.Sprintf("color space changed: %s --> %s", fdd.Before.ColorSpace, fdd.After.ColorSpace))
			}
			if fdd.Before.Height != fdd.After.Height {
				ss = append(ss, fmt.Sprintf("height changed: %d --> %d", fdd.Before.Height, fdd.After.Height))
			}
			if fdd.Before.PixelFormat != fdd.After.PixelFormat {
				ss = append(ss, fmt.Sprintf("pixel format changed: %s --> %s", fdd.Before.PixelFormat, fdd.After.PixelFormat))
			}
			if fdd.Before.SampleAspectRatio != fdd.After.SampleAspectRatio {
				ss = append(ss, fmt.Sprintf("sample aspect ratio changed: %s --> %s", fdd.Before.SampleAspectRatio, fdd.After.SampleAspectRatio))
			}
			if fdd.Before.Width != fdd.After.Width {
				ss = append(ss, fmt.Sprintf("width changed: %d --> %d", fdd.Before.Width, fdd.After.Width))
			}
		}
	}
	return strings.Join(ss, " && ")
}

type FrameDescriptorAdapter func(fd *FrameDescriptor)
