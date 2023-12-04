package astiavflow

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

type mockedFrameFiltererGraphers struct {
	gs        []*mockedFrameFiltererGrapher
	onGrapher func(g *mockedFrameFiltererGrapher)
	previous  func() frameFiltererGrapher
}

func newMockedFrameFiltererGraphers() *mockedFrameFiltererGraphers {
	gs := &mockedFrameFiltererGraphers{previous: newFrameFiltererGrapher}
	newFrameFiltererGrapher = func() frameFiltererGrapher {
		g := &mockedFrameFiltererGrapher{}
		if gs.onGrapher != nil {
			gs.onGrapher(g)
		}
		gs.gs = append(gs.gs, g)
		return g
	}
	return gs
}

func (gs *mockedFrameFiltererGraphers) flush() {
	gs.gs = []*mockedFrameFiltererGrapher{}
}

func (gs *mockedFrameFiltererGraphers) close() {
	newFrameFiltererGrapher = gs.previous
}

var _ frameFiltererGrapher = (*mockedFrameFiltererGrapher)(nil)

type mockedFrameFiltererGrapher struct {
	freed                        bool
	onConfigure                  func() error
	onNewBuffersinkFilterContext func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersinkContexter, error)
	onNewBuffersrcFilterContext  func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersrcContexter, error)
	onParseSegment               func(content string) (frameFiltererSegmenter, error)
	parsedContent                string
	parsedInputs                 frameFiltererInOuter
	parsedOutputs                frameFiltererInOuter
	threadCount                  int
	threadType                   astiav.ThreadType
}

func (g *mockedFrameFiltererGrapher) Class() *astiav.Class {
	return nil
}

func (g *mockedFrameFiltererGrapher) Configure() error {
	if g.onConfigure != nil {
		return g.onConfigure()
	}
	return nil
}

func (g *mockedFrameFiltererGrapher) Free() {
	g.freed = true
}

func (g *mockedFrameFiltererGrapher) NewBuffersinkFilterContext(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersinkContexter, error) {
	if g.onNewBuffersinkFilterContext != nil {
		return g.onNewBuffersinkFilterContext(f, name, args)
	}
	return nil, nil
}

func (g *mockedFrameFiltererGrapher) NewBuffersrcFilterContext(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersrcContexter, error) {
	if g.onNewBuffersrcFilterContext != nil {
		return g.onNewBuffersrcFilterContext(f, name, args)
	}
	return nil, nil
}

func (g *mockedFrameFiltererGrapher) Parse(content string, inputs, outputs frameFiltererInOuter) error {
	g.parsedContent = content
	g.parsedInputs = inputs
	g.parsedOutputs = outputs
	return nil
}

func (g *mockedFrameFiltererGrapher) ParseSegment(content string) (frameFiltererSegmenter, error) {
	if g.onParseSegment != nil {
		return g.onParseSegment(content)
	}
	return nil, nil
}

func (g *mockedFrameFiltererGrapher) SetThreadCount(i int) {
	g.threadCount = i
}

func (g *mockedFrameFiltererGrapher) SetThreadType(tt astiav.ThreadType) {
	g.threadType = tt
}

var _ frameFiltererBuffersinkContexter = (*mockedFrameFiltererBuffersinkContexter)(nil)

type mockedFrameFiltererBuffersinkContexter struct {
	cl         astiav.ChannelLayout
	cr         astiav.ColorRange
	cs         astiav.ColorSpace
	fr         astiav.Rational
	h          int
	mt         astiav.MediaType
	onGetFrame func(f *astiav.Frame, fs astiav.BuffersinkFlags) error
	p          *astiav.FilterContext
	pf         astiav.PixelFormat
	sar        astiav.Rational
	sf         astiav.SampleFormat
	sr         int
	tb         astiav.Rational
	w          int
}

func (c *mockedFrameFiltererBuffersinkContexter) ChannelLayout() astiav.ChannelLayout {
	return c.cl
}

func (c *mockedFrameFiltererBuffersinkContexter) ColorRange() astiav.ColorRange {
	return c.cr
}

func (c *mockedFrameFiltererBuffersinkContexter) ColorSpace() astiav.ColorSpace {
	return c.cs
}

func (c *mockedFrameFiltererBuffersinkContexter) FrameRate() astiav.Rational {
	return c.fr
}

func (c *mockedFrameFiltererBuffersinkContexter) GetFrame(f *astiav.Frame, fs astiav.BuffersinkFlags) error {
	if c.onGetFrame != nil {
		return c.onGetFrame(f, fs)
	}
	return nil
}

func (c *mockedFrameFiltererBuffersinkContexter) Height() int {
	return c.h
}

func (c *mockedFrameFiltererBuffersinkContexter) MediaType() astiav.MediaType {
	return c.mt
}

func (c *mockedFrameFiltererBuffersinkContexter) PixelFormat() astiav.PixelFormat {
	return c.pf
}

func (c *mockedFrameFiltererBuffersinkContexter) ptr() *astiav.FilterContext {
	return c.p
}

func (c *mockedFrameFiltererBuffersinkContexter) SampleAspectRatio() astiav.Rational {
	return c.sar
}

func (c *mockedFrameFiltererBuffersinkContexter) SampleFormat() astiav.SampleFormat {
	return c.sf
}

func (c *mockedFrameFiltererBuffersinkContexter) SampleRate() int {
	return c.sr
}

func (c *mockedFrameFiltererBuffersinkContexter) TimeBase() astiav.Rational {
	return c.tb
}

func (c *mockedFrameFiltererBuffersinkContexter) Width() int {
	return c.w
}

var _ frameFiltererBuffersrcContexter = (*mockedFrameFiltererBuffersrcContexter)(nil)

