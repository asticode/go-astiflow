package astiavflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

var _ astiav.IOInterrupter = (*mockedIOInterrupter)(nil)

type mockedIOInterrupter struct {
	cancel      context.CancelFunc
	ctx         context.Context
	interrupted bool
}

func newMockedIOInterrupter() *mockedIOInterrupter {
	ii := &mockedIOInterrupter{}
	ii.Resume()
	return ii
}

func (ii *mockedIOInterrupter) close() {
	ii.cancel()
}

func (ii *mockedIOInterrupter) Interrupt() {
	ii.interrupted = true
	ii.cancel()
}

func (ii *mockedIOInterrupter) Resume() {
	if ii.cancel != nil {
		ii.cancel()
	}
	ii.interrupted = false
	ii.ctx, ii.cancel = context.WithTimeout(context.Background(), time.Second)
}

var _ demuxerReader = (*mockedDemuxerReader)(nil)

type mockedDemuxerReader struct {
	findStreamInfoFunc       func() error
	flushed                  bool
	freed                    bool
	ii                       *mockedIOInterrupter
	inputClosed              bool
	openInputDictionary      *astiav.Dictionary
	openInputDictionaryValue string
	openInputFmt             *astiav.InputFormat
	openInputFunc            func() error
	openInputUrl             string
	pb                       *astiav.IOContext
	pbPath                   string
	previous                 func() demuxerReader
	readFrameFunc            func(p *astiav.Packet) error
	seekFrameStreamIndex     int
	seekFrameTimestamp       int64
	seekFrameFlags           astiav.SeekFlags
	startTimeFunc            func() int64
	streamInfoFound          bool
	streams                  []*astiav.Stream
}

func newMockedDemuxerReader(t *testing.T) *mockedDemuxerReader {
	r := &mockedDemuxerReader{
		pbPath:   filepath.Join(t.TempDir(), "iocontext.txt"),
		previous: newDemuxerReader,
	}
	newDemuxerReader = func() demuxerReader { return r }
	c, err := astiav.OpenIOContext(r.pbPath, astiav.NewIOContextFlags(astiav.IOContextFlagWrite))
	require.NoError(t, err)
	r.pb = c
	return r
}

func (r *mockedDemuxerReader) close() {
	r.pb.Closep() //nolint: errcheck
	os.Remove(r.pbPath)
	if r.ii != nil {
		r.ii.close()
	}
	newDemuxerReader = r.previous
}

func (r *mockedDemuxerReader) Class() *astiav.Class {
	return nil
}

func (r *mockedDemuxerReader) CloseInput() {
	r.inputClosed = true
}

func (r *mockedDemuxerReader) FindStreamInfo(d *astiav.Dictionary) error {
	if r.findStreamInfoFunc != nil {
		if err := r.findStreamInfoFunc(); err != nil {
			return err
		}
	}
	r.streamInfoFound = true
	return nil
}

func (r *mockedDemuxerReader) Flush() error {
	r.flushed = true
	return nil
}

func (r *mockedDemuxerReader) Free() {
	r.freed = true
}

func (r *mockedDemuxerReader) OpenInput(url string, fmt *astiav.InputFormat, d *astiav.Dictionary) error {
	if r.openInputFunc != nil {
		if err := r.openInputFunc(); err != nil {
			return err
		}
	}
	if d != nil {
		r.openInputDictionaryValue = d.Get("k", nil, astiav.NewDictionaryFlags()).Value()
	}
	r.openInputDictionary = d
	r.openInputFmt = fmt
	r.openInputUrl = url
	return nil
}

func (r *mockedDemuxerReader) Pb() *astiav.IOContext {
	return r.pb
}

func (r *mockedDemuxerReader) ReadFrame(p *astiav.Packet) error {
	return r.readFrameFunc(p)
}

func (r *mockedDemuxerReader) SeekFrame(streamIndex int, timestamp int64, f astiav.SeekFlags) error {
	r.seekFrameFlags = f
	r.seekFrameStreamIndex = streamIndex
	r.seekFrameTimestamp = timestamp
	return nil
}

func (r *mockedDemuxerReader) SetInterruptCallback() astiav.IOInterrupter {
	r.ii = newMockedIOInterrupter()
	return r.ii
}

func (r *mockedDemuxerReader) StartTime() int64 {
	return r.startTimeFunc()
}

func (r *mockedDemuxerReader) Streams() []*astiav.Stream {
	return r.streams
}

