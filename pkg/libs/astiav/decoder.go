package astiavflow

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

var (
	countDecoder uint64
)

type Decoder struct {
	c                    *astikit.Closer
	ch                   *astikit.Chan
	cumulativeStats      *decoderCumulativeStats
	enforceMonotonicDTS  bool
	n                    *astiflow.Node
	fd                   *frameDispatcher
	fp                   *framePool
	pp                   *packetPool
	previousDts          *int64
	previousMediaContext MediaContext
	r                    decoderReader
	ro                   DecoderReaderOptions
}

type DecoderOptions struct {
	EnforceMonotonicDTS bool
	Group               *astiflow.Group
	Metadata            astiflow.Metadata
	Reader              DecoderReaderOptions
	Stop                *astiflow.NodeStopOptions
}

type DecoderReaderOptions func(p Packet) (decoderName string)

func NewDecoder(o DecoderOptions) (d *Decoder, err error) {
	// Create decoder
	d = &Decoder{
		ch:                  astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		cumulativeStats:     &decoderCumulativeStats{},
		enforceMonotonicDTS: o.EnforceMonotonicDTS,
		fd:                  newFrameDispatcher(),
		fp:                  newFramePool(),
		pp:                  newPacketPool(),
		ro:                  o.Reader,
	}

	// Create node
	if d.n, d.c, err = o.Group.NewNode(astiflow.NodeOptions{
		Metadata: (&astiflow.Metadata{
			Name: fmt.Sprintf("decoder_%d", atomic.AddUint64(&countDecoder, uint64(1))),
			Tags: []string{"decoder"},
		}).Merge(o.Metadata),
		Noder: d,
		Stop:  o.Stop,
	}); err != nil {
		err = fmt.Errorf("astiavflow: creating node failed: %w", err)
		return
	}

	// Initialize dispatchers and pools
	d.fd.init(d.n)
	d.fp.init(d.c)
	d.pp.init(d.c)

	// Make sure to close reader
	d.c.Add(d.closeReader)
	return
}

type DecoderCumulativeStats struct {
	AllocatedFrames  uint64
	AllocatedPackets uint64
	IncomingBytes    uint64
	IncomingPackets  uint64
	OutgoingFrames   uint64
	ProcessedBytes   uint64
	ProcessedPackets uint64
	WorkedDuration   time.Duration
}

func (d *Decoder) CumulativeStats() DecoderCumulativeStats {
	return DecoderCumulativeStats{
		AllocatedFrames:  atomic.LoadUint64(&d.fp.cumulativeStats.allocatedFrames),
		AllocatedPackets: atomic.LoadUint64(&d.pp.cumulativeStats.allocatedPackets),
		IncomingBytes:    atomic.LoadUint64(&d.cumulativeStats.incomingBytes),
		IncomingPackets:  atomic.LoadUint64(&d.cumulativeStats.incomingPackets),
		OutgoingFrames:   atomic.LoadUint64(&d.fd.cumulativeStats.outgoingFrames),
		ProcessedBytes:   atomic.LoadUint64(&d.cumulativeStats.processedBytes),
		ProcessedPackets: atomic.LoadUint64(&d.cumulativeStats.processedPackets),
		WorkedDuration:   d.ch.CumulativeStats().WorkedDuration,
	}
}

type decoderCumulativeStats struct {
	incomingBytes    uint64
	incomingPackets  uint64
	processedBytes   uint64
	processedPackets uint64
}

func (d *Decoder) DeltaStats() []astikit.DeltaStat {
	ss := d.fp.deltaStats()
	ss = append(ss, d.pp.deltaStats()...)
	ss = append(ss, d.fd.deltaStats()...)
	ss = append(ss, d.ch.DeltaStats()...)
	ss = append(ss,
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of packets processed per second",
				Label:       "Processed rate",
				Name:        astiflow.DeltaStatNameProcessedRate,
				Unit:        "pps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&d.cumulativeStats.processedPackets),
		},
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of bytes processed per second",
				Label:       "Processed byte rate",
				Name:        astiflow.DeltaStatNameProcessedByteRate,
				Unit:        "Bps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&d.cumulativeStats.processedBytes),
		},
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of bytes coming in per second",
				Label:       "Incoming byte rate",
				Name:        astiflow.DeltaStatNameIncomingByteRate,
				Unit:        "Bps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&d.cumulativeStats.incomingBytes),
		},
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of packets coming in per second",
				Label:       "Incoming rate",
				Name:        astiflow.DeltaStatNameIncomingRate,
				Unit:        "pps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&d.cumulativeStats.incomingPackets),
		},
	)
	return ss
}

func (d *Decoder) On(n astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return d.n.On(n, h)
}

func (d *Decoder) Connect(h FrameHandler) {
	// Connect
	d.n.Connector().Connect(h.NodeConnector())
}

func (d *Decoder) Disconnect(h FrameHandler) {
	// Disconnect
	d.n.Connector().Disconnect(h.NodeConnector())
}

func (d *Decoder) NodeConnector() *astiflow.NodeConnector {
	return d.n.Connector()
}

func (d *Decoder) Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	tc().Do(func() {
		// Make sure to flush the reader
		defer func() {
			if d.r != nil {
				d.flush()
			}
		}()

		// Make sure to stop the chan properly
		defer d.ch.Stop()

		// Start chan
		d.ch.Start(ctx)
	})
}