type mockedFrameFiltererBuffersrcContexter struct {
	onAddFrame func(f *astiav.Frame, fs astiav.BuffersrcFlags) error
	p          *astiav.FilterContext
}

func (c *mockedFrameFiltererBuffersrcContexter) AddFrame(f *astiav.Frame, fs astiav.BuffersrcFlags) error {
	if c.onAddFrame != nil {
		return c.onAddFrame(f, fs)
	}
	return nil
}

func (c *mockedFrameFiltererBuffersrcContexter) ptr() *astiav.FilterContext {
	return c.p
}

type mockedFrameFiltererInOuters struct {
	ios       []*mockedFrameFiltererInOuter
	onInOuter func(io *mockedFrameFiltererInOuter)
	previous  func() frameFiltererInOuter
}

func newMockedFrameFiltererInOuters() *mockedFrameFiltererInOuters {
	ios := &mockedFrameFiltererInOuters{previous: newFrameFiltererInOuter}
	newFrameFiltererInOuter = func() frameFiltererInOuter {
		io := &mockedFrameFiltererInOuter{}
		if ios.onInOuter != nil {
			ios.onInOuter(io)
		}
		ios.ios = append(ios.ios, io)
		return io
	}
	return ios
}

func (ios *mockedFrameFiltererInOuters) flush() {
	ios.ios = []*mockedFrameFiltererInOuter{}
}

func (ios *mockedFrameFiltererInOuters) close() {
	newFrameFiltererInOuter = ios.previous
}

var _ frameFiltererInOuter = (*mockedFrameFiltererInOuter)(nil)

type mockedFrameFiltererInOuter struct {
	ffc    frameFiltererContexter
	freed  bool
	name   string
	next   frameFiltererInOuter
	padIdx int
}

func (io *mockedFrameFiltererInOuter) Free() {
	io.freed = true
}

func (io *mockedFrameFiltererInOuter) SetFilterContext(i frameFiltererContexter) {
	io.ffc = i
}

func (io *mockedFrameFiltererInOuter) SetName(i string) {
	io.name = i
}

func (io *mockedFrameFiltererInOuter) SetNext(i frameFiltererInOuter) {
	io.next = i
}

func (io *mockedFrameFiltererInOuter) SetPadIdx(i int) {
	io.padIdx = i
}

func (io *mockedFrameFiltererInOuter) ptr() *astiav.FilterInOut {
	return &astiav.FilterInOut{}
}

var _ frameFiltererSegmenter = (*mockedFrameFiltererSegmenter)(nil)

type mockedFrameFiltererSegmenter struct {
	cs    []*mockedFrameFiltererChainer
	freed bool
}

func (s *mockedFrameFiltererSegmenter) Chains() (cs []frameFiltererChainer) {
	for _, c := range s.cs {
		cs = append(cs, c)
	}
	return
}

func (s *mockedFrameFiltererSegmenter) Free() {
	s.freed = true
}

var _ frameFiltererChainer = (*mockedFrameFiltererChainer)(nil)

type mockedFrameFiltererChainer struct {
	ps []*mockedFrameFiltererParamers
}

func (c *mockedFrameFiltererChainer) Filters() (ps []frameFiltererParamers) {
	for _, p := range c.ps {
		ps = append(ps, p)
	}
	return
}

var _ frameFiltererParamers = (*mockedFrameFiltererParamers)(nil)

type mockedFrameFiltererParamers struct {
	fn string
}

func (fps *mockedFrameFiltererParamers) FilterName() string {
	return fps.fn
}

type mockedFrameFiltererFilterers struct {
	fs         []*mockedFrameFiltererFilterer
	onFilterer func(f *mockedFrameFiltererFilterer) *mockedFrameFiltererFilterer
	previous   func(name string) frameFiltererFilterer
}

func newMockedFrameFiltererFilterers() *mockedFrameFiltererFilterers {
	fs := &mockedFrameFiltererFilterers{previous: newFrameFiltererFilterer}
	newFrameFiltererFilterer = func(name string) frameFiltererFilterer {
		f := &mockedFrameFiltererFilterer{name: name}
		if fs.onFilterer != nil {
			f = fs.onFilterer(f)
		}
		if f == nil {
			return nil
		}
		fs.fs = append(fs.fs, f)
		return f
	}
	return fs
}

func (fs *mockedFrameFiltererFilterers) flush() {
	fs.fs = []*mockedFrameFiltererFilterer{}
}

func (fs *mockedFrameFiltererFilterers) close() {
	newFrameFiltererFilterer = fs.previous
}

var _ frameFiltererFilterer = (*mockedFrameFiltererFilterer)(nil)

type mockedFrameFiltererFilterer struct {
	name string
	ps   []*mockedFrameFiltererPader
}

func (f *mockedFrameFiltererFilterer) Outputs() (ps []frameFiltererPader) {
	for _, p := range f.ps {
		ps = append(ps, p)
	}
	return
}

func (f *mockedFrameFiltererFilterer) ptr() *astiav.Filter {
	return nil
}

var _ frameFiltererPader = (*mockedFrameFiltererPader)(nil)

type mockedFrameFiltererPader struct {
	mt astiav.MediaType
}

func (p *mockedFrameFiltererPader) MediaType() astiav.MediaType {
	return p.mt
}

