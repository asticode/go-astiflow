package astiavflow

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

var _ PacketHandler = (*Decoder)(nil)

var (
	countDecoder uint64
)

type Decoder struct {
	*packetHandler
	c                   *astikit.Closer
	enforceMonotonicDTS bool
	fd                  *frameDispatcher
	fp                  *framePool
	n                   *astiflow.Node
	previousDts         *int64
	r                   decoderReader
	ro                  DecoderReaderOptions
	threadCount         int
	threadType          astiav.ThreadType
}

type DecoderOptions struct {
	EnforceMonotonicDTS bool
	Group               *astiflow.Group
	Metadata            astiflow.Metadata
	Reader              DecoderReaderOptions
	Stop                *astiflow.NodeStopOptions
	ThreadCount         int
	ThreadType          astiav.ThreadType
}

type DecoderReaderOptions func(d PacketDescriptor) (decoderName string)

func NewDecoder(o DecoderOptions) (d *Decoder, err error) {
	// Create decoder
	d = &Decoder{
		enforceMonotonicDTS: o.EnforceMonotonicDTS,
		fd:                  newFrameDispatcher(),
		fp:                  newFramePool(),
		packetHandler:       newPacketHandler(),
		ro:                  o.Reader,
		threadCount:         o.ThreadCount,
		threadType:          o.ThreadType,
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

	// Initialize dispatchers, handler and pools
	d.fd.init(d.n)
	d.fp.init(d.c)
	d.packetHandler.init(packetHandlerInitOptions{
		c:         d.c,
		n:         d.n,
		onConnect: d.onConnect,
		onPacket:  d.onPacket,
	})

	// Make sure to close reader
	d.c.Add(d.closeReader)
	return
}

type DecoderCumulativeStats struct {
	AllocatedFrames uint64
	OutgoingFrames  uint64
	PacketHandlerCumulativeStats
}

func (d *Decoder) CumulativeStats() DecoderCumulativeStats {
	return DecoderCumulativeStats{
		AllocatedFrames:              atomic.LoadUint64(&d.fp.cs.allocatedFrames),
		OutgoingFrames:               atomic.LoadUint64(&d.fd.cs.outgoingFrames),
		PacketHandlerCumulativeStats: d.packetHandler.cumulativeStats(),
	}
}

func (d *Decoder) DeltaStats() []astikit.DeltaStat {
	ss := d.fp.deltaStats()
	ss = append(ss, d.fd.deltaStats()...)
	ss = append(ss, d.packetHandler.deltaStats()...)
	return ss
}

func (d *Decoder) On(n astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return d.n.On(n, h)
}

func (d *Decoder) Connect(h FrameHandler) error {
	// Get initial outgoing frame descriptor
	fd, err := d.initialOutgoingFrameDescriptor()
	if err != nil {
		return fmt.Errorf("astiavflow: getting initial outgoing frame descriptor failed: %w: decoder needs at least one parent before adding children", err)
	}

	// Callback
	if err := h.OnConnect(fd, d); err != nil {
		return fmt.Errorf("astiavflow: callback failed: %w", err)
	}

	// Connect
	d.n.Connector().Connect(h.NodeConnector())
	return nil
}

func (d *Decoder) initialOutgoingFrameDescriptor() (FrameDescriptor, error) {
	// No initial incoming packet descriptor
	pd, ok := d.initialIncomingPacketDescriptors.one()
	if !ok {
		return FrameDescriptor{}, errors.New("astiavflow: no initial incoming previous packet descriptor")
	}

	// Create frame descriptor
	return newFrameDescriptorFromPacketDescriptor(pd), nil
}

func (d *Decoder) Disconnect(h FrameHandler) {
	// Disconnect
	d.n.Connector().Disconnect(h.NodeConnector())
}

func (d *Decoder) Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	tc().Do(func() {
		// Make sure to flush the reader
		defer func() {
			if d.r != nil {
				d.flush()
			}
		}()

		// Start packet handler
		d.packetHandler.start(ctx)
	})
}

