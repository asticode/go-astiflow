package astiavflow

import (
	"errors"
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

type mockedEncoderWriters struct {
	previous          func(c *astiav.Codec) encoderWriter
	receivePacketFunc func(p *astiav.Packet) error
	sendFrameFunc     func(f *astiav.Frame) error
	ws                []*mockedEncoderWriter
}

func newMockedEncoderWriters() *mockedEncoderWriters {
	ws := &mockedEncoderWriters{previous: newEncoderWriter}
	newEncoderWriter = func(c *astiav.Codec) encoderWriter {
		w := newMockedEncoderWriter(c, ws)
		ws.ws = append(ws.ws, w)
		return w
	}
	return ws
}

func (w *mockedEncoderWriters) close() {
	newEncoderWriter = w.previous
}

var _ encoderWriter = (*mockedEncoderWriter)(nil)

type mockedEncoderWriter struct {
	bitRate             int64
	c                   *astiav.Codec
	cl                  astiav.ChannelLayout
	cp                  *astiav.CodecParameters
	flags               astiav.CodecContextFlags
	framerate           astiav.Rational
	freed               bool
	gopSize             int
	height              int
	openDictionaryValue string
	opened              bool
	pixFmt              astiav.PixelFormat
	sampleRate          int
	sar                 astiav.Rational
	sf                  astiav.SampleFormat
	threadCount         int
	timeBase            astiav.Rational
	tt                  astiav.ThreadType
	width               int
	ws                  *mockedEncoderWriters
}

func newMockedEncoderWriter(c *astiav.Codec, ws *mockedEncoderWriters) *mockedEncoderWriter {
	return &mockedEncoderWriter{
		c:  c,
		ws: ws,
	}
}

func (w *mockedEncoderWriter) Class() *astiav.Class {
	return nil
}

func (w *mockedEncoderWriter) Flags() astiav.CodecContextFlags {
	return w.flags
}

func (w *mockedEncoderWriter) Framerate() astiav.Rational {
	return w.framerate
}

func (w *mockedEncoderWriter) Free() {
	w.freed = true
}

func (w *mockedEncoderWriter) Open(c *astiav.Codec, d *astiav.Dictionary) error {
	w.opened = true
	if d != nil {
		w.openDictionaryValue = d.Get("k", nil, astiav.NewDictionaryFlags()).Value()
	}
	return nil
}

func (w *mockedEncoderWriter) ReceivePacket(p *astiav.Packet) error {
	if w.ws.receivePacketFunc != nil {
		return w.ws.receivePacketFunc(p)
	}
	return astiav.ErrEof
}

func (w *mockedEncoderWriter) SendFrame(f *astiav.Frame) error {
	if w.ws.sendFrameFunc != nil {
		return w.ws.sendFrameFunc(f)
	}
	return astiav.ErrEof
}

func (w *mockedEncoderWriter) SetBitRate(bitRate int64) {
	w.bitRate = bitRate
}

func (w *mockedEncoderWriter) SetChannelLayout(cl astiav.ChannelLayout) {
	w.cl = cl
}

func (w *mockedEncoderWriter) SetFlags(fs astiav.CodecContextFlags) {
	w.flags = fs
}

func (w *mockedEncoderWriter) SetFramerate(f astiav.Rational) {
	w.framerate = f
}

func (w *mockedEncoderWriter) SetGopSize(gopSize int) {
	w.gopSize = gopSize
}

func (w *mockedEncoderWriter) SetHeight(height int) {
	w.height = height
}

func (w *mockedEncoderWriter) SetPixelFormat(pixFmt astiav.PixelFormat) {
	w.pixFmt = pixFmt
}

func (w *mockedEncoderWriter) SetSampleAspectRatio(r astiav.Rational) {
	w.sar = r
}

func (w *mockedEncoderWriter) SetSampleFormat(f astiav.SampleFormat) {
	w.sf = f
}

func (w *mockedEncoderWriter) SetSampleRate(sampleRate int) {
	w.sampleRate = sampleRate
}

func (w *mockedEncoderWriter) SetThreadCount(threadCount int) {
	w.threadCount = threadCount
}

func (w *mockedEncoderWriter) SetThreadType(t astiav.ThreadType) {
	w.tt = t
}

func (w *mockedEncoderWriter) SetTimeBase(r astiav.Rational) {
	w.timeBase = r
}

func (w *mockedEncoderWriter) SetWidth(width int) {
	w.width = width
}

func (w *mockedEncoderWriter) TimeBase() astiav.Rational {
	return w.timeBase
}

func (w *mockedEncoderWriter) ToCodecParameters(cp *astiav.CodecParameters) error {
	w.cp = cp
	return nil
}

func TestNewEncoder(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		countEncoder = 0
		e, err := NewEncoder(EncoderOptions{Group: g})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{Name: "encoder_1", Tags: []string{"encoder"}}, e.n.Metadata())
		var emitted bool
		e.On(astiflow.EventNameNodeClosed, func(payload interface{}) (delete bool) {
			emitted = true
			return
		})
		g.Close()
		require.True(t, emitted)
	})

	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		e, err := NewEncoder(EncoderOptions{
			Group:    g,
			Metadata: astiflow.Metadata{Description: "d", Name: "n", Tags: []string{"t"}},
		})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{
			Description: "d",
			Name:        "n",
			Tags:        []string{"encoder", "t"},
		}, e.n.Metadata())
	})
}

