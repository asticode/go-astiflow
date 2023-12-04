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

type mockedDecoderReaders struct {
	previous         func(c *astiav.Codec) decoderReader
	receiveFrameFunc func(f *astiav.Frame) error
	rs               []*mockedDecoderReader
	sendPacketFunc   func(p *astiav.Packet) error
}

func newMockedDecoderReaders() *mockedDecoderReaders {
	rs := &mockedDecoderReaders{previous: newDecoderReader}
	newDecoderReader = func(c *astiav.Codec) decoderReader {
		r := newMockedDecoderReader(c, rs)
		rs.rs = append(rs.rs, r)
		return r
	}
	return rs
}

func (w *mockedDecoderReaders) close() {
	newDecoderReader = w.previous
}

var _ decoderReader = (*mockedDecoderReader)(nil)

type mockedDecoderReader struct {
	c         *astiav.Codec
	cpCodecID astiav.CodecID
	freed     bool
	opened    bool
	rs        *mockedDecoderReaders
	tc        int
	tt        astiav.ThreadType
}

func newMockedDecoderReader(c *astiav.Codec, rs *mockedDecoderReaders) *mockedDecoderReader {
	return &mockedDecoderReader{
		c:  c,
		rs: rs,
	}
}

func (r *mockedDecoderReader) Class() *astiav.Class {
	return nil
}

func (r *mockedDecoderReader) Free() {
	r.freed = true
}

func (r *mockedDecoderReader) FromCodecParameters(cp *astiav.CodecParameters) error {
	r.cpCodecID = cp.CodecID()
	return nil
}

func (r *mockedDecoderReader) Open(c *astiav.Codec, d *astiav.Dictionary) error {
	r.opened = true
	return nil
}

func (r *mockedDecoderReader) ReceiveFrame(f *astiav.Frame) error {
	if r.rs.receiveFrameFunc != nil {
		return r.rs.receiveFrameFunc(f)
	}
	return astiav.ErrEof
}

func (r *mockedDecoderReader) SendPacket(p *astiav.Packet) error {
	if r.rs.sendPacketFunc != nil {
		return r.rs.sendPacketFunc(p)
	}
	return astiav.ErrEof
}

func (r *mockedDecoderReader) SetThreadCount(i int) {
	r.tc = i
}

func (r *mockedDecoderReader) SetThreadType(tt astiav.ThreadType) {
	r.tt = tt
}

func TestNewDecoder(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		countDecoder = 0
		d, err := NewDecoder(DecoderOptions{Group: g})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{Name: "decoder_1", Tags: []string{"decoder"}}, d.n.Metadata())
		var emitted bool
		d.On(astiflow.EventNameNodeClosed, func(payload interface{}) (delete bool) {
			emitted = true
			return
		})
		g.Close()
		require.True(t, emitted)
	})

	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		d, err := NewDecoder(DecoderOptions{
			Group:    g,
			Metadata: astiflow.Metadata{Description: "d", Name: "n", Tags: []string{"t"}},
		})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{
			Description: "d",
			Name:        "n",
			Tags:        []string{"decoder", "t"},
		}, d.n.Metadata())
	})
}

func TestDecoderOnConnect(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		rs := newMockedDecoderReaders()
		defer rs.close()
		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		cp.SetCodecID(astiav.CodecIDMjpeg)
		d1, err := NewDecoder(DecoderOptions{Group: g})
		require.NoError(t, err)
		d2, err := NewDecoder(DecoderOptions{
			Group:  g,
			Reader: func(d PacketDescriptor) (decoderName string) { return "invalid" },
		})
		require.NoError(t, err)

		require.NoError(t, d1.OnConnect(PacketDescriptor{CodecParameters: cp}, nil))
		require.NotNil(t, d1.r)
		pd, ok := d1.initialIncomingPacketDescriptors.one()
		require.True(t, ok)
		require.Equal(t, astiav.CodecIDMjpeg, pd.CodecParameters.CodecID())
		require.Error(t, d2.OnConnect(PacketDescriptor{CodecParameters: cp}, nil))
	})
}