func (d *Decoder) onConnect(n astiflow.Noder) error {
	// Get initial incoming packet descriptor
	pd, ok := d.initialIncomingPacketDescriptors.get(n)
	if !ok {
		return errors.New("astiavflow: no initial incoming packet descriptor for noder")
	}

	// Refresh reader
	if err := d.refreshReader(pd); err != nil {
		return fmt.Errorf("astiavflow: refreshing reader failed: %w", err)
	}
	return nil
}

func (d *Decoder) onPacket(n astiflow.Noder, p *astiav.Packet, pd PacketDescriptor) {
	// Enforce monotonic dts
	if d.enforceMonotonicDTS && d.previousDts != nil && *d.previousDts >= p.Dts() {
		dispatchError(d.n, "astiavflow: enforcing monotonic dts failed: %d >= %d", *d.previousDts, p.Dts())
		return
	}
	d.previousDts = astikit.Int64Ptr(p.Dts())

	// Refresh the reader
	if d.mustRefreshReader() {
		if err := d.refreshReader(pd); err != nil {
			dispatchError(d.n, fmt.Errorf("astiavflow: refreshing reader failed: %w", err).Error())
			return
		}
	}

	// Decode
	d.decode(p, pd)
}

// TODO Detect mandatory changes + test
func (d *Decoder) mustRefreshReader() bool {
	return d.r == nil
}

func (d *Decoder) refreshReader(pd PacketDescriptor) error {
	// Handle previous reader
	// TODO Test
	if d.r != nil {
		// Flush
		d.flush()

		// Close
		d.closeReader()
	}

	// Create reader
	if err := d.createReader(pd); err != nil {
		return fmt.Errorf("astiavflow: creating reader failed: %w", err)
	}
	return nil
}

func (d *Decoder) createReader(pd PacketDescriptor) (err error) {
	// Get reader options
	var name string
	if d.ro != nil {
		name = d.ro(pd)
	}

	// Find decoder
	var codec *astiav.Codec
	if name != "" {
		if codec = astiav.FindDecoderByName(name); codec == nil {
			err = fmt.Errorf("astiavflow: no decoder found with name %s", name)
			return
		}
	} else {
		if codec = astiav.FindDecoder(pd.CodecParameters.CodecID()); codec == nil {
			err = fmt.Errorf("astiavflow: no decoder found for codec id %s", pd.CodecParameters.CodecID())
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

	// Set thread parameters
	if d.threadCount > 0 {
		r.SetThreadCount(d.threadCount)
	}
	if d.threadType != astiav.ThreadTypeUndefined {
		r.SetThreadType(d.threadType)
	}

	// Initialize reader with codec parameters
	if err = r.FromCodecParameters(pd.CodecParameters); err != nil {
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
	pd, ok := d.previousIncomingPacketDescriptors.one()
	if !ok {
		if pd, ok = d.initialIncomingPacketDescriptors.one(); !ok {
			return
		}
	}
	d.decode(nil, pd)
}

func (d *Decoder) decode(p *astiav.Packet, pd PacketDescriptor) {
	// Send packet
	if err := d.r.SendPacket(p); err != nil {
		dispatchError(d.n, fmt.Errorf("astiavflow: sending packet failed: %w", err).Error())
		return
	}

	// Loop
	for {
		// Receive frame
		if stop := d.receiveFrame(pd.MediaDescriptor, pd.CodecParameters.MediaType()); stop {
			return
		}
	}
}

func (d *Decoder) receiveFrame(md MediaDescriptor, mt astiav.MediaType) (stop bool) {
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
		Frame:           f,
		FrameDescriptor: newFrameDescriptorFromFrame(f, md, mt),
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
	SetThreadCount(int)
	SetThreadType(astiav.ThreadType)
}

var newDecoderReader = func(c *astiav.Codec) decoderReader {
	return astiav.AllocCodecContext(c)
}
