package astiavflow

import (
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

var _ decoderReader = (*mockedDecoderReader)(nil)

type mockedDecoderReader struct {
	c                *astiav.Codec
	cp               *astiav.CodecParameters
	freed            bool
	opened           bool
	previous         func(c *astiav.Codec) decoderReader
	receiveFrameFunc func(f *astiav.Frame) error
	sendPacketFunc   func(p *astiav.Packet) error
}

func newMockedDecoderReader() *mockedDecoderReader {
	r := &mockedDecoderReader{previous: newDecoderReader}
	newDecoderReader = func(c *astiav.Codec) decoderReader {
		r.c = c
		return r
	}
	return r
}

func (r *mockedDecoderReader) close() {
	newDecoderReader = r.previous
}

func (r *mockedDecoderReader) Class() *astiav.Class {
	return nil
}

func (r *mockedDecoderReader) Free() {
	r.freed = true
}

func (r *mockedDecoderReader) FromCodecParameters(cp *astiav.CodecParameters) error {
	r.cp = cp
	return nil
}

func (r *mockedDecoderReader) Open(c *astiav.Codec, d *astiav.Dictionary) error {
	r.opened = true
	return nil
}

func (r *mockedDecoderReader) ReceiveFrame(f *astiav.Frame) error {
	if r.receiveFrameFunc != nil {
		return r.receiveFrameFunc(f)
	}
	return nil
}

func (r *mockedDecoderReader) SendPacket(p *astiav.Packet) error {
	if r.sendPacketFunc != nil {
		return r.sendPacketFunc(p)
	}
	return nil
}

func TestNewDecoder(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDecoderReader()
		defer r.close()
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

func TestDecoderStart(t *testing.T) {
	// Reader should be flushed on stop and freed on close, stats should be correct,
	// reader should be created properly, reader should read properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDecoderReader()
		defer r.close()
		ps := []int64{}
		r.sendPacketFunc = func(p *astiav.Packet) error {
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
		require.Len(t, dss, 8)

		type frame struct {
			mc  MediaContext
			n   astiflow.Noder
			pts int64
		}
		fs1 := []frame{}
		fh1 := newMockedFrameHandler()
		fh1.handleFrameFunc = func(f Frame) {
			fs1 = append(fs1, frame{
				mc:  f.MediaContext,
				n:   f.Noder,
				pts: f.Pts(),
			})
		}
		fh1.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: fh1})
		require.NoError(t, err)
		fs2 := []frame{}
		fh2 := newMockedFrameHandler()
		fh2.handleFrameFunc = func(f Frame) {
			fs2 = append(fs2, frame{
				mc:  f.MediaContext,
				n:   f.Noder,
				pts: f.Pts(),
			})
		}
		fh2.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: fh2})
		require.NoError(t, err)

		count := 0
		r.receiveFrameFunc = func(f *astiav.Frame) error {
			count++
			if count == 1 {
				n, ok := classers.get(r)
				require.True(t, ok)
				require.Equal(t, d.n, n)
			}
			if count == 3 {
				d.Disconnect(fh2)
			}
			f.SetPts(int64(count))
			switch count {
			case 1, 3, 5:
				return nil
			}
			if count == 4 {
				require.NoError(t, g.Stop())
			}
			return astiav.ErrEagain
		}

		d.Connect(fh1)
		d.Connect(fh2)

		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		cp.SetCodecID(astiav.CodecIDMjpeg)
		mc := MediaContext{TimeBase: astiav.NewRational(1, 10)}
		pkt := astiav.AllocPacket()
		defer pkt.Free()
		require.NoError(t, pkt.AllocPayload(1))
		packetSize := 2
		pkt.SetSize(packetSize)
		pkt.SetPts(1)
		d.HandlePacket(Packet{
			CodecParameters: cp,
			Packet:          pkt,
			MediaContext:    mc,
		})
		pkt.SetPts(3)
		d.HandlePacket(Packet{
			CodecParameters: cp,
			Packet:          pkt,
			MediaContext:    mc,
		})

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return d.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		cs := d.CumulativeStats()
		require.Greater(t, cs.WorkedDuration, time.Duration(0))
		cs.WorkedDuration = 0
		require.Equal(t, DecoderCumulativeStats{
			AllocatedFrames:  1,
			AllocatedPackets: 2,
			IncomingBytes:    uint64(packetSize * 2),
			IncomingPackets:  2,
			OutgoingFrames:   3,
			ProcessedBytes:   uint64(packetSize * 2),
			ProcessedPackets: 2,
		}, cs)
		require.Equal(t, uint64(1), dss[0].Valuer.Value(time.Second))
		require.Equal(t, uint64(2), dss[1].Valuer.Value(time.Second))
		require.Equal(t, float64(3), dss[2].Valuer.Value(time.Second))
		require.Greater(t, dss[3].Valuer.Value(time.Second), 0.0)
		require.Equal(t, float64(2), dss[4].Valuer.Value(time.Second))
		require.Equal(t, float64(packetSize*2), dss[5].Valuer.Value(time.Second))
		require.Equal(t, float64(packetSize*2), dss[6].Valuer.Value(time.Second))
		require.Equal(t, float64(2), dss[7].Valuer.Value(time.Second))
		require.Equal(t, []int64{1, 3, -1}, ps)
		require.Equal(t, []frame{
			{
				mc:  mc,
				n:   d,
				pts: 1,
			},
			{
				mc:  mc,
				n:   d,
				pts: 3,
			},
			{
				mc:  mc,
				n:   d,
				pts: 5,
			},
		}, fs1)
		require.Equal(t, []frame{
			{
				mc:  mc,
				n:   d,
				pts: 1,
			},
		}, fs2)
		require.True(t, r.freed)
		require.True(t, r.opened)
		require.Equal(t, astiav.CodecIDMjpeg, r.c.ID())
		require.Equal(t, cp, r.cp)
		_, ok := classers.get(r)
		require.False(t, ok)
	})

	// Enforce monotonic DTS should work properly, reader options should be processed properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDecoderReader()
		defer r.close()
		r.receiveFrameFunc = func(f *astiav.Frame) error { return astiav.ErrEagain }
		ps := []int64{}
		r.sendPacketFunc = func(p *astiav.Packet) error {
			if p != nil {
				if p.Dts() == 2 {
					require.NoError(t, g.Stop())
				}
				ps = append(ps, p.Dts())
			}
			return nil
		}

		d, err := NewDecoder(DecoderOptions{
			EnforceMonotonicDTS: true,
			Group:               g,
			Reader:              func(p Packet) (decoderName string) { return "aac" },
		})
		require.NoError(t, err)

		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		cp.SetCodecID(astiav.CodecIDMjpeg)
		mc := MediaContext{TimeBase: astiav.NewRational(1, 10)}
		pkt := astiav.AllocPacket()
		defer pkt.Free()
		require.NoError(t, pkt.AllocPayload(1))
		pkt.SetDts(1)
		d.HandlePacket(Packet{
			CodecParameters: cp,
			Packet:          pkt,
			MediaContext:    mc,
		})
		pkt.SetDts(0)
		d.HandlePacket(Packet{
			CodecParameters: cp,
			Packet:          pkt,
			MediaContext:    mc,
		})
		pkt.SetDts(2)
		d.HandlePacket(Packet{
			CodecParameters: cp,
			Packet:          pkt,
			MediaContext:    mc,
		})

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return d.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Equal(t, []int64{1, 2}, ps)
		require.Equal(t, astiav.CodecIDAac, r.c.ID())
	})
}
