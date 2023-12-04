package astiavflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

var _ PacketHandler = (*Muxer)(nil)

var (
	countMuxer uint64
)

type Muxer struct {
	*packetHandler
	c  *astikit.Closer
	cs *muxerCumulativeStats
	ms sync.Mutex // Locks ss
	n  *astiflow.Node
	ss map[astiflow.Noder]*astiav.Stream
	w  muxerWriter
}

type MuxerOptions struct {
	Group    *astiflow.Group
	Metadata astiflow.Metadata
	Stop     *astiflow.NodeStopOptions
}

func NewMuxer(o MuxerOptions) (m *Muxer, err error) {
	// Create muxer
	m = &Muxer{
		cs:            &muxerCumulativeStats{},
		packetHandler: newPacketHandler(),
		ss:            make(map[astiflow.Noder]*astiav.Stream),
	}

	// Create node
	if m.n, m.c, err = o.Group.NewNode(astiflow.NodeOptions{
		Metadata: (&astiflow.Metadata{
			Name: fmt.Sprintf("muxer_%d", atomic.AddUint64(&countMuxer, uint64(1))),
			Tags: []string{"muxer"},
		}).Merge(o.Metadata),
		Noder: m,
		Stop:  o.Stop,
	}); err != nil {
		err = fmt.Errorf("astiavflow: creating node failed: %w", err)
		return
	}

	// Initialize handler
	m.packetHandler.init(packetHandlerInitOptions{
		c:         m.c,
		n:         m.n,
		onConnect: m.onConnect,
		onPacket:  m.onPacket,
	})

	// Make sure to close writer
	m.c.Add(m.closeWriter)
	return
}

type MuxerOpenOptions struct {
	Format     *astiav.OutputFormat
	FormatName string
	URL        string
}

func (m *Muxer) Open(o MuxerOpenOptions) (err error) {
	// Create writer
	var w muxerWriter
	if w, err = newMuxerWriter(o.Format, o.FormatName, o.URL); err != nil {
		err = fmt.Errorf("astiavflow: creating writer failed: %w", err)
		return
	}

	// Make sure to close writer in case of error
	defer func() {
		if err != nil {
			w.Free()
		}
	}()

	// We need to create an io context if this is a file
	if !w.OutputFormat().Flags().Has(astiav.IOFormatFlagNofile) {
		// Open io context
		var ioContext *astiav.IOContext
		if ioContext, err = astiav.OpenIOContext(o.URL, astiav.NewIOContextFlags(astiav.IOContextFlagWrite)); err != nil {
			err = fmt.Errorf("astiavflow: opening io context failed: %w", err)
			return
		}

		// Update writer
		w.SetPb(ioContext)
	}

	// Make sure to close previous writer
	m.closeWriter()

	// Store writer
	m.w = w
	if pb := w.Pb(); pb != nil {
		classers.set(pb, m.n)
	}
	classers.set(w, m.n)
	return
}

func (m *Muxer) closeWriter() {
	if m.w != nil {
		if pb := m.w.Pb(); pb != nil {
			classers.del(pb)
			pb.Close() //nolint: errcheck
		}
		classers.del(m.w)
		m.w.Free()
	}
}

type MuxerCumulativeStats struct {
	OutgoingBytes   uint64
	OutgoingPackets uint64
	PacketHandlerCumulativeStats
}

func (m *Muxer) CumulativeStats() MuxerCumulativeStats {
	return MuxerCumulativeStats{
		OutgoingBytes:                atomic.LoadUint64(&m.cs.outgoingBytes),
		OutgoingPackets:              atomic.LoadUint64(&m.cs.outgoingPackets),
		PacketHandlerCumulativeStats: m.packetHandler.cumulativeStats(),
	}
}

type muxerCumulativeStats struct {
	outgoingBytes   uint64
	outgoingPackets uint64
}