func TestDemuxerStartOptions(t *testing.T) {
	o := DemuxerStartOptions{}
	require.Equal(t, &demuxerEmulateRate{}, o.emulateRate())
	require.Equal(t, &demuxerLoop{}, o.loop())

	o = DemuxerStartOptions{
		EmulateRate: &DemuxerEmulateRateOptions{},
		Loop:        true,
	}
	require.Equal(t, &demuxerEmulateRate{enabled: true}, o.emulateRate())
	require.Equal(t, &demuxerLoop{enabled: 1}, o.loop())

	o = DemuxerStartOptions{EmulateRate: &DemuxerEmulateRateOptions{BufferDuration: time.Millisecond}}
	require.Equal(t, time.Millisecond, o.emulateRate().bufferDuration)

	o = DemuxerStartOptions{EmulateRate: &DemuxerEmulateRateOptions{BufferDuration: -time.Millisecond}}
	require.Equal(t, time.Duration(0), o.emulateRate().bufferDuration)
}

func TestNewDemuxer(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()
		countDemuxer = 0
		d, err := NewDemuxer(DemuxerOptions{Group: g})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{Name: "demuxer_1", Tags: []string{"demuxer"}}, d.n.Metadata())
		n, ok := classers.get(r)
		require.True(t, ok)
		require.Equal(t, d.n, n)
		var emitted bool
		d.On(astiflow.EventNameNodeClosed, func(payload interface{}) (delete bool) {
			emitted = true
			return
		})
		g.Close()
		require.True(t, r.freed)
		require.True(t, emitted)
		_, ok = classers.get(r)
		require.False(t, ok)
	})

	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		d, err := NewDemuxer(DemuxerOptions{
			Group:    g,
			Metadata: astiflow.Metadata{Description: "d", Name: "n", Tags: []string{"t"}},
		})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{
			Description: "d",
			Name:        "n",
			Tags:        []string{"demuxer", "t"},
		}, d.n.Metadata())
	})
}

func TestDemuxerPacketDispatcherSkippers(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		d, err := NewDemuxer(DemuxerOptions{Group: g})
		require.NoError(t, err)
		require.Len(t, d.pd.skippers, 0)

		h := newMockedPacketHandler()
		h.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h})
		require.NoError(t, err)
		d.Connect(h, StreamPacketSkipper(Stream{}))
		require.Len(t, d.pd.skippers, 1)

		d.Disconnect(h)
		require.Len(t, d.pd.skippers, 0)
	})
}