func TestNewFrameFilterer(t *testing.T) {
	// Default metadata should be set properly and event emitter should work properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		countFrameFilterer = 0
		ff, err := NewFrameFilterer(FrameFiltererOptions{
			Filterer: FrameFiltererFiltererOptions{Aliases: map[astiflow.Noder]string{nil: ""}},
			Group:    g,
		})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{Name: "frame_filterer_1", Tags: []string{"frame_filterer"}}, ff.n.Metadata())
		var emitted bool
		ff.On(astiflow.EventNameNodeClosed, func(payload interface{}) (delete bool) {
			emitted = true
			return
		})
		g.Close()
		require.True(t, emitted)
	})

	// Metadata should be overwritten properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		ff, err := NewFrameFilterer(FrameFiltererOptions{
			Filterer: FrameFiltererFiltererOptions{Aliases: map[astiflow.Noder]string{nil: ""}},
			Group:    g,
			Metadata: astiflow.Metadata{Description: "d", Name: "n", Tags: []string{"t"}},
		})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{
			Description: "d",
			Name:        "n",
			Tags:        []string{"frame_filterer", "t"},
		}, ff.n.Metadata())
	})

	// No aliases should be processed properly (mock)
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		fs := newMockedFrameFiltererFilterers()
		defer fs.close()
		filterName := "filter name"
		mediaType := astiav.MediaTypeAudio
		fs.onFilterer = func(f *mockedFrameFiltererFilterer) *mockedFrameFiltererFilterer {
			switch f.name {
			case "abuffersink":
			case filterName:
				f.ps = append(f.ps, &mockedFrameFiltererPader{mt: mediaType})
			default:
				return nil
			}
			return f
		}

		ios := newMockedFrameFiltererInOuters()
		defer ios.close()

		gs := newMockedFrameFiltererGraphers()
		defer gs.close()
		countGrapher := 0
		content := "content"
		s := &mockedFrameFiltererSegmenter{cs: []*mockedFrameFiltererChainer{{ps: []*mockedFrameFiltererParamers{{fn: filterName}}}}}
		gs.onGrapher = func(g *mockedFrameFiltererGrapher) {
			countGrapher++
			switch countGrapher {
			case 1:
				g.onParseSegment = func(c string) (frameFiltererSegmenter, error) {
					require.Equal(t, content, c)
					return s, nil
				}
			case 2:
				g.onNewBuffersinkFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersinkContexter, error) {
					return &mockedFrameFiltererBuffersinkContexter{mt: mediaType}, nil
				}
				g.onNewBuffersrcFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersrcContexter, error) {
					return &mockedFrameFiltererBuffersrcContexter{}, nil
				}
			}
		}

		emulateRate := astiav.NewRational(1, 1)
		ff, err := NewFrameFilterer(FrameFiltererOptions{
			Filterer: FrameFiltererFiltererOptions{
				Content:     content,
				EmulateRate: emulateRate,
			},
			Group: g,
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(gs.gs))
		require.Equal(t, mediaType, ff.f.outgoingFrameDescriptor.MediaType)
		require.True(t, gs.gs[0].freed)
		require.True(t, s.freed)
		require.NotNil(t, ff.f)
		require.Equal(t, emulateRate, ff.f.outgoingFrameDescriptor.MediaDescriptor.FrameRate)
	})

	// No aliases should be processed properly (no mock)
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		for _, v := range []struct {
			content     string
			emulateRate astiav.Rational
			mediaType   astiav.MediaType
			withError   bool
		}{
			{
				content:   "anullsrc",
				mediaType: astiav.MediaTypeAudio,
				withError: true,
			},
			{
				content:     "anullsrc",
				emulateRate: astiav.NewRational(1, 1),
				mediaType:   astiav.MediaTypeAudio,
			},
			{
				content:     "color",
				emulateRate: astiav.NewRational(1, 1),
				mediaType:   astiav.MediaTypeVideo,
			},
			{
				content:     "invalid",
				emulateRate: astiav.NewRational(1, 1),
				withError:   true,
			},
		} {
			ff, err := NewFrameFilterer(FrameFiltererOptions{
				Filterer: FrameFiltererFiltererOptions{
					Content:     v.content,
					EmulateRate: v.emulateRate,
				},
				Group: g,
			})
			if v.withError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, ff.f)
			require.Equal(t, v.mediaType, ff.f.outgoingFrameDescriptor.MediaType)
		}
	})
}