func (m *Muxer) DeltaStats() []astikit.DeltaStat {
	ss := m.packetHandler.deltaStats()
	ss = append(ss,
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of bytes going out per second",
				Label:       "Outgoing byte rate",
				Name:        astiflow.DeltaStatNameOutgoingByteRate,
				Unit:        "Bps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&m.cs.outgoingBytes),
		},
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of packets going out per second",
				Label:       "Outgoing rate",
				Name:        astiflow.DeltaStatNameOutgoingRate,
				Unit:        "pps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&m.cs.outgoingPackets),
		},
	)
	return ss
}

func (m *Muxer) On(n astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return m.n.On(n, h)
}

func (m *Muxer) onConnect(n astiflow.Noder) error {
	// Lock
	m.ms.Lock()
	defer m.ms.Unlock()

	// Get initial incoming packet descriptor
	pd, ok := m.initialIncomingPacketDescriptors.get(n)
	if !ok {
		return errors.New("astiavflow: no initial incoming packet descriptor for noder")
	}

	// Open has not been called
	if m.w == nil {
		return errors.New("astiavflow: .Open() needs to be called first")
	}

	// Noder is already connected
	if _, ok := m.ss[n]; ok {
		return errors.New("astiavflow: noder is already connected")
	}

	// Create stream
	s := m.w.NewStream(nil)

	// Copy codec parameters
	if err := pd.CodecParameters.Copy(s.CodecParameters()); err != nil {
		return fmt.Errorf("astiavflow: copying codec parameters failed: %w", err)
	}

	// Needed at least by remuxing when dealing with an output format with no default time base
	// However this may be overwritten if output format has a default time base
	s.SetTimeBase(pd.MediaDescriptor.TimeBase)

	// Store stream
	m.ss[n] = s
	return nil
}

func (m *Muxer) Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	tc().Do(func() {
		// Write header
		if err := m.w.WriteHeader(nil); err != nil {
			dispatchError(m.n, fmt.Errorf("astiavflow: writing header failed: %w", err).Error())
			cancel()
			return
		}

		// Make sure to write trailer
		defer func() {
			if err := m.w.WriteTrailer(); err != nil {
				dispatchError(m.n, fmt.Errorf("astiavflow: writing trailer failed: %w", err).Error())
				return
			}
		}()

		// Start packet handler
		m.packetHandler.start(ctx)
	})
}

func (m *Muxer) onPacket(n astiflow.Noder, p *astiav.Packet, pd PacketDescriptor) {
	// Get stream
	m.ms.Lock()
	s, ok := m.ss[n]
	if !ok {
		dispatchError(m.n, "astiavflow: no stream found for noder %s", n)
		m.ms.Unlock()
		return
	}
	m.ms.Unlock()

	// Update pkt
	// In case output format has a default time base (e.g. mpegts) and therefore stream's time base has
	// been overwritten, we need to make sure packet timestamps are rescaled
	p.RescaleTs(pd.MediaDescriptor.TimeBase, s.TimeBase())
	p.SetStreamIndex(s.Index())

	// TODO Use OnPacket in options instead of Restamper

	// Write
	pktSize := p.Size()
	if err := m.w.WriteInterleavedFrame(p); err != nil {
		dispatchError(m.n, fmt.Errorf("astiavflow: writing interleaved frame failed: %w", err).Error())
		return
	}

	// Increment stats
	atomic.AddUint64(&m.cs.outgoingBytes, uint64(pktSize))
	atomic.AddUint64(&m.cs.outgoingPackets, 1)
}

type muxerWriter interface {
	Class() *astiav.Class
	Free()
	NewStream(c *astiav.Codec) *astiav.Stream
	OutputFormat() *astiav.OutputFormat
	Pb() *astiav.IOContext
	SetPb(i *astiav.IOContext)
	WriteHeader(d *astiav.Dictionary) error
	WriteInterleavedFrame(p *astiav.Packet) error
	WriteTrailer() error
}

var newMuxerWriter = func(format *astiav.OutputFormat, formatName, filename string) (muxerWriter, error) {
	return astiav.AllocOutputFormatContext(format, formatName, filename)
}