func (d *Decoder) HandlePacket(p Packet) {
	// Everything executed outside the main loop should be protected from the closer
	d.c.Do(func() {
		// Increment stats
		atomic.AddUint64(&d.cumulativeStats.incomingBytes, uint64(p.Size()))
		atomic.AddUint64(&d.cumulativeStats.incomingPackets, 1)

		// Copy packet
		pkt, err := d.pp.copy(p.Packet)
		if err != nil {
			dispatchError(d.n, fmt.Errorf("astiavflow: copying packet failed: %w", err).Error())
			return
		}

		// Add to chan
		d.ch.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			d.c.Do(func() {
				// Make sure to close pkt
				defer d.pp.put(pkt)

				// Increment stats
				atomic.AddUint64(&d.cumulativeStats.processedBytes, uint64(pkt.Size()))
				atomic.AddUint64(&d.cumulativeStats.processedPackets, 1)

				// Store previous media context
				d.previousMediaContext = p.MediaContext

				// Enforce monotonic dts
				if d.enforceMonotonicDTS && d.previousDts != nil && *d.previousDts >= pkt.Dts() {
					dispatchError(d.n, "astiavflow: enforcing monotonic dts failed: %d >= %d", *d.previousDts, pkt.Dts())
					return
				}
				d.previousDts = astikit.Int64Ptr(pkt.Dts())

				// We must refresh the reader
				if d.mustRefreshReader(p) {
					// Handle previous reader
					// TODO Test
					if d.r != nil {
						// Flush
						d.flush()

						// Close
						d.closeReader()
					}

					// Create reader
					if err = d.createReader(p); err != nil {
						dispatchError(d.n, fmt.Errorf("astiavflow: creating reader failed: %w", err).Error())
						return
					}
				}

				// Decode
				d.decode(pkt, p.MediaContext)
			})
		})
	})
}

// TODO Detect mandatory changes + test
func (d *Decoder) mustRefreshReader(_ Packet) bool {
	return d.r == nil
}

func (d *Decoder) createReader(p Packet) (err error) {
	// Get reader options
	var name string
	if d.ro != nil {
		name = d.ro(p)
	}

	// Find decoder
	var codec *astiav.Codec
	if name != "" {
		if codec = astiav.FindDecoderByName(name); codec == nil {
			err = fmt.Errorf("astiavflow: no decoder found with name %s", name)
			return
		}
	} else {
		if codec = astiav.FindDecoder(p.CodecParameters.CodecID()); codec == nil {
			err = fmt.Errorf("astiavflow: no decoder found for codec id %s", p.CodecParameters.CodecID())
			return
		}
	}

	// No codec
	if codec == nil {
		err = errors.New("astiavflow: empty codec")
		return
	}

	// Create reader
	r := newDecoderReader(codec)
	if r == nil {
		err = errors.New("astiavflow: empty reader")
		return
	}

	// Make sure to free reader on error
	defer func() {
		if err != nil {
			r.Free()
		}
	}()

	// Initialize reader with codec parameters
	if err = r.FromCodecParameters(p.CodecParameters); err != nil {
		err = fmt.Errorf("astiavflow: initializing reader with codec parameters failed: %w", err)
		return
	}

	// Open
	if err = r.Open(codec, nil); err != nil {
		err = fmt.Errorf("astiavflow: opening reader failed: %w", err)
		return
	}

	// Store reader
	d.r = r
	classers.set(r, d.n)
	return
}

func (d *Decoder) closeReader() {
	if d.r == nil {
		return
	}
	classers.del(d.r)
	d.r.Free()
}

func (d *Decoder) flush() {
	d.decode(nil, d.previousMediaContext)
}

func (d *Decoder) decode(pkt *astiav.Packet, m MediaContext) {
	// Send packet
	if err := d.r.SendPacket(pkt); err != nil {
		dispatchError(d.n, fmt.Errorf("astiavflow: sending packet failed: %w", err).Error())
		return
	}

	// Loop
	for {
		// Receive frame
		if stop := d.receiveFrame(m); stop {
			return
		}
	}
}

func (d *Decoder) receiveFrame(m MediaContext) (stop bool) {
	// Get frame
	f := d.fp.get()
	defer d.fp.put(f)

	// Receive frame
	if err := d.r.ReceiveFrame(f); err != nil {
		if !errors.Is(err, astiav.ErrEof) && !errors.Is(err, astiav.ErrEagain) {
			dispatchError(d.n, fmt.Errorf("astiavflow: receiving frame failed: %w", err).Error())
		}
		stop = true
		return
	}

	// Dispatch frame
	d.fd.dispatch(Frame{
		Frame:        f,
		MediaContext: m,
	})
	return
}

type decoderReader interface {
	Class() *astiav.Class
	Free()
	FromCodecParameters(cp *astiav.CodecParameters) error
	Open(c *astiav.Codec, d *astiav.Dictionary) error
	ReceiveFrame(f *astiav.Frame) error
	SendPacket(p *astiav.Packet) error
}

var newDecoderReader = func(c *astiav.Codec) decoderReader {
	return astiav.AllocCodecContext(c)
}