func TestDemuxerOpen(t *testing.T) {
	// No probe, no context error
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()
		fc := astiav.AllocFormatContext()
		defer fc.Free()
		s1 := fc.NewStream(nil)
		s1.SetAvgFrameRate(astiav.NewRational(1, 2))
		s1.SetID(1)
		s1.SetIndex(1)
		s1.SetTimeBase(astiav.NewRational(1, 3))
		s2 := fc.NewStream(nil)
		s2.SetID(2)
		s2.SetIndex(2)
		r.streams = []*astiav.Stream{s1, s2}
		mp4 := astiav.FindInputFormat("mp4")

		d, err := NewDemuxer(DemuxerOptions{Group: g})
		require.NoError(t, err)

		require.NoError(t, d.Open(context.Background(), DemuxerOpenOptions{
			Dictionary: NewCommaDictionaryOptions("k=v"),
			Format:     mp4,
			URL:        "url",
		}))
		require.Equal(t, r.ii, d.ii)
		require.Nil(t, r.openInputDictionary.Get("k", nil, astiav.NewDictionaryFlags()))
		require.Equal(t, "v", r.openInputDictionaryValue)
		require.Equal(t, mp4, r.openInputFmt)
		require.Equal(t, "url", r.openInputUrl)
		require.False(t, r.inputClosed)
		pb := r.Pb()
		n, ok := classers.get(r.Pb())
		require.True(t, ok)
		require.Equal(t, d.n, n)
		require.True(t, r.streamInfoFound)
		streamIndexes := []int{}
		for _, s := range d.Streams() {
			streamIndexes = append(streamIndexes, s.Index)
		}
		sort.Ints(streamIndexes)
		require.Equal(t, []int{1, 2}, streamIndexes)
		require.Eventually(t, func() bool { return !r.ii.interrupted }, time.Second, 10*time.Millisecond)
		require.Equal(t, []Stream{
			{
				CodecParameters: s1.CodecParameters(),
				ID:              1,
				Index:           1,
				MediaContext: MediaContext{
					FrameRate: astiav.NewRational(1, 2),
					TimeBase:  astiav.NewRational(1, 3),
				},
			},
			{
				CodecParameters: s2.CodecParameters(),
				ID:              2,
				Index:           2,
			},
		}, d.Streams())

		d.c.Close()
		_, ok = classers.get(pb)
		require.False(t, ok)
		require.True(t, r.inputClosed)
	})

	// Context errors
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		r.openInputFunc = func() error {
			<-r.ii.ctx.Done()
			return r.ii.ctx.Err()
		}

		d, err := NewDemuxer(DemuxerOptions{Group: g})
		require.NoError(t, err)

		require.ErrorIs(t, d.Open(ctx, DemuxerOpenOptions{URL: "url"}), context.Canceled)
		require.Eventually(t, func() bool { return r.ii.interrupted }, time.Second, 10*time.Millisecond)
		require.Empty(t, r.openInputUrl)
	})
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r.openInputFunc = func() error {
			cancel()
			return nil
		}

		d, err := NewDemuxer(DemuxerOptions{Group: g})
		require.NoError(t, err)

		require.ErrorIs(t, d.Open(ctx, DemuxerOpenOptions{URL: "url"}), context.Canceled)
		require.Eventually(t, func() bool { return r.ii.interrupted }, time.Second, 10*time.Millisecond)
		require.NotEmpty(t, r.openInputUrl)
		require.False(t, r.streamInfoFound)
	})
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r.findStreamInfoFunc = func() error {
			cancel()
			return nil
		}

		d, err := NewDemuxer(DemuxerOptions{Group: g})
		require.NoError(t, err)

		require.ErrorIs(t, d.Open(ctx, DemuxerOpenOptions{URL: "url"}), context.Canceled)
		require.Eventually(t, func() bool { return r.ii.interrupted }, time.Second, 10*time.Millisecond)
		require.NotEmpty(t, r.openInputUrl)
		require.True(t, r.streamInfoFound)
	})
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()

		d, err := NewDemuxer(DemuxerOptions{Group: g})
		require.NoError(t, err)

		require.NoError(t, d.Open(context.Background(), DemuxerOpenOptions{URL: "url"}))
		require.Eventually(t, func() bool { return !r.ii.interrupted }, time.Second, 10*time.Millisecond)
	})

	// Probe
	// No need for probe
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()

		d, err := NewDemuxer(DemuxerOptions{Group: g})
		require.NoError(t, err)

		require.NoError(t, d.Open(context.Background(), DemuxerOpenOptions{}))
		require.Nil(t, d.Probe())
	})

	// Probe
	// Probe is needed + not enough information error
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()
		r.readFrameFunc = func(p *astiav.Packet) error { return astiav.ErrEof }

		d1, err := NewDemuxer(DemuxerOptions{Group: g, Start: DemuxerStartOptions{Loop: true}})
		require.NoError(t, err)
		require.Error(t, d1.Open(context.Background(), DemuxerOpenOptions{}))

		d2, err := NewDemuxer(DemuxerOptions{Group: g, Start: DemuxerStartOptions{EmulateRate: &DemuxerEmulateRateOptions{}}})
		require.NoError(t, err)
		require.Error(t, d2.Open(context.Background(), DemuxerOpenOptions{}))
	})

	// Probe
	// Probed packets are stored, only valid PTS values are processed, probe is canceled once buffer duration is reached,
	// probe info is valid, emulate rate reference timestamps are updated, probe duration has a default value,
	// first pts can be in later packets
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()
		fc := astiav.AllocFormatContext()
		defer fc.Free()
		s1 := fc.NewStream(nil)
		s1.SetIndex(1)
		s1.SetTimeBase(astiav.NewRational(1, 10))
		s2 := fc.NewStream(nil)
		s2.SetIndex(2)
		s2.SetTimeBase(astiav.NewRational(1, 20))
		count := 0
		r.readFrameFunc = func(p *astiav.Packet) error {
			count++
			switch count {
			case 1:
				p.SetStreamIndex(1)
			case 2:
				p.SetStreamIndex(2)
				p.SetPts(-1)
			case 3:
				p.SetStreamIndex(2)
				p.SetPts(15)
			case 4:
				p.SetStreamIndex(1)
				p.SetPts(6)
			case 5:
				p.SetStreamIndex(1)
				p.SetPts(5)
			case 6:
				p.SetStreamIndex(3)
				p.SetPts(0)
			case 7:
				p.SetStreamIndex(2)
				p.SetPts(36)
			default:
				count--
				return errors.New("invalid")
			}
			return nil
		}
		r.streams = []*astiav.Stream{s1, s2}

		d, err := NewDemuxer(DemuxerOptions{Group: g, Start: DemuxerStartOptions{Loop: true}})
		require.NoError(t, err)

		require.NoError(t, d.Open(context.Background(), DemuxerOpenOptions{}))
		require.Len(t, d.pb.data, count)
		require.Equal(t, int64(36), d.pb.data[len(d.pb.data)-1].Pts())
		pb := d.Probe()
		require.NotNil(t, pb)
		require.True(t, pb.FirstPTS.IsStream(Stream{Index: 1}))
		require.False(t, pb.FirstPTS.IsStream(Stream{Index: 2}))
		require.Equal(t, NanosecondRational, pb.FirstPTS.Timebase)
		require.Equal(t, int64(5e8), pb.FirstPTS.Value)
	})

	// Probe
	// Developer can provide a buffer duration, skipped start is handled
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()
		fc := astiav.AllocFormatContext()
		defer fc.Free()
		s := fc.NewStream(nil)
		s.SetIndex(1)
		s.SetTimeBase(astiav.NewRational(1, 10))
		s.CodecParameters().SetMediaType(astiav.MediaTypeAudio)
		s.CodecParameters().SetSampleRate(200)
		count := 0
		r.readFrameFunc = func(p *astiav.Packet) error {
			count++
			switch count {
			case 1:
				p.SetStreamIndex(1)
				require.NoError(t, p.AddSideData(astiav.PacketSideDataTypeSkipSamples, []byte{100, 0, 0, 0, 0, 0, 0, 0, 0, 0}))
				p.SetPts(0)
			case 2:
				p.SetStreamIndex(1)
				p.SetPts(15)
			case 3:
				p.SetStreamIndex(1)
				p.SetPts(26)
			default:
				count--
				return errors.New("invalid")
			}
			return nil
		}
		r.streams = []*astiav.Stream{s}

		d, err := NewDemuxer(DemuxerOptions{Group: g, Start: DemuxerStartOptions{Loop: true}})
		require.NoError(t, err)

		require.NoError(t, d.Open(context.Background(), DemuxerOpenOptions{ProbeDuration: 2 * time.Second}))
		require.Len(t, d.pb.data, count)
		pb := d.Probe()
		require.NotNil(t, pb)
		require.True(t, pb.FirstPTS.IsStream(Stream{Index: 1}))
		require.Equal(t, int64(5e8), pb.FirstPTS.Value)
	})

	// Probe
	// No error if eof but enough information
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()
		fc := astiav.AllocFormatContext()
		defer fc.Free()
		s := fc.NewStream(nil)
		s.SetIndex(1)
		s.SetTimeBase(astiav.NewRational(1, 10))
		count := 0
		r.readFrameFunc = func(p *astiav.Packet) error {
			count++
			switch count {
			case 1:
				p.SetStreamIndex(1)
				p.SetPts(5)
			default:
				count--
				return astiav.ErrEof
			}
			return nil
		}
		r.streams = []*astiav.Stream{s}

		d, err := NewDemuxer(DemuxerOptions{Group: g, Start: DemuxerStartOptions{Loop: true}})
		require.NoError(t, err)

		require.NoError(t, d.Open(context.Background(), DemuxerOpenOptions{ProbeDuration: 2 * time.Second}))
		require.Len(t, d.pb.data, count)
		pb := d.Probe()
		require.NotNil(t, pb)
		require.True(t, pb.FirstPTS.IsStream(Stream{Index: 1}))
		require.Equal(t, int64(5e8), pb.FirstPTS.Value)
	})
}

