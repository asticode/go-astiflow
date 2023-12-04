package astiavflow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

type mockedMuxerWriters struct {
	previous                  func(format *astiav.OutputFormat, formatName, filename string) (muxerWriter, error)
	writeInterleavedFrameFunc func(p *astiav.Packet) error
	writeHeader               func(*astiav.Dictionary) error
	ws                        []*mockedMuxerWriter
}

func newMockedMuxerWriters() *mockedMuxerWriters {
	ws := &mockedMuxerWriters{previous: newMuxerWriter}
	newMuxerWriter = func(format *astiav.OutputFormat, formatName, filename string) (muxerWriter, error) {
		w := newMockedMuxerWriter(format, formatName, filename, ws)
		ws.ws = append(ws.ws, w)
		return w, nil
	}
	return ws
}

func (ws *mockedMuxerWriters) close() {
	newMuxerWriter = ws.previous
	for _, w := range ws.ws {
		w.close()
	}
}

var _ muxerWriter = (*mockedMuxerWriter)(nil)

type mockedMuxerWriter struct {
	fc         *astiav.FormatContext
	filename   string
	format     *astiav.OutputFormat
	formatName string
	freed      bool
	header     bool
	pb         *astiav.IOContext
	ss         []*astiav.Stream
	trailer    bool
	ws         *mockedMuxerWriters
}

func newMockedMuxerWriter(format *astiav.OutputFormat, formatName, filename string, ws *mockedMuxerWriters) *mockedMuxerWriter {
	return &mockedMuxerWriter{
		fc:         astiav.AllocFormatContext(),
		filename:   filename,
		format:     format,
		formatName: formatName,
		ws:         ws,
	}
}

func (w *mockedMuxerWriter) close() {
	w.fc.Free()
}

func (w *mockedMuxerWriter) Class() *astiav.Class {
	return nil
}

func (w *mockedMuxerWriter) Free() {
	w.freed = true
}

func (w *mockedMuxerWriter) NewStream(c *astiav.Codec) *astiav.Stream {
	s := w.fc.NewStream(c)
	w.ss = append(w.ss, s)
	return s
}

func (w *mockedMuxerWriter) OutputFormat() *astiav.OutputFormat {
	return w.format
}

func (w *mockedMuxerWriter) Pb() *astiav.IOContext {
	return w.pb
}

func (w *mockedMuxerWriter) SetPb(i *astiav.IOContext) {
	w.pb = i
}

func (w *mockedMuxerWriter) WriteHeader(d *astiav.Dictionary) error {
	w.header = true
	if w.ws.writeHeader != nil {
		return w.ws.writeHeader(d)
	}
	return nil
}

func (w *mockedMuxerWriter) WriteInterleavedFrame(p *astiav.Packet) error {
	if w.ws.writeInterleavedFrameFunc != nil {
		return w.ws.writeInterleavedFrameFunc(p)
	}
	return nil
}

func (w *mockedMuxerWriter) WriteTrailer() error {
	w.trailer = true
	return nil
}

func TestNewMuxer(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		countMuxer = 0
		m, err := NewMuxer(MuxerOptions{Group: g})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{Name: "muxer_1", Tags: []string{"muxer"}}, m.n.Metadata())
		var emitted bool
		m.On(astiflow.EventNameNodeClosed, func(payload interface{}) (delete bool) {
			emitted = true
			return
		})
		g.Close()
		require.True(t, emitted)
	})

	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		m, err := NewMuxer(MuxerOptions{
			Group:    g,
			Metadata: astiflow.Metadata{Description: "d", Name: "n", Tags: []string{"t"}},
		})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{
			Description: "d",
			Name:        "n",
			Tags:        []string{"muxer", "t"},
		}, m.n.Metadata())
	})
}