func TestFrameFiltererOnConnect(t *testing.T) {
	// Audio and video filter should be created properly on parent connect, filter should be closed properly
	// when node closes and when creation fails, outgoing frame descriptor adapter should work properly,
	// connecting children to frame filterer should work properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		nr1 := mocks.NewMockedNoder()
		nr2 := mocks.NewMockedNoder()
		fd1 := FrameDescriptor{
			ColorRange: astiav.ColorRangeMpeg,
			ColorSpace: astiav.ColorSpaceBt470Bg,
			Height:     13,
			MediaDescriptor: MediaDescriptor{
				FrameRate: astiav.NewRational(1, 1),
				TimeBase:  astiav.NewRational(1, 2),
			},
			MediaType:         astiav.MediaTypeVideo,
			PixelFormat:       astiav.PixelFormatYuv410P,
			SampleAspectRatio: astiav.NewRational(1, 4),
			Width:             15,
		}
		fd2 := FrameDescriptor{
			ColorRange: astiav.ColorRangeNb,
			ColorSpace: astiav.ColorSpaceBt2020Cl,
			Height:     23,
			MediaDescriptor: MediaDescriptor{
				FrameRate: astiav.NewRational(2, 1),
				TimeBase:  astiav.NewRational(2, 2),
			},
			MediaType:         astiav.MediaTypeVideo,
			PixelFormat:       astiav.PixelFormatYuv440P,
			SampleAspectRatio: astiav.NewRational(2, 4),
			Width:             25,
		}
		nr3 := mocks.NewMockedNoder()

		fs := newMockedFrameFiltererFilterers()
		defer fs.close()
		gs := newMockedFrameFiltererGraphers()
		defer gs.close()
		ios := newMockedFrameFiltererInOuters()
		defer ios.close()

		alias1 := "alias1"
		alias2 := "alias2"
		content := "content"
		threadCount := 1
		threadType := astiav.ThreadTypeSlice
		ff1, err := NewFrameFilterer(FrameFiltererOptions{
			Filterer: FrameFiltererFiltererOptions{
				Aliases: map[astiflow.Noder]string{
					nr1: alias1,
					nr2: alias2,
				},
				Content:     content,
				ThreadCount: threadCount,
				ThreadType:  threadType,
			},
			Group: g,
		})
		require.NoError(t, err)

		// Noder not in aliases
		require.Error(t, ff1.OnConnect(FrameDescriptor{}, nr3))

		// Not all aliases have connected
		require.NoError(t, ff1.OnConnect(fd1, nr1))
		require.Nil(t, ff1.f)

		// Everything is closed properly when creating filter failed
		gs.onGrapher = func(g *mockedFrameFiltererGrapher) {
			g.onConfigure = func() error { return errors.New("error") }
			g.onNewBuffersinkFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersinkContexter, error) {
				return &mockedFrameFiltererBuffersinkContexter{p: &astiav.FilterContext{}}, nil
			}
			g.onNewBuffersrcFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersrcContexter, error) {
				return &mockedFrameFiltererBuffersrcContexter{p: &astiav.FilterContext{}}, nil
			}
		}
		classersCount := len(classers.p)
		require.Error(t, ff1.OnConnect(fd2, nr2))
		require.Equal(t, 1, len(gs.gs))
		require.True(t, gs.gs[0].freed)
		require.Equal(t, classersCount, len(classers.p))
		require.Equal(t, 3, len(ios.ios))
		require.True(t, ios.ios[0].freed)
		require.True(t, ios.ios[2].freed)
		require.Nil(t, ff1.f)

		// Video filter is created properly on connect
		fs.flush()
		gs.flush()
		ios.flush()
		fs.onFilterer = func(f *mockedFrameFiltererFilterer) *mockedFrameFiltererFilterer {
			switch f.name {
			case "buffer", "buffersink":
				return f
			}
			return nil
		}
		buffersinkContext := &mockedFrameFiltererBuffersinkContexter{
			cr:  astiav.ColorRangeJpeg,
			cs:  astiav.ColorSpaceBt709,
			fr:  astiav.NewRational(3, 2),
			h:   33,
			mt:  astiav.MediaTypeVideo,
			p:   &astiav.FilterContext{},
			pf:  astiav.PixelFormatYuv420P,
			sar: astiav.NewRational(3, 4),
			tb:  astiav.NewRational(3, 5),
			w:   36,
		}
		buffersrcContext1 := &mockedFrameFiltererBuffersrcContexter{p: &astiav.FilterContext{}}
		buffersrcContext2 := &mockedFrameFiltererBuffersrcContexter{p: &astiav.FilterContext{}}
		filterContextArgs := make(map[*astiav.FilterContext]astiav.FilterArgs)
		var configured bool
		gs.onGrapher = func(g *mockedFrameFiltererGrapher) {
			count := 0
			g.onConfigure = func() error {
				configured = true
				return nil
			}
			g.onNewBuffersinkFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersinkContexter, error) {
				v, ok := f.(*mockedFrameFiltererFilterer)
				if !ok {
					return nil, errors.New("f not a *mockedFrameFiltererFilterer")
				}
				if v.name != "buffersink" {
					return nil, fmt.Errorf("invalid name %s", v.name)
				}
				filterContextArgs[buffersinkContext.p] = args
				return buffersinkContext, nil
			}
			g.onNewBuffersrcFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersrcContexter, error) {
				v, ok := f.(*mockedFrameFiltererFilterer)
				if !ok {
					return nil, errors.New("f not a *mockedFrameFiltererFilterer")
				}
				if v.name != "buffer" {
					return nil, fmt.Errorf("invalid name %s", v.name)
				}
				ctx := buffersrcContext1
				if count > 0 {
					ctx = buffersrcContext2
				}
				filterContextArgs[ctx.p] = args
				count++
				return ctx, nil
			}
		}
		require.NoError(t, ff1.OnConnect(fd2, nr2))
		require.Equal(t, 1, len(gs.gs))
		require.Equal(t, 3, len(ios.ios))
		require.True(t, configured)
		require.False(t, gs.gs[0].freed)
		require.Equal(t, content, gs.gs[0].parsedContent)
		require.Equal(t, ios.ios[0], gs.gs[0].parsedInputs)
		require.Equal(t, ios.ios[2], gs.gs[0].parsedOutputs)
		require.Equal(t, threadCount, gs.gs[0].threadCount)
		require.Equal(t, threadType, gs.gs[0].threadType)
		require.Nil(t, filterContextArgs[buffersinkContext.p])
		contains := []astiav.FilterArgs{
			{"colorspace": "bt470bg", "frame_rate": "1/1", "height": "13", "pix_fmt": "6", "range": "tv", "sar": "1/4", "time_base": "1/2", "width": "15"},
			{"colorspace": "bt2020c", "frame_rate": "2/1", "height": "23", "pix_fmt": "31", "range": "", "sar": "2/4", "time_base": "2/2", "width": "25"},
		}
		require.Contains(t, contains, filterContextArgs[buffersrcContext1.p])
		buffersrcContexts := map[astiflow.Noder]frameFiltererBuffersrcContexter{
			nr1: buffersrcContext1,
			nr2: buffersrcContext2,
		}
		iosName1 := alias1
		iosName2 := alias2
		if filterContextArgs[buffersrcContext1.p]["colorspace"] == contains[0]["colorspace"] {
			contains = []astiav.FilterArgs{contains[1]}
		} else {
			buffersrcContexts = map[astiflow.Noder]frameFiltererBuffersrcContexter{
				nr1: buffersrcContext2,
				nr2: buffersrcContext1,
			}
			contains = []astiav.FilterArgs{contains[0]}
			iosName1 = alias2
			iosName2 = alias1
		}
		require.Contains(t, contains, filterContextArgs[buffersrcContext2.p])
		v, ok := classers.get(gs.gs[0])
		require.True(t, ok)
		require.Equal(t, ff1.n, v)
		v, ok = classers.get(buffersinkContext.p)
		require.True(t, ok)
		require.Equal(t, ff1.n, v)
		v, ok = classers.get(buffersrcContext1.p)
		require.True(t, ok)
		require.Equal(t, ff1.n, v)
		v, ok = classers.get(buffersrcContext2.p)
		require.True(t, ok)
		require.Equal(t, ff1.n, v)
		require.Equal(t, &mockedFrameFiltererInOuter{
			ffc:   buffersinkContext,
			freed: true,
			name:  "out",
		}, ios.ios[0])
		require.Equal(t, &mockedFrameFiltererInOuter{
			ffc:  buffersrcContext1,
			name: iosName1,
		}, ios.ios[1])
		require.Equal(t, &mockedFrameFiltererInOuter{
			ffc:   buffersrcContext2,
			freed: true,
			name:  iosName2,
			next:  ios.ios[1],
		}, ios.ios[2])
		require.NotNil(t, ff1.f)
		require.Equal(t, buffersinkContext, ff1.f.buffersinkContext)
		require.Equal(t, buffersrcContexts, ff1.f.buffersrcContexts)
		require.Equal(t, gs.gs[0], ff1.f.g)
		fd, ok := ff1.f.incomingFrameDescriptors.get(nr1)
		require.True(t, ok)
		require.Equal(t, fd1, fd)
		fd, ok = ff1.f.incomingFrameDescriptors.get(nr2)
		require.True(t, ok)
		require.Equal(t, fd2, fd)
		require.Equal(t, FrameDescriptor{
			ColorRange: buffersinkContext.cr,
			ColorSpace: buffersinkContext.cs,
			Height:     buffersinkContext.h,
			MediaDescriptor: MediaDescriptor{
				FrameRate: buffersinkContext.fr,
				TimeBase:  buffersinkContext.tb,
			},
			MediaType:         buffersinkContext.mt,
			PixelFormat:       buffersinkContext.pf,
			SampleAspectRatio: buffersinkContext.sar,
			Width:             buffersinkContext.w,
		}, ff1.f.outgoingFrameDescriptor)

		// Audio filter is created properly on connect and outgoing frame descriptor
		// adapter works properly
		fs.flush()
		gs.flush()
		ios.flush()
		fs.onFilterer = func(f *mockedFrameFiltererFilterer) *mockedFrameFiltererFilterer {
			switch f.name {
			case "abuffer", "abuffersink":
				return f
			}
			return nil
		}
		buffersinkContext = &mockedFrameFiltererBuffersinkContexter{
			cl: astiav.ChannelLayout21,
			fr: astiav.NewRational(3, 2),
			mt: astiav.MediaTypeAudio,
			p:  &astiav.FilterContext{},
			sf: astiav.SampleFormatDbl,
			sr: 33,
			tb: astiav.NewRational(3, 5),
		}
		filterContextArgs = make(map[*astiav.FilterContext]astiav.FilterArgs)
		gs.onGrapher = func(g *mockedFrameFiltererGrapher) {
			count := 0
			g.onNewBuffersinkFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersinkContexter, error) {
				v, ok := f.(*mockedFrameFiltererFilterer)
				if !ok {
					return nil, errors.New("f not a *mockedFrameFiltererFilterer")
				}
				if v.name != "abuffersink" {
					return nil, fmt.Errorf("invalid name %s", v.name)
				}
				filterContextArgs[buffersinkContext.p] = args
				return buffersinkContext, nil
			}
			g.onNewBuffersrcFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersrcContexter, error) {
				v, ok := f.(*mockedFrameFiltererFilterer)
				if !ok {
					return nil, errors.New("f not a *mockedFrameFiltererFilterer")
				}
				if v.name != "abuffer" {
					return nil, fmt.Errorf("invalid name %s", v.name)
				}
				ctx := buffersrcContext1
				if count > 0 {
					ctx = buffersrcContext2
				}
				filterContextArgs[ctx.p] = args
				count++
				return ctx, nil
			}
		}
		ff2, err := NewFrameFilterer(FrameFiltererOptions{
			Filterer: FrameFiltererFiltererOptions{
				Aliases: map[astiflow.Noder]string{
					nr1: alias1,
					nr2: alias2,
				},
				Content: content,
				OutgoingFrameDescriptorAdapter: func(fd *FrameDescriptor) {
					fd.MediaDescriptor.Rotation = 1
				},
			},
			Group: g,
		})
		require.NoError(t, err)
		fd1 = FrameDescriptor{
			ChannelLayout: astiav.ChannelLayout22,
			MediaDescriptor: MediaDescriptor{
				FrameRate: astiav.NewRational(1, 1),
				TimeBase:  astiav.NewRational(1, 2),
			},
			MediaType:    astiav.MediaTypeAudio,
			SampleFormat: astiav.SampleFormatDblp,
			SampleRate:   13,
		}
		fd2 = FrameDescriptor{
			ChannelLayout: astiav.ChannelLayout22Point2,
			MediaDescriptor: MediaDescriptor{
				FrameRate: astiav.NewRational(2, 1),
				TimeBase:  astiav.NewRational(2, 2),
			},
			MediaType:    astiav.MediaTypeAudio,
			SampleFormat: astiav.SampleFormatFlt,
			SampleRate:   23,
		}
		require.NoError(t, err)
		require.NoError(t, ff2.OnConnect(fd1, nr1))
		require.NoError(t, ff2.OnConnect(fd2, nr2))
		require.Equal(t, 1, len(gs.gs))
		require.Equal(t, 0, gs.gs[0].threadCount)
		require.Equal(t, astiav.ThreadTypeUndefined, gs.gs[0].threadType)
		require.Nil(t, filterContextArgs[buffersinkContext.p])
		contains = []astiav.FilterArgs{
			{"channel_layout": "quad(side)", "sample_fmt": "dblp", "sample_rate": "13", "time_base": "1/2"},
			{"channel_layout": "22.2", "sample_fmt": "flt", "sample_rate": "23", "time_base": "2/2"},
		}
		require.Contains(t, contains, filterContextArgs[buffersrcContext1.p])
		if filterContextArgs[buffersrcContext1.p]["channel_layout"] == contains[0]["channel_layout"] {
			contains = []astiav.FilterArgs{contains[1]}
		} else {
			contains = []astiav.FilterArgs{contains[0]}
		}
		require.Contains(t, contains, filterContextArgs[buffersrcContext2.p])
		require.NotNil(t, ff2.f)
		require.Equal(t, FrameDescriptor{
			ChannelLayout: buffersinkContext.cl,
			MediaDescriptor: MediaDescriptor{
				FrameRate: buffersinkContext.fr,
				Rotation:  1,
				TimeBase:  buffersinkContext.tb,
			},
			MediaType:    buffersinkContext.mt,
			SampleFormat: buffersinkContext.sf,
			SampleRate:   buffersinkContext.sr,
		}, ff2.f.outgoingFrameDescriptor)

		// Connecting children to frame filterer works properly

		h := newMockedFrameHandler()
		h.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h})
		require.NoError(t, err)
		ff3, err := NewFrameFilterer(FrameFiltererOptions{
			Filterer: FrameFiltererFiltererOptions{
				Aliases: map[astiflow.Noder]string{
					nr1: alias1,
					nr2: alias2,
				},
			},
			Group: g,
		})
		require.NoError(t, err)
		require.Error(t, ff3.Connect(h))

		err1 := errors.New("test")
		h.onConnect = func(d FrameDescriptor, n astiflow.Noder) error { return err1 }
		err = ff2.Connect(h)
		require.Error(t, err)
		require.ErrorIs(t, err, err1)

		h.onConnect = func(d FrameDescriptor, n astiflow.Noder) error {
			require.Equal(t, ff2.f.outgoingFrameDescriptor, d)
			return nil
		}
		require.NoError(t, ff2.Connect(h))

		// Filter is closed properly on close
		g.Close()
		require.Equal(t, classersCount, len(classers.p))
		require.True(t, gs.gs[0].freed)
	})
}