func TestDemuxerProcessPacketSideData(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		d, err := NewDemuxer(DemuxerOptions{Group: g})
		require.NoError(t, err)

		pkt := astiav.AllocPacket()
		defer pkt.Free()
		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		cp.SetMediaType(astiav.MediaTypeAudio)
		cp.SetSampleRate(44100)
		ds := &demuxerStream{s: Stream{CodecParameters: cp}}

		s, e := d.processPacketSideData(pkt, ds)
		require.Equal(t, time.Duration(0), s)
		require.Equal(t, time.Duration(0), e)

		require.NoError(t, pkt.AddSideData(astiav.PacketSideDataTypeSkipSamples, []byte{1, 2, 3, 4, 5, 6, 7, 8, 0, 0}))
		s, e = d.processPacketSideData(pkt, ds)
		require.Equal(t, time.Duration(float64(0x4030201)/float64(cp.SampleRate())*float64(1e9)), s)
		require.Equal(t, time.Duration(float64(0x8070605)/float64(cp.SampleRate())*float64(1e9)), e)
	})
}

func TestDemuxerLoop(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		d, err := NewDemuxer(DemuxerOptions{Group: g, Start: DemuxerStartOptions{Loop: true}})
		require.NoError(t, err)
		require.True(t, d.l.enabled > 0)
		d.Loop(false)
		require.True(t, d.l.enabled == 0)
		d.Loop(true)
		require.True(t, d.l.enabled > 0)
	})
}