func TestMuxerOpen(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		ws := newMockedMuxerWriters()
		defer ws.close()

		m, err := NewMuxer(MuxerOptions{Group: g})
		require.NoError(t, err)

		of := astiav.FindOutputFormat("null")
		require.NotNil(t, of)
		require.NoError(t, m.Open(MuxerOpenOptions{
			Format:     of,
			FormatName: "n",
			URL:        "u",
		}))
		require.Len(t, ws.ws, 1)
		require.Equal(t, ws.ws[0], m.w)
		require.Equal(t, of, ws.ws[0].format)
		require.Equal(t, "n", ws.ws[0].formatName)
		require.Equal(t, "u", ws.ws[0].filename)
		require.Nil(t, ws.ws[0].pb)
		n, ok := classers.get(ws.ws[0])
		require.True(t, ok)
		require.Equal(t, m.n, n)

		of = astiav.FindOutputFormat("mp4")
		require.NotNil(t, of)
		require.Error(t, m.Open(MuxerOpenOptions{
			Format: of,
			URL:    t.TempDir() + "/invalid-dir/test",
		}))
		require.Len(t, ws.ws, 2)
		require.Equal(t, ws.ws[0], m.w)
		require.True(t, ws.ws[1].freed)

		path1 := filepath.Join(t.TempDir(), "iocontext-1")
		defer os.RemoveAll(path1)
		require.NoError(t, m.Open(MuxerOpenOptions{
			Format: of,
			URL:    path1,
		}))
		require.Len(t, ws.ws, 3)
		require.Equal(t, ws.ws[2], m.w)
		require.True(t, ws.ws[0].freed)
		require.NotNil(t, ws.ws[2].pb)
		n, ok = classers.get(ws.ws[2].pb)
		require.True(t, ok)
		require.Equal(t, m.n, n)

		path2 := filepath.Join(t.TempDir(), "iocontext-2")
		defer os.RemoveAll(path2)
		require.NoError(t, m.Open(MuxerOpenOptions{
			Format: of,
			URL:    path2,
		}))
		require.Len(t, ws.ws, 4)
		require.True(t, ws.ws[2].freed)
		_, ok = classers.get(ws.ws[2].pb)
		require.False(t, ok)
	})
}

func TestMuxerOnConnect(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		ws := newMockedMuxerWriters()
		defer ws.close()

		m, err := NewMuxer(MuxerOptions{Group: g})
		require.NoError(t, err)

		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		cp.SetCodecID(astiav.CodecIDMjpeg)
		n := mocks.NewMockedNoder()
		pd := PacketDescriptor{
			CodecParameters: cp,
			MediaDescriptor: MediaDescriptor{TimeBase: astiav.NewRational(1, 2)},
		}

		require.Error(t, m.OnConnect(pd, n))
		require.NoError(t, m.Open(MuxerOpenOptions{Format: astiav.FindOutputFormat("null")}))
		require.NoError(t, m.OnConnect(pd, n))
		require.Len(t, ws.ws, 1)
		require.Len(t, ws.ws[0].ss, 1)
		require.Equal(t, astiav.CodecIDMjpeg, ws.ws[0].ss[0].CodecParameters().CodecID())
		require.Equal(t, pd.MediaDescriptor.TimeBase, ws.ws[0].ss[0].TimeBase())
		require.Error(t, m.OnConnect(pd, n))
	})
}