func TestEncoderOnConnect(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		ws := newMockedEncoderWriters()
		defer ws.close()

		e1, err := NewEncoder(EncoderOptions{
			Group:  g,
			Writer: EncoderWriterOptions{CodecID: astiav.CodecIDMjpeg},
		})
		require.NoError(t, err)
		e2, err := NewEncoder(EncoderOptions{
			Group:  g,
			Writer: EncoderWriterOptions{CodecName: "invalid"},
		})
		require.NoError(t, err)

		fd := FrameDescriptor{
			Height:      1,
			MediaType:   astiav.MediaTypeVideo,
			PixelFormat: astiav.PixelFormatRgba,
			Width:       1,
		}
		require.NoError(t, e1.OnConnect(fd, nil))
		require.NotNil(t, e1.w)
		fd, ok := e1.initialIncomingFrameDescriptors.one()
		require.True(t, ok)
		require.Equal(t, 1, fd.Height)
		require.Error(t, e2.OnConnect(fd, nil))
	})
}

func TestEncoderConnect(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		ws := newMockedEncoderWriters()
		defer ws.close()

		h := newMockedPacketHandler()
		var err error
		h.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h})
		require.NoError(t, err)

		e, err := NewEncoder(EncoderOptions{
			Group:  g,
			Writer: EncoderWriterOptions{CodecID: astiav.CodecIDMjpeg},
		})
		require.NoError(t, err)

		err1 := errors.New("test")
		h.onConnect = func(d PacketDescriptor, n astiflow.Noder) error { return err1 }
		err = e.Connect(h)
		require.Error(t, err)
		require.NotErrorIs(t, err, err1)

		require.NoError(t, e.OnConnect(FrameDescriptor{MediaDescriptor: MediaDescriptor{Rotation: 1}}, nil))
		require.ErrorIs(t, e.Connect(h), err1)

		var pd PacketDescriptor
		h.onConnect = func(d PacketDescriptor, n astiflow.Noder) error {
			pd = d
			return nil
		}
		require.NoError(t, e.Connect(h))
		require.Len(t, ws.ws, 1)
		require.Equal(t, 1.0, pd.MediaDescriptor.Rotation)
	})
}