func TestDemuxerProbe(t *testing.T) {
	var count int64
	defer astikit.MockNow(func() time.Time {
		count++
		return time.Unix(count, 0)
	}).Close()

	p := DemuxerProbe{FirstPTS: DemuxerProbeFirstPTS{
		streams:  map[int]bool{1: true},
		Timebase: astiav.NewRational(1, 1),
		Value:    1,
	}}
	require.True(t, p.FirstPTS.IsStream(Stream{Index: 1}))
	require.False(t, p.FirstPTS.IsStream(Stream{Index: 2}))
	r := p.TimeReference()
	require.Equal(t, int64(4), r.TimestampFromTime(time.Unix(2, 0), astiav.NewRational(1, 2)))
}

func TestDemuxerStart(t *testing.T) {
	// No loop, no emulate rate, stopped by custom read error
	// Loop cycle info should not be empty even though loop is not enabled, stats
	// should be correct, custom read error should be handled
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()
		fc := astiav.AllocFormatContext()
		defer fc.Free()
		s1 := fc.NewStream(nil)
		s1.SetIndex(1)
		s1.SetTimeBase(astiav.NewRational(1, 10))
		s2 := fc.NewStream(nil)
		s2.SetIndex(2)
		s2.SetTimeBase(astiav.NewRational(1, 20))
		r.streams = []*astiav.Stream{s1, s2}

		countReadFrameError := 0
		d, err := NewDemuxer(DemuxerOptions{
			Group: g,
			Start: DemuxerStartOptions{OnReadFrameError: func(d *Demuxer, err error) (stop bool, handled bool) {
				countReadFrameError++
				switch countReadFrameError {
				case 1:
					handled = true
				case 2:
					stop = true
				}
				return
			}},
		})
		require.NoError(t, err)

		dss := d.DeltaStats()
		require.Len(t, dss, 5)

		type packet struct {
			cp  *astiav.CodecParameters
			mc  MediaContext
			n   astiflow.Noder
			pts int64
		}
		ps1 := []packet{}
		h1 := newMockedPacketHandler()
		h1.handlePacketFunc = func(p Packet) {
			ps1 = append(ps1, packet{
				cp:  p.CodecParameters,
				mc:  p.MediaContext,
				n:   p.Noder,
				pts: p.Pts(),
			})
		}
		h1.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h1})
		require.NoError(t, err)
		ps2 := []packet{}
		h2 := newMockedPacketHandler()
		h2.handlePacketFunc = func(p Packet) {
			ps2 = append(ps2, packet{
				cp:  p.CodecParameters,
				mc:  p.MediaContext,
				n:   p.Noder,
				pts: p.Pts(),
			})
		}
		h2.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h2})
		require.NoError(t, err)

		count := 0
		pktSize := 2
		r.readFrameFunc = func(p *astiav.Packet) error {
			count++
			p.SetDuration(10)
			p.SetDts(int64(count * 10))
			p.SetPts(int64(count * 10))
			p.SetSize(pktSize)
			switch count {
			case 1:
				p.SetStreamIndex(1)
			case 2:
				p.SetStreamIndex(2)
			case 3:
				p.SetStreamIndex(3)
			case 4:
				p.SetStreamIndex(1)
			case 5:
				p.SetStreamIndex(2)
			case 6:
				d.Disconnect(h2)
				p.SetStreamIndex(1)
			case 7:
				p.SetStreamIndex(2)
			default:
				return astiav.ErrEof
			}
			return nil
		}

		require.NoError(t, d.Open(context.Background(), DemuxerOpenOptions{}))
		ss := d.Streams()
		require.Len(t, ss, 2)

		d.Connect(h1, nil)
		d.Connect(h2, StreamPacketSkipper(ss[0]))

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return d.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Equal(t, DemuxerCumulativeStats{
			AllocatedPackets: 1,
			IncomingBytes:    uint64(pktSize * 7),
			IncomingPackets:  uint64(7),
			OutgoingBytes:    uint64(pktSize * 6),
			OutgoingPackets:  uint64(6),
		}, d.CumulativeStats())
		require.Equal(t, float64(6), dss[0].Valuer.Value(time.Second))
		require.Equal(t, float64(pktSize*6), dss[1].Valuer.Value(time.Second))
		require.Equal(t, uint64(1), dss[2].Valuer.Value(time.Second))
		require.Equal(t, float64(pktSize*7), dss[3].Valuer.Value(time.Second))
		require.Equal(t, float64(7), dss[4].Valuer.Value(time.Second))
		require.Equal(t, []packet{
			{
				cp:  d.ss[1].s.CodecParameters,
				mc:  d.ss[1].s.MediaContext,
				n:   d,
				pts: 10,
			},
			{
				cp:  d.ss[2].s.CodecParameters,
				mc:  d.ss[2].s.MediaContext,
				n:   d,
				pts: 20,
			},
			{
				cp:  d.ss[1].s.CodecParameters,
				mc:  d.ss[1].s.MediaContext,
				n:   d,
				pts: 40,
			},
			{
				cp:  d.ss[2].s.CodecParameters,
				mc:  d.ss[2].s.MediaContext,
				n:   d,
				pts: 50,
			},
			{
				cp:  d.ss[1].s.CodecParameters,
				mc:  d.ss[1].s.MediaContext,
				n:   d,
				pts: 60,
			},
			{
				cp:  d.ss[2].s.CodecParameters,
				mc:  d.ss[2].s.MediaContext,
				n:   d,
				pts: 70,
			},
		}, ps1)
		require.Equal(t, []packet{
			{
				cp:  d.ss[1].s.CodecParameters,
				mc:  d.ss[1].s.MediaContext,
				n:   d,
				pts: 10,
			},
			{
				cp:  d.ss[1].s.CodecParameters,
				mc:  d.ss[1].s.MediaContext,
				n:   d,
				pts: 40,
			},
		}, ps2)
		require.False(t, r.flushed)
		for _, s := range d.ss {
			require.NotNil(t, s.l.cycleFirstPacketPTS)
			require.Greater(t, s.l.cycleLastPacketDuration, time.Duration(0))
			require.Greater(t, s.l.cycleLastPacketPTS, int64(0))
		}
		require.Equal(t, 2, countReadFrameError)
	})

	// Loop, no emulate rate, stopped by context
	// IO interrupter should be interrupted, loop cycle info should be correct, demuxer should be flushed,
	// skipped durations should be handled
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()
		fc := astiav.AllocFormatContext()
		defer fc.Free()
		s1 := fc.NewStream(nil)
		s1.SetIndex(1)
		s1.SetTimeBase(astiav.NewRational(1, 10))
		s1.CodecParameters().SetMediaType(astiav.MediaTypeAudio)
		s1.CodecParameters().SetSampleRate(20)
		s2 := fc.NewStream(nil)
		s2.SetIndex(2)
		s2.SetTimeBase(astiav.NewRational(1, 20))
		r.startTimeFunc = func() int64 { return 0 }
		r.streams = []*astiav.Stream{s1, s2}

		d, err := NewDemuxer(DemuxerOptions{Group: g, Start: DemuxerStartOptions{
			Flush: true,
			Loop:  true,
		}})
		require.NoError(t, err)

		count := 0
		r.readFrameFunc = func(p *astiav.Packet) error {
			count++
			switch count {
			case 1, 8, 15:
				require.NoError(t, p.AddSideData(astiav.PacketSideDataTypeSkipSamples, []byte{3, 0, 0, 0, 0, 0, 0, 0, 0, 0}))
				p.SetDts(2)
				p.SetDuration(2)
				p.SetPts(2)
				p.SetStreamIndex(1)
			case 2, 9, 16:
				p.SetDts(8)
				p.SetDuration(2)
				p.SetPts(8)
				p.SetStreamIndex(2)
			case 3, 10, 17:
				p.SetDts(5)
				p.SetDuration(1)
				p.SetPts(5)
				p.SetStreamIndex(1)
			case 4, 11, 18:
				p.SetDts(10)
				p.SetDuration(2)
				p.SetPts(10)
				p.SetStreamIndex(2)
			case 5, 12, 19:
				require.NoError(t, p.AddSideData(astiav.PacketSideDataTypeSkipSamples, []byte{0, 0, 0, 0, 2, 0, 0, 0, 0, 0}))
				p.SetDts(15)
				p.SetDuration(2)
				p.SetPts(15)
				p.SetStreamIndex(1)
			case 6, 13, 20:
				if d.l.cycleCount > 1 {
					w.Stop()
				}
				p.SetDts(30)
				p.SetDuration(2)
				p.SetPts(30)
				p.SetStreamIndex(2)
			case 7, 14:
				return astiav.ErrEof
			}
			return nil
		}

		ps := make(map[int][]int64)
		h := newMockedPacketHandler()
		h.handlePacketFunc = func(p Packet) { ps[p.StreamIndex()] = append(ps[p.StreamIndex()], p.Pts()) }
		h.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h})
		require.NoError(t, err)

		require.NoError(t, d.Open(context.Background(), DemuxerOpenOptions{}))
		require.Len(t, d.ss, 2)

		d.Connect(h, nil)

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return d.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Equal(t, 1, r.seekFrameStreamIndex)
		require.Equal(t, int64(3), r.seekFrameTimestamp)
		require.Equal(t, astiav.NewSeekFlags(astiav.SeekFlagBackward), r.seekFrameFlags)
		require.Equal(t, uint(2), d.l.cycleCount)
		require.Equal(t, time.Second+250*time.Millisecond, d.l.cycleDuration)
		require.Equal(t, demuxerStreamLoop{
			cycleFirstPacketPTS:          astikit.Int64Ptr(3),
			cycleFirstPacketPTSRemainder: 50 * time.Millisecond,
			cycleLastPacketDuration:      100 * time.Millisecond,
			cycleLastPacketPTS:           15,
			restampRemainder:             50 * time.Millisecond,
		}, *d.ss[1].l)
		require.Equal(t, demuxerStreamLoop{
			cycleFirstPacketPTS:          astikit.Int64Ptr(8),
			cycleFirstPacketPTSRemainder: 0,
			cycleLastPacketDuration:      100 * time.Millisecond,
			cycleLastPacketPTS:           30,
		}, *d.ss[2].l)
		require.Equal(t, map[int][]int64{
			1: {3, 5, 15, 16, 17, 28, 28, 30, 40},
			2: {8, 10, 30, 33, 35, 55, 58, 60, 80},
		}, ps)
		require.True(t, r.flushed)
		require.Eventually(t, func() bool { return r.ii.interrupted }, time.Second, 10*time.Millisecond)
	})

	// No loop, emulate rate, stopped by eof
	// demuxer should sleep properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		defer astikit.MockNow(func() time.Time { return time.Unix(2, 0) }).Close()
		var lastSleep time.Duration
		defer astikit.MockSleep(func(ctx context.Context, d time.Duration) {
			lastSleep = d
		}).Close()

		r := newMockedDemuxerReader(t)
		defer r.close()
		fc := astiav.AllocFormatContext()
		defer fc.Free()
		s := fc.NewStream(nil)
		s.SetIndex(1)
		s.SetTimeBase(astiav.NewRational(1, 2))
		countReadFrame := 0
		r.readFrameFunc = func(p *astiav.Packet) error {
			countReadFrame++
			p.SetStreamIndex(1)
			switch countReadFrame {
			case 1:
				p.SetDts(0)
				p.SetPts(0)
			case 2:
				p.SetDts(2)
				p.SetPts(2)
			case 3:
				p.SetDts(4)
				p.SetPts(4)
			case 4:
				p.SetDts(5)
				p.SetPts(5)
			case 5:
				p.SetDts(7)
				p.SetPts(7)
			case 6:
				return astiav.ErrEof
			}
			return nil
		}
		r.streams = []*astiav.Stream{s}

		d, err := NewDemuxer(DemuxerOptions{Group: g, Start: DemuxerStartOptions{EmulateRate: &DemuxerEmulateRateOptions{
			BufferDuration: 2 * time.Second,
		}}})
		require.NoError(t, err)

		type packet struct {
			pts   int64
			sleep time.Duration
		}
		var ps []packet
		h := newMockedPacketHandler()
		h.handlePacketFunc = func(p Packet) {
			ps = append(ps, packet{
				pts:   p.Pts(),
				sleep: lastSleep,
			})
			lastSleep = time.Duration(0)
		}
		h.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h})
		require.NoError(t, err)

		require.NoError(t, d.Open(context.Background(), DemuxerOpenOptions{}))

		d.Connect(h, nil)

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return d.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Equal(t, []packet{
			{
				pts:   0,
				sleep: time.Duration(0),
			},
			{
				pts:   2,
				sleep: time.Duration(0),
			},
			{
				pts:   4,
				sleep: time.Duration(0),
			},
			{
				pts:   5,
				sleep: 500 * time.Millisecond,
			},
			{
				pts:   7,
				sleep: time.Second + 500*time.Millisecond,
			},
		}, ps)
	})
}