func TestMuxerStart(t *testing.T) {
	// Header and trailer should be written, stats should be correct,
	// writer should write properly, invalid noder should be ignored,
	// pkt should be updated
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		ws := newMockedMuxerWriters()
		defer ws.close()
		type packet struct {
			pos         int64
			pts         int64
			streamIndex int
		}
		var ps []packet
		count := 0
		ws.writeInterleavedFrameFunc = func(p *astiav.Packet) error {
			count++
			ps = append(ps, packet{
				pos:         p.Pos(),
				pts:         p.Pts(),
				streamIndex: p.StreamIndex(),
			})
			if count == 4 {
				require.NoError(t, g.Stop())
			}
			return nil
		}

		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		cp.SetCodecID(astiav.CodecIDMjpeg)
		n1 := mocks.NewMockedNoder()
		pd1 := PacketDescriptor{
			CodecParameters: cp,
			MediaDescriptor: MediaDescriptor{TimeBase: astiav.NewRational(1, 1)},
		}
		n2 := mocks.NewMockedNoder()
		pd2 := PacketDescriptor{
			CodecParameters: cp,
			MediaDescriptor: MediaDescriptor{TimeBase: astiav.NewRational(2, 1)},
		}
		n3 := mocks.NewMockedNoder()
		pd3 := PacketDescriptor{
			CodecParameters: cp,
			MediaDescriptor: MediaDescriptor{TimeBase: astiav.NewRational(3, 1)},
		}

		m, err := NewMuxer(MuxerOptions{Group: g})
		require.NoError(t, err)
		dss := m.DeltaStats()

		require.NoError(t, m.Open(MuxerOpenOptions{Format: astiav.FindOutputFormat("null")}))
		require.NoError(t, m.OnConnect(pd1, n1))
		require.NoError(t, m.OnConnect(pd2, n2))
		require.Len(t, ws.ws, 1)
		require.Len(t, ws.ws[0].ss, 2)
		ws.ws[0].ss[1].SetTimeBase(astiav.NewRational(4, 1))

		pkt := astiav.AllocPacket()
		defer pkt.Free()
		require.NoError(t, pkt.AllocPayload(1))
		packetSize := 2
		pkt.SetSize(2)
		pkt.SetPos(1)
		pkt.SetPts(1)
		m.HandlePacket(Packet{
			Packet:           pkt,
			PacketDescriptor: pd1,
			Noder:            n1,
		})
		pkt.SetPos(2)
		pkt.SetPts(2)
		m.HandlePacket(Packet{
			Packet:           pkt,
			PacketDescriptor: pd2,
			Noder:            n2,
		})
		m.HandlePacket(Packet{
			Packet:           pkt,
			PacketDescriptor: pd3,
			Noder:            n3,
		})
		pkt.SetPos(3)
		pkt.SetPts(3)
		m.HandlePacket(Packet{
			Packet:           pkt,
			PacketDescriptor: pd1,
			Noder:            n1,
		})
		pkt.SetPos(4)
		pkt.SetPts(4)
		m.HandlePacket(Packet{
			Packet:           pkt,
			PacketDescriptor: pd2,
			Noder:            n2,
		})

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return m.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		cs := m.CumulativeStats()
		require.Greater(t, cs.WorkedDuration, time.Duration(0))
		cs.WorkedDuration = 0
		require.Equal(t, MuxerCumulativeStats{
			OutgoingBytes:   uint64(packetSize * 4),
			OutgoingPackets: 4,
			PacketHandlerCumulativeStats: PacketHandlerCumulativeStats{
				AllocatedCodecParameters: 7,
				AllocatedPackets:         5,
				IncomingBytes:            uint64(5 * packetSize),
				IncomingPackets:          5,
				ProcessedBytes:           uint64(5 * packetSize),
				ProcessedPackets:         5,
			},
		}, cs)
		requireDeltaStats(t, map[string]interface{}{
			DeltaStatNameAllocatedCodecParameters:   uint64(7),
			DeltaStatNameAllocatedPackets:           uint64(5),
			astiflow.DeltaStatNameIncomingByteRate:  float64(5 * packetSize),
			astiflow.DeltaStatNameIncomingRate:      float64(5),
			astiflow.DeltaStatNameOutgoingByteRate:  float64(packetSize * 4),
			astiflow.DeltaStatNameOutgoingRate:      float64(4),
			astiflow.DeltaStatNameProcessedByteRate: float64(5 * packetSize),
			astiflow.DeltaStatNameProcessedRate:     float64(5),
			astikit.StatNameWorkedRatio:             nil,
		}, dss)
		require.Equal(t, []packet{
			{
				pos:         1,
				pts:         1,
				streamIndex: 0,
			},
			{
				pos:         2,
				pts:         1,
				streamIndex: 1,
			},
			{
				pos:         3,
				pts:         3,
				streamIndex: 0,
			},
			{
				pos:         4,
				pts:         2,
				streamIndex: 1,
			},
		}, ps)
		require.True(t, ws.ws[0].header)
		require.True(t, ws.ws[0].trailer)
	})

	// Muxer is stopped if writing header fails
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		ws := newMockedMuxerWriters()
		defer ws.close()
		ws.writeHeader = func(d *astiav.Dictionary) error { return errors.New("test") }

		m, err := NewMuxer(MuxerOptions{Group: g})
		require.NoError(t, err)
		require.NoError(t, m.Open(MuxerOpenOptions{Format: astiav.FindOutputFormat("null")}))

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return m.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
	})
}