func TestEncoderStart(t *testing.T) {
	// Writer should be flushed on stop and freed on close, stats should be correct,
	// writer should be created properly, writer should write properly, frame attributes must be reset
	// before being sent, packet duration should be updated if framerate is provided
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		ws := newMockedEncoderWriters()
		defer ws.close()
		fs := []int64{}
		ws.sendFrameFunc = func(f *astiav.Frame) error {
			pts := int64(-1)
			if f != nil {
				require.False(t, f.KeyFrame())
				require.Equal(t, astiav.PictureTypeNone, f.PictureType())
				pts = f.Pts()
			}
			fs = append(fs, pts)
			return nil
		}

		e, err := NewEncoder(EncoderOptions{
			Group:  g,
			Writer: EncoderWriterOptions{CodecID: astiav.CodecIDMjpeg},
		})
		require.NoError(t, err)

		dss := e.DeltaStats()

		type packet struct {
			duration int64
			n        astiflow.Noder
			pd       PacketDescriptor
			pts      int64
		}
		ps1 := []packet{}
		ph1 := newMockedPacketHandler()
		ph1.onPacket = func(p Packet) {
			ps1 = append(ps1, packet{
				duration: p.Duration(),
				n:        p.Noder,
				pd:       p.PacketDescriptor,
				pts:      p.Pts(),
			})
		}
		ph1.Node, _, err = g.NewNode(astiflow.NodeOptions{
			Noder: ph1,
			Stop:  &astiflow.NodeStopOptions{WhenAllParentsAreDone: true},
		})
		require.NoError(t, err)
		ps2 := []packet{}
		ph2 := newMockedPacketHandler()
		ph2.onPacket = func(p Packet) {
			ps2 = append(ps2, packet{
				duration: p.Duration(),
				n:        p.Noder,
				pd:       p.PacketDescriptor,
				pts:      p.Pts(),
			})
		}
		ph2.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: ph2})
		require.NoError(t, err)

		count := 0
		packetSize := 2
		ws.receivePacketFunc = func(p *astiav.Packet) error {
			count++
			if count == 1 {
				require.Len(t, ws.ws, 1)
				n, ok := classers.get(ws.ws[0])
				require.True(t, ok)
				require.Equal(t, e.n, n)
			}
			if count == 4 {
				e.Disconnect(ph2)
			}
			p.SetPts(int64(count))
			p.SetSize(packetSize)
			switch count {
			case 1, 2, 4, 6:
				return nil
			}
			if count == 5 {
				require.NoError(t, g.Stop())
			}
			return astiav.ErrEagain
		}

		md := MediaDescriptor{
			FrameRate: astiav.NewRational(2, 1),
			TimeBase:  astiav.NewRational(1, 10),
		}
		fm := astiav.AllocFrame()
		defer fm.Free()
		fm.SetHeight(2)
		fm.SetKeyFrame(true)
		fm.SetPictureType(astiav.PictureTypeB)
		fm.SetPixelFormat(astiav.PixelFormatRgba)
		fm.SetWidth(2)
		require.NoError(t, fm.AllocBuffer(0))
		fd := newFrameDescriptorFromFrame(fm, md, astiav.MediaTypeVideo)
		require.NoError(t, e.OnConnect(fd, nil))
		require.NoError(t, e.Connect(ph1))
		require.NoError(t, e.Connect(ph2))

		for _, pts := range []int64{1, 3} {
			fm.SetPts(pts)
			e.HandleFrame(Frame{
				Frame:           fm,
				FrameDescriptor: fd,
			})
		}

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return e.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		cs := e.CumulativeStats()
		require.Greater(t, cs.WorkedDuration, time.Duration(0))
		cs.WorkedDuration = 0
		require.Equal(t, EncoderCumulativeStats{
			AllocatedPackets: 1,
			FrameHandlerCumulativeStats: FrameHandlerCumulativeStats{
				AllocatedFrames: 2,
				IncomingFrames:  2,
				ProcessedFrames: 2,
			},
			OutgoingBytes:   uint64(packetSize * 4),
			OutgoingPackets: 4,
		}, cs)
		requireDeltaStats(t, map[string]interface{}{
			DeltaStatNameAllocatedFrames:           uint64(2),
			DeltaStatNameAllocatedPackets:          uint64(1),
			astiflow.DeltaStatNameIncomingRate:     float64(2),
			astiflow.DeltaStatNameOutgoingByteRate: float64(packetSize * 4),
			astiflow.DeltaStatNameOutgoingRate:     float64(4),
			astiflow.DeltaStatNameProcessedRate:    float64(2),
			astikit.StatNameWorkedRatio:            nil,
		}, dss)
		require.Equal(t, []int64{1, 3, -1}, fs)
		pd := PacketDescriptor{
			CodecParameters: e.cp,
			MediaDescriptor: md,
		}
		require.Equal(t, []packet{
			{
				duration: 5,
				n:        e,
				pd:       pd,
				pts:      1,
			},
			{
				duration: 5,
				n:        e,
				pd:       pd,
				pts:      2,
			},
			{
				duration: 5,
				n:        e,
				pd:       pd,
				pts:      4,
			},
			{
				duration: 5,
				n:        e,
				pd:       pd,
				pts:      6,
			},
		}, ps1)
		require.Equal(t, []packet{
			{
				duration: 5,
				n:        e,
				pd:       pd,
				pts:      1,
			},
			{
				duration: 5,
				n:        e,
				pd:       pd,
				pts:      2,
			},
		}, ps2)
		require.Len(t, ws.ws, 1)
		require.True(t, ws.ws[0].freed)
		require.True(t, ws.ws[0].opened)
		require.Equal(t, e.cp, ws.ws[0].cp)
		require.Panics(t, func() { e.cp.BitRate() })
		_, ok := classers.get(ws.ws[0])
		require.False(t, ok)
	})

	// Writer options should be processed properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		ws := newMockedEncoderWriters()
		defer ws.close()
		count := 0
		ws.sendFrameFunc = func(f *astiav.Frame) error {
			count++
			if count == 2 {
				require.NoError(t, g.Stop())
			}
			return astiav.ErrEagain
		}

		wo1 := EncoderWriterOptions{
			BitRate:     1,
			CodecID:     astiav.CodecIDRawvideo,
			Dictionary:  NewCommaDictionaryOptions("k=v"),
			Flags:       astiav.NewCodecContextFlags(astiav.CodecContextFlagGlobalHeader),
			GopSize:     func(framerate astiav.Rational) int { return 2 },
			ThreadCount: 3,
			ThreadType:  astiav.ThreadTypeFrame,
		}
		e1, err := NewEncoder(EncoderOptions{
			Group:  g,
			Writer: wo1,
		})
		require.NoError(t, err)

		wo2 := EncoderWriterOptions{CodecName: "pcm_f32be"}
		e2, err := NewEncoder(EncoderOptions{
			Group:  g,
			Writer: wo2,
		})
		require.NoError(t, err)

		md := MediaDescriptor{
			FrameRate: astiav.NewRational(1, 8),
			TimeBase:  astiav.NewRational(1, 9),
		}

		fm1 := astiav.AllocFrame()
		defer fm1.Free()
		fm1.SetHeight(4)
		fm1.SetPixelFormat(astiav.PixelFormatRgba)
		fm1.SetSampleAspectRatio(astiav.NewRational(1, 5))
		fm1.SetWidth(7)
		require.NoError(t, fm1.AllocBuffer(0))
		e1.HandleFrame(Frame{
			Frame:           fm1,
			FrameDescriptor: newFrameDescriptorFromFrame(fm1, md, astiav.MediaTypeVideo),
		})

		fm2 := astiav.AllocFrame()
		defer fm2.Free()
		fm2.SetChannelLayout(astiav.ChannelLayout21)
		fm2.SetNbSamples(6)
		fm2.SetSampleFormat(astiav.SampleFormatS16)
		fm2.SetSampleRate(6)
		require.NoError(t, fm2.AllocBuffer(0))
		e2.HandleFrame(Frame{
			Frame:           fm2,
			FrameDescriptor: newFrameDescriptorFromFrame(fm2, md, astiav.MediaTypeAudio),
		})

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return e1.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Eventually(t, func() bool { return e2.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Len(t, ws.ws, 2)
		// Since we don't know which of e1 or e2 will process the frame first, we need to do the below tricky logic
		m := make(map[astiav.CodecID]bool)
		for _, w := range ws.ws {
			m[w.c.ID()] = true
			switch w.c.ID() {
			case astiav.CodecIDPcmF32Be:
				require.Equal(t, int64(0), w.bitRate)
				require.Equal(t, fm2.ChannelLayout(), w.cl)
				require.Equal(t, astiav.CodecContextFlags(0), w.flags)
				require.Equal(t, astiav.NewRational(0, 0), w.framerate)
				require.Equal(t, 0, w.gopSize)
				require.Equal(t, 0, w.height)
				require.Equal(t, astiav.PixelFormat(0), w.pixFmt)
				require.Equal(t, astiav.NewRational(0, 0), w.sar)
				require.Equal(t, fm2.SampleFormat(), w.sf)
				require.Equal(t, fm2.SampleRate(), w.sampleRate)
				require.Equal(t, 0, w.threadCount)
				require.Equal(t, astiav.ThreadType(0), w.tt)
				require.Equal(t, md.TimeBase, w.timeBase)
				require.Equal(t, 0, w.width)
				require.Equal(t, "", w.openDictionaryValue)
			case astiav.CodecIDRawvideo:
				require.Equal(t, wo1.BitRate, w.bitRate)
				require.Equal(t, astiav.ChannelLayout{}, w.cl)
				require.Equal(t, wo1.Flags, w.flags)
				require.Equal(t, md.FrameRate, w.framerate)
				require.Equal(t, wo1.GopSize(md.FrameRate), w.gopSize)
				require.Equal(t, fm1.Height(), w.height)
				require.Equal(t, fm1.PixelFormat(), w.pixFmt)
				require.Equal(t, fm1.SampleAspectRatio(), w.sar)
				require.Equal(t, astiav.SampleFormat(0), w.sf)
				require.Equal(t, 0, w.sampleRate)
				require.Equal(t, wo1.ThreadCount, w.threadCount)
				require.Equal(t, wo1.ThreadType, w.tt)
				require.Equal(t, md.TimeBase, w.timeBase)
				require.Equal(t, fm1.Width(), w.width)
				require.Equal(t, "v", w.openDictionaryValue)
			}
		}
		require.Equal(t, map[astiav.CodecID]bool{
			wo1.CodecID:            true,
			astiav.CodecIDPcmF32Be: true,
		}, m)
	})

	// Writer should be refreshed on frame/media context change, writer should be flushed
	// and closed on refresh
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		ws := newMockedEncoderWriters()
		defer ws.close()
		fs := []int64{}
		count := 0
		ws.sendFrameFunc = func(f *astiav.Frame) error {
			count++
			pts := int64(-1)
			if f != nil {
				pts = f.Pts()
			}
			fs = append(fs, pts)
			switch count {
			case 3:
				require.True(t, ws.ws[0].freed)
			case 5:
				require.True(t, ws.ws[1].freed)
				require.NoError(t, g.Stop())
			}
			return nil
		}

		e, err := NewEncoder(EncoderOptions{
			Group:  g,
			Writer: EncoderWriterOptions{CodecID: astiav.CodecIDMjpeg},
		})
		require.NoError(t, err)

		fm := astiav.AllocFrame()
		defer fm.Free()
		fm.SetHeight(1)
		fm.SetPixelFormat(astiav.PixelFormatRgba)
		fm.SetPts(1)
		fm.SetWidth(2)
		require.NoError(t, fm.AllocBuffer(0))
		md := MediaDescriptor{TimeBase: astiav.NewRational(1, 3)}
		e.HandleFrame(Frame{
			Frame:           fm,
			FrameDescriptor: newFrameDescriptorFromFrame(fm, md, astiav.MediaTypeVideo),
		})
		fm.SetHeight(4)
		fm.SetPts(2)
		e.HandleFrame(Frame{
			Frame:           fm,
			FrameDescriptor: newFrameDescriptorFromFrame(fm, md, astiav.MediaTypeVideo),
		})
		fm.SetPts(3)
		md.TimeBase = astiav.NewRational(1, 5)
		e.HandleFrame(Frame{
			Frame:           fm,
			FrameDescriptor: newFrameDescriptorFromFrame(fm, md, astiav.MediaTypeVideo),
		})

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return e.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Len(t, ws.ws, 3)
		require.Equal(t, 1, ws.ws[0].height)
		require.Equal(t, astiav.NewRational(1, 3), ws.ws[0].timeBase)
		require.Equal(t, 4, ws.ws[1].height)
		require.Equal(t, astiav.NewRational(1, 3), ws.ws[1].timeBase)
		require.Equal(t, 4, ws.ws[2].height)
		require.Equal(t, astiav.NewRational(1, 5), ws.ws[2].timeBase)
		require.Equal(t, []int64{1, -1, 2, -1, 3, -1}, fs)
	})
}