func TestDemuxerPause(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		now := time.Unix(2, 0)
		defer astikit.MockNow(func() time.Time { return now }).Close()
		var lastSleep time.Duration
		defer astikit.MockSleep(func(ctx context.Context, d time.Duration) {
			lastSleep = d
		}).Close()

		r := newMockedDemuxerReader(t)
		defer r.close()
		fc := astiav.AllocFormatContext()
		defer fc.Free()
		s := fc.NewStream(nil)
		s.SetIndex(1)
		s.SetTimeBase(astiav.NewRational(1, 2))
		r.streams = []*astiav.Stream{s}

		d, err := NewDemuxer(DemuxerOptions{Group: g, Start: DemuxerStartOptions{EmulateRate: &DemuxerEmulateRateOptions{}}})
		require.NoError(t, err)

		count := 0
		r.readFrameFunc = func(p *astiav.Packet) error {
			count++
			p.SetStreamIndex(1)
			switch count {
			case 1:
				p.SetDts(0)
				p.SetPts(0)
			case 2:
				p.SetDts(2)
				p.SetPts(2)
			case 3:
				now = time.Unix(3, 0)
				d.Pause()
				p.SetDts(4)
				p.SetPts(4)
			case 4:
				require.False(t, d.Paused())
				p.SetDts(6)
				p.SetPts(6)
			case 5:
				return astiav.ErrEof
			}
			return nil
		}

		type packet struct {
			pts   int64
			sleep time.Duration
		}
		var ps []packet
		h := newMockedPacketHandler()
		h.handlePacketFunc = func(p Packet) {
			ps = append(ps, packet{
				pts:   p.Pts(),
				sleep: lastSleep,
			})
			lastSleep = time.Duration(0)
		}
		h.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h})
		require.NoError(t, err)

		require.NoError(t, d.Open(context.Background(), DemuxerOpenOptions{ProbeDuration: 500 * time.Millisecond}))

		d.Connect(h, nil)

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return d.Paused() }, time.Second, 10*time.Millisecond)
		require.Equal(t, []packet{
			{pts: 0},
			{pts: 2, sleep: time.Second},
			{pts: 4, sleep: time.Second},
		}, ps)
		ps = []packet{}
		now = time.Unix(6, 0)
		d.Resume()

		require.Eventually(t, func() bool { return d.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Equal(t, []packet{
			{
				pts:   6,
				sleep: 2 * time.Second,
			},
		}, ps)
	})

	// Pause is canceled on context cancel
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		r := newMockedDemuxerReader(t)
		defer r.close()
		fc := astiav.AllocFormatContext()
		defer fc.Free()
		s := fc.NewStream(nil)
		s.SetIndex(1)
		s.SetTimeBase(astiav.NewRational(1, 2))

		d, err := NewDemuxer(DemuxerOptions{Group: g})
		require.NoError(t, err)

		count := 0
		r.readFrameFunc = func(p *astiav.Packet) error {
			count++
			p.SetStreamIndex(1)
			switch count {
			case 1:
				d.Pause()
			}
			return nil
		}

		require.NoError(t, d.Open(context.Background(), DemuxerOpenOptions{}))

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return d.Paused() }, time.Second, 10*time.Millisecond)
		w.Stop()
		require.Eventually(t, func() bool { return f.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
	})
}