func TestFrameFiltererStart(t *testing.T) {
	// Starting with emulate rate should work properly, before dispatch callback should work properly
	withGroup(t, func(f *astiflow.Flow, gp *astiflow.Group, w *astikit.Worker) {
		countNow := 0
		defer astikit.MockNow(func() time.Time {
			countNow++
			switch countNow {
			case 1, 2, 3:
				return time.Unix(1, 0)
			case 4:
				return time.Unix(1, 7e8)
			}
			return time.Unix(2, 0)
		}).Close()

		var sleeps []time.Duration
		defer astikit.MockSleep(func(ctx context.Context, d time.Duration, additionalContexts ...context.Context) {
			sleeps = append(sleeps, d)
		}).Close()

		fs := newMockedFrameFiltererFilterers()
		defer fs.close()
		fs.onFilterer = func(f *mockedFrameFiltererFilterer) *mockedFrameFiltererFilterer {
			f.ps = []*mockedFrameFiltererPader{{mt: astiav.MediaTypeVideo}}
			return f
		}
		gs := newMockedFrameFiltererGraphers()
		defer gs.close()
		gs.onGrapher = func(g *mockedFrameFiltererGrapher) {
			countGetFrame := 0
			g.onNewBuffersinkFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersinkContexter, error) {
				return &mockedFrameFiltererBuffersinkContexter{
					onGetFrame: func(f *astiav.Frame, fs astiav.BuffersinkFlags) error {
						countGetFrame++
						switch countGetFrame {
						case 1, 3, 5:
							f.SetPts(int64(countGetFrame))
							if countGetFrame == 5 {
								require.NoError(t, gp.Stop())
							}
							return nil
						}
						return astiav.ErrEagain
					},
					mt: astiav.MediaTypeVideo,
					p:  &astiav.FilterContext{},
				}, nil
			}
			g.onParseSegment = func(content string) (frameFiltererSegmenter, error) {
				return &mockedFrameFiltererSegmenter{cs: []*mockedFrameFiltererChainer{{ps: []*mockedFrameFiltererParamers{{}}}}}, nil
			}
		}

		const height = 1
		ff, err := NewFrameFilterer(FrameFiltererOptions{
			Filterer: FrameFiltererFiltererOptions{
				EmulateRate: astiav.NewRational(2, 1),
			},
			Group:            gp,
			OnBeforeDispatch: func(f *astiav.Frame) { f.SetHeight(height) },
		})
		require.NoError(t, err)

		h := newMockedFrameHandler()
		h.Node, _, err = gp.NewNode(astiflow.NodeOptions{Noder: h})
		require.NoError(t, err)
		require.NoError(t, ff.Connect(h))

		type frame struct {
			height int
			pts    int64
			sleeps []time.Duration
		}
		var frames []frame
		h.onFrame = func(f Frame) {
			frames = append(frames, frame{
				height: f.Frame.Height(),
				pts:    f.Pts(),
				sleeps: sleeps,
			})
			sleeps = []time.Duration{}
		}

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return ff.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Equal(t, []frame{
			{
				height: 1,
				pts:    1,
			},
			{
				height: 1,
				pts:    3,
				sleeps: []time.Duration{500 * time.Millisecond},
			},
			{
				height: 1,
				pts:    5,
				sleeps: []time.Duration{300 * time.Millisecond},
			},
		}, frames)
	})

	// Starting with aliases should work properly, filter should be refreshed and closed properly
	// when incoming frame descriptor changes, filter should be closed properly on write buffersrc
	// errors, stats should be correct, frame descriptors delta callback should work properly, acquired
	// frames should be released properly
	withGroup(t, func(f *astiflow.Flow, gp *astiflow.Group, w *astikit.Worker) {
		nr1 := mocks.NewMockedNoder()
		nr2 := mocks.NewMockedNoder()

		fd1 := FrameDescriptor{
			Height:    1,
			MediaType: astiav.MediaTypeVideo,
		}
		fd2 := FrameDescriptor{
			Height:    2,
			MediaType: astiav.MediaTypeVideo,
		}
		fd3 := FrameDescriptor{Height: 3}

		fs := newMockedFrameFiltererFilterers()
		defer fs.close()
		gs := newMockedFrameFiltererGraphers()
		defer gs.close()
		countGrapher := 0
		countSetPTS := 0
		gs.onGrapher = func(g *mockedFrameFiltererGrapher) {
			countGrapher++
			countBuffersinkGetFrame := 0
			countBuffersrcAddFrame := 0
			g.onNewBuffersinkFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersinkContexter, error) {
				return &mockedFrameFiltererBuffersinkContexter{
					h: countGrapher,
					onGetFrame: func(f *astiav.Frame, fs astiav.BuffersinkFlags) error {
						countBuffersinkGetFrame++
						if countBuffersinkGetFrame%2 == 1 {
							countSetPTS++
							f.SetPts(int64(countSetPTS))
							return nil
						}
						return astiav.ErrEagain
					},
					p: &astiav.FilterContext{},
				}, nil
			}
			g.onNewBuffersrcFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersrcContexter, error) {
				return &mockedFrameFiltererBuffersrcContexter{
					onAddFrame: func(f *astiav.Frame, fs astiav.BuffersrcFlags) error {
						countBuffersrcAddFrame++
						if countBuffersrcAddFrame == 3 && countGrapher == 2 {
							return errors.New("test")
						}
						return nil
					},
					p: &astiav.FilterContext{},
				}, nil
			}
		}

		var frameDescriptorsDeltas []FrameDescriptorsDelta
		ff, err := NewFrameFilterer(FrameFiltererOptions{
			Filterer: FrameFiltererFiltererOptions{
				Aliases: map[astiflow.Noder]string{
					nr1: "nr1",
					nr2: "nr2",
				},
				OnIncomingFrameDescriptorsDelta: func(d FrameDescriptorsDelta, ff *FrameFilterer) {
					frameDescriptorsDeltas = append(frameDescriptorsDeltas, d)
				},
			},
			Group: gp,
		})
		require.NoError(t, err)

		require.NoError(t, ff.OnConnect(fd1, nr1))
		require.NoError(t, ff.OnConnect(fd2, nr2))

		h := newMockedFrameHandler()
		h.Node, _, err = gp.NewNode(astiflow.NodeOptions{Noder: h})
		require.NoError(t, err)
		require.NoError(t, ff.Connect(h))
		type frame struct {
			height int
			pts    int64
		}
		var frames []frame
		h.onFrame = func(f Frame) {
			frames = append(frames, frame{
				height: f.FrameDescriptor.Height,
				pts:    f.Pts(),
			})
			switch len(frames) {
			case 2:
				require.Equal(t, 2, len(ff.fp.fs))

				// Filter was closed due to incoming frame descriptor change and was refreshed
				require.Equal(t, 2, len(gs.gs))
				require.True(t, gs.gs[0].freed)
			case 3:
				require.Equal(t, 6, len(ff.fp.fs))

				// Filter was closed due to buffersrc add and was refreshed
				require.Equal(t, 3, len(gs.gs))
				require.True(t, gs.gs[1].freed)
				require.NoError(t, gp.Stop())
			}
		}

		dss := ff.DeltaStats()

		f1 := astiav.AllocFrame()
		defer f1.Free()
		f1.SetHeight(2)
		f1.SetPixelFormat(astiav.PixelFormatRgba)
		f1.SetWidth(2)
		require.NoError(t, f1.AllocBuffer(0))
		ff.HandleFrame(Frame{
			Frame:           f1,
			FrameDescriptor: fd1,
			Noder:           nr1,
		})
		ff.HandleFrame(Frame{
			Frame:           f1,
			FrameDescriptor: fd1,
			Noder:           nr1,
		})
		ff.HandleFrame(Frame{
			Frame:           f1,
			FrameDescriptor: fd2,
			Noder:           nr2,
		})
		ff.HandleFrame(Frame{
			Frame:           f1,
			FrameDescriptor: fd1,
			Noder:           nr1,
		})
		ff.HandleFrame(Frame{
			Frame:           f1,
			FrameDescriptor: fd3,
			Noder:           nr2,
		})
		ff.HandleFrame(Frame{
			Frame:           f1,
			FrameDescriptor: fd3,
			Noder:           nr2,
		})
		ff.HandleFrame(Frame{
			Frame:           f1,
			FrameDescriptor: fd1,
			Noder:           nr1,
		})
		ff.HandleFrame(Frame{
			Frame:           f1,
			FrameDescriptor: fd3,
			Noder:           nr2,
		})
		ff.HandleFrame(Frame{
			Frame:           f1,
			FrameDescriptor: fd1,
			Noder:           nr1,
		})

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return ff.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		cs := ff.CumulativeStats()
		require.Greater(t, cs.WorkedDuration, time.Duration(0))
		cs.WorkedDuration = 0
		require.Equal(t, FrameFiltererCumulativeStats{
			FrameHandlerCumulativeStats: FrameHandlerCumulativeStats{
				AllocatedFrames: 10,
				IncomingFrames:  9,
				ProcessedFrames: 9,
			},
			OutgoingFrames: 3,
		}, cs)
		requireDeltaStats(t, map[string]interface{}{
			DeltaStatNameAllocatedFrames:        uint64(10),
			astiflow.DeltaStatNameIncomingRate:  float64(9),
			astiflow.DeltaStatNameOutgoingRate:  float64(3),
			astiflow.DeltaStatNameProcessedRate: float64(9),
			astikit.StatNameWorkedRatio:         nil,
		}, dss)
		require.Equal(t, []FrameDescriptorsDelta{{
			"nr1": FrameDescriptorDelta{
				After:  fd1,
				Before: fd1,
			},
			"nr2": FrameDescriptorDelta{
				After:  fd3,
				Before: fd2,
			},
		}}, frameDescriptorsDeltas)
		require.Equal(t, []frame{
			{
				height: 1,
				pts:    1,
			},
			{
				height: 2,
				pts:    2,
			},
			{
				height: 3,
				pts:    3,
			},
		}, frames)
	})

	// Synchronize incoming frames with PTS should work properly
	withGroup(t, func(f *astiflow.Flow, gp *astiflow.Group, w *astikit.Worker) {
		fs := newMockedFrameFiltererFilterers()
		defer fs.close()
		gs := newMockedFrameFiltererGraphers()
		defer gs.close()
		var buffersrcAddFramePts []int64
		gs.onGrapher = func(g *mockedFrameFiltererGrapher) {
			countBuffersinkGetFrame := 0
			g.onNewBuffersinkFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersinkContexter, error) {
				return &mockedFrameFiltererBuffersinkContexter{
					onGetFrame: func(f *astiav.Frame, fs astiav.BuffersinkFlags) error {
						countBuffersinkGetFrame++
						if countBuffersinkGetFrame%2 == 1 {
							return nil
						}
						return astiav.ErrEagain
					},
					p: &astiav.FilterContext{},
				}, nil
			}
			g.onNewBuffersrcFilterContext = func(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersrcContexter, error) {
				return &mockedFrameFiltererBuffersrcContexter{
					onAddFrame: func(f *astiav.Frame, fs astiav.BuffersrcFlags) error {
						buffersrcAddFramePts = append(buffersrcAddFramePts, f.Pts())
						return nil
					},
					p: &astiav.FilterContext{},
				}, nil
			}
		}

		nr1 := mocks.NewMockedNoder()
		nr2 := mocks.NewMockedNoder()
		ff, err := NewFrameFilterer(FrameFiltererOptions{
			Filterer: FrameFiltererFiltererOptions{
				Aliases: map[astiflow.Noder]string{
					nr1: "nr1",
					nr2: "nr2",
				},
				SynchronizeIncomingFramesWithPTS: true,
			},
			Group: gp,
		})
		require.NoError(t, err)

		fd := FrameDescriptor{MediaType: astiav.MediaTypeVideo}
		require.NoError(t, ff.OnConnect(fd, nr1))
		require.NoError(t, ff.OnConnect(fd, nr2))

		h := newMockedFrameHandler()
		h.Node, _, err = gp.NewNode(astiflow.NodeOptions{Noder: h})
		require.NoError(t, err)
		require.NoError(t, ff.Connect(h))
		type frame struct {
			pts int64
		}
		var frames []frame
		h.onFrame = func(f Frame) {
			q, ok := ff.q.(*ptsFrameFiltererQueuer)
			require.True(t, ok)
			frames = append(frames, frame{
				pts: f.Pts(),
			})
			switch len(frames) {
			case 1:
				require.Equal(t, 0, len(ff.fp.fs))
				require.Equal(t, 0, len(q.slots))
			case 2:
				require.Equal(t, 3, len(ff.fp.fs))
				require.Equal(t, 1, len(q.slots))
			case 3:
				require.Equal(t, 5, len(ff.fp.fs))
				require.Equal(t, 0, len(q.slots))
				require.NoError(t, gp.Stop())
			}
		}

		f1 := astiav.AllocFrame()
		defer f1.Free()
		f1.SetHeight(2)
		f1.SetPixelFormat(astiav.PixelFormatRgba)
		f1.SetWidth(2)
		require.NoError(t, f1.AllocBuffer(0))
		for _, v := range []struct {
			n   astiflow.Noder
			pts int64
		}{
			{
				n:   nr1,
				pts: 1,
			},
			{
				n:   nr1,
				pts: 2,
			},
			{
				n:   nr2,
				pts: 2,
			},
			{
				n:   nr1,
				pts: 4,
			},
			{
				n:   nr2,
				pts: 3,
			},
			{
				n:   nr1,
				pts: 3,
			},
			{
				n:   nr2,
				pts: 4,
			},
			{
				n:   nr1,
				pts: 5,
			},
		} {
			f1.SetPts(v.pts)
			ff.HandleFrame(Frame{
				Frame:           f1,
				FrameDescriptor: fd,
				Noder:           v.n,
			})
		}

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return ff.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Equal(t, []int64{2, 2, 3, 3, 4, 4}, buffersrcAddFramePts)
		require.Equal(t, []frame{
			{pts: 2},
			{pts: 3},
			{pts: 4},
		}, frames)
	})
}