func TestDecoderConnect(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		rs := newMockedDecoderReaders()
		defer rs.close()
		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		cp.SetCodecID(astiav.CodecIDMjpeg)
		cp.SetHeight(1)
		cp.SetMediaType(astiav.MediaTypeVideo)
		pd := PacketDescriptor{CodecParameters: cp}
		h := newMockedFrameHandler()
		var err error
		h.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h})
		require.NoError(t, err)

		d, err := NewDecoder(DecoderOptions{Group: g})
		require.NoError(t, err)

		e := errors.New("test")
		h.onConnect = func(d FrameDescriptor, n astiflow.Noder) error { return e }
		err = d.Connect(h)
		require.Error(t, err)
		require.NotErrorIs(t, err, e)

		require.NoError(t, d.OnConnect(pd, nil))
		require.ErrorIs(t, d.Connect(h), e)

		var fd FrameDescriptor
		h.onConnect = func(d FrameDescriptor, n astiflow.Noder) error {
			fd = d
			return nil
		}
		require.NoError(t, d.Connect(h))
		require.Equal(t, 1, fd.Height)
	})
}

func TestDecoderStart(t *testing.T) {
	// Reader should be flushed on stop and freed on close, stats should be correct,
	// reader should be created properly, reader should read properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		rs := newMockedDecoderReaders()
		defer rs.close()
		ps := []int64{}
		rs.sendPacketFunc = func(p *astiav.Packet) error {
			pts := int64(-1)
			if p != nil {
				pts = p.Pts()
			}
			ps = append(ps, pts)
			return nil
		}

		d, err := NewDecoder(DecoderOptions{Group: g})
		require.NoError(t, err)

		dss := d.DeltaStats()

		type frame struct {
			fd  FrameDescriptor
			n   astiflow.Noder
			pts int64
		}
		fs1 := []frame{}
		fh1 := newMockedFrameHandler()
		fh1.onFrame = func(f Frame) {
			fs1 = append(fs1, frame{
				fd:  f.FrameDescriptor,
				n:   f.Noder,
				pts: f.Pts(),
			})
		}
		fh1.Node, _, err = g.NewNode(astiflow.NodeOptions{
			Noder: fh1,
			Stop:  &astiflow.NodeStopOptions{WhenAllParentsAreDone: true},
		})
		require.NoError(t, err)
		fs2 := []frame{}
		fh2 := newMockedFrameHandler()
		fh2.onFrame = func(f Frame) {
			fs2 = append(fs2, frame{
				fd:  f.FrameDescriptor,
				n:   f.Noder,
				pts: f.Pts(),
			})
		}
		fh2.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: fh2})
		require.NoError(t, err)

		count := 0
		rs.receiveFrameFunc = func(f *astiav.Frame) error {
			count++
			if count == 1 {
				require.Len(t, rs.rs, 1)
				n, ok := classers.get(rs.rs[0])
				require.True(t, ok)
				require.Equal(t, d.n, n)
			}
			if count == 4 {
				d.Disconnect(fh2)
			}
			f.SetChannelLayout(astiav.ChannelLayout21)
			f.SetPts(int64(count))
			switch count {
			case 1, 2, 4, 6:
				return nil
			}
			if count == 5 {
				require.NoError(t, g.Stop())
			}
			return astiav.ErrEagain
		}

		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		cp.SetCodecID(astiav.CodecIDMjpeg)
		cp.SetMediaType(astiav.MediaTypeAudio)
		md := MediaDescriptor{TimeBase: astiav.NewRational(1, 10)}
		pd := PacketDescriptor{
			CodecParameters: cp,
			MediaDescriptor: md,
		}
		require.NoError(t, d.OnConnect(pd, nil))
		require.NoError(t, d.Connect(fh1))
		require.NoError(t, d.Connect(fh2))

		pkt := astiav.AllocPacket()
		defer pkt.Free()
		require.NoError(t, pkt.AllocPayload(1))
		packetSize := 2
		pkt.SetSize(packetSize)
		for _, pts := range []int64{1, 3} {
			pkt.SetPts(pts)
			d.HandlePacket(Packet{
				Packet:           pkt,
				PacketDescriptor: pd,
			})
		}

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return d.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		cs := d.CumulativeStats()
		require.Greater(t, cs.WorkedDuration, time.Duration(0))
		cs.WorkedDuration = 0
		require.Equal(t, DecoderCumulativeStats{
			AllocatedFrames: 1,
			OutgoingFrames:  4,
			PacketHandlerCumulativeStats: PacketHandlerCumulativeStats{
				AllocatedCodecParameters: 3,
				AllocatedPackets:         2,
				IncomingBytes:            uint64(packetSize * 2),
				IncomingPackets:          2,
				ProcessedBytes:           uint64(packetSize * 2),
				ProcessedPackets:         2,
			},
		}, cs)
		requireDeltaStats(t, map[string]interface{}{
			DeltaStatNameAllocatedCodecParameters:   uint64(3),
			DeltaStatNameAllocatedFrames:            uint64(1),
			DeltaStatNameAllocatedPackets:           uint64(2),
			astiflow.DeltaStatNameIncomingByteRate:  float64(packetSize * 2),
			astiflow.DeltaStatNameIncomingRate:      float64(2),
			astiflow.DeltaStatNameOutgoingRate:      float64(4),
			astiflow.DeltaStatNameProcessedByteRate: float64(packetSize * 2),
			astiflow.DeltaStatNameProcessedRate:     float64(2),
			astikit.StatNameWorkedRatio:             nil,
		}, dss)
		require.Equal(t, []int64{1, 3, -1}, ps)
		fd := FrameDescriptor{
			ChannelLayout:   astiav.ChannelLayout21,
			MediaDescriptor: md,
			MediaType:       astiav.MediaTypeAudio,
			SampleFormat:    astiav.SampleFormatNone,
		}
		require.Equal(t, []frame{
			{
				fd:  fd,
				n:   d,
				pts: 1,
			},
			{
				fd:  fd,
				n:   d,
				pts: 2,
			},
			{
				fd:  fd,
				n:   d,
				pts: 4,
			},
			{
				fd:  fd,
				n:   d,
				pts: 6,
			},
		}, fs1)
		require.Equal(t, []frame{
			{
				fd:  fd,
				n:   d,
				pts: 1,
			},
			{
				fd:  fd,
				n:   d,
				pts: 2,
			},
		}, fs2)
		require.Len(t, rs.rs, 1)
		require.True(t, rs.rs[0].freed)
		require.True(t, rs.rs[0].opened)
		require.Equal(t, astiav.CodecIDMjpeg, rs.rs[0].c.ID())
		require.Equal(t, astiav.CodecIDMjpeg, rs.rs[0].cpCodecID)
		_, ok := classers.get(rs.rs[0])
		require.False(t, ok)
	})

	// Enforce monotonic DTS should work properly, reader options should be processed properly,
	// thread options should be processed properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		rs := newMockedDecoderReaders()
		defer rs.close()
		rs.receiveFrameFunc = func(f *astiav.Frame) error { return astiav.ErrEagain }
		ps := []int64{}
		rs.sendPacketFunc = func(p *astiav.Packet) error {
			if p != nil {
				if p.Dts() == 2 {
					require.NoError(t, g.Stop())
				}
				ps = append(ps, p.Dts())
			}
			return nil
		}

		threadCount := 1
		threadType := astiav.ThreadTypeFrame
		d, err := NewDecoder(DecoderOptions{
			EnforceMonotonicDTS: true,
			Group:               g,
			Reader:              func(pd PacketDescriptor) (decoderName string) { return "aac" },
			ThreadCount:         threadCount,
			ThreadType:          threadType,
		})
		require.NoError(t, err)

		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		cp.SetCodecID(astiav.CodecIDMjpeg)
		md := MediaDescriptor{TimeBase: astiav.NewRational(1, 10)}
		pkt := astiav.AllocPacket()
		defer pkt.Free()
		require.NoError(t, pkt.AllocPayload(1))
		pkt.SetDts(1)
		d.HandlePacket(Packet{
			Packet: pkt,
			PacketDescriptor: PacketDescriptor{
				CodecParameters: cp,
				MediaDescriptor: md,
			},
		})
		pkt.SetDts(0)
		d.HandlePacket(Packet{
			Packet: pkt,
			PacketDescriptor: PacketDescriptor{
				CodecParameters: cp,
				MediaDescriptor: md,
			},
		})
		pkt.SetDts(2)
		d.HandlePacket(Packet{
			Packet: pkt,
			PacketDescriptor: PacketDescriptor{
				CodecParameters: cp,
				MediaDescriptor: md,
			},
		})

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return d.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Equal(t, []int64{1, 2}, ps)
		require.Len(t, rs.rs, 1)
		require.Equal(t, astiav.CodecIDAac, rs.rs[0].c.ID())
		require.Equal(t, threadCount, rs.rs[0].tc)
		require.Equal(t, threadType, rs.rs[0].tt)
	})
}
