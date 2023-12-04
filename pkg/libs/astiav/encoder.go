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

var _ FrameHandler = (*Encoder)(nil)

var (
	countEncoder uint64
)

type Encoder struct {
	*frameHandler
	c  *astikit.Closer
	cp *astiav.CodecParameters
	n  *astiflow.Node
	pd *packetDispatcher
	pp *packetPool
	w  encoderWriter
	wo EncoderWriterOptions
}

type EncoderOptions struct {
	Group    *astiflow.Group
	Metadata astiflow.Metadata
	Stop     *astiflow.NodeStopOptions
	Writer   EncoderWriterOptions
}

type EncoderWriterOptions struct {
	BitRate     int64
	CodecID     astiav.CodecID
	CodecName   string
	Dictionary  DictionaryOptions
	Flags       astiav.CodecContextFlags
	GopSize     func(framerate astiav.Rational) int
	ThreadCount int
	ThreadType  astiav.ThreadType
}

func NewEncoder(o EncoderOptions) (e *Encoder, err error) {
	// Create encoder
	e = &Encoder{
		frameHandler: newFrameHandler(),
		pd:           newPacketDispatcher(),
		pp:           newPacketPool(),
		wo:           o.Writer,
	}

	// Create node
	if e.n, e.c, err = o.Group.NewNode(astiflow.NodeOptions{
		Metadata: (&astiflow.Metadata{
			Name: fmt.Sprintf("encoder_%d", atomic.AddUint64(&countEncoder, uint64(1))),
			Tags: []string{"encoder"},
		}).Merge(o.Metadata),
		Noder: e,
		Stop:  o.Stop,
	}); err != nil {
		err = fmt.Errorf("astiavflow: creating node failed: %w", err)
		return
	}

	// Initialize dispatchers, handlers and pools
	e.frameHandler.init(frameHandlerInitOptions{
		c:         e.c,
		n:         e.n,
		onConnect: e.onConnect,
		onFrame:   e.onFrame,
	})
	e.pd.init(e.n)
	e.pp.init(e.c)

	// Make sure to close writer
	e.c.Add(e.closeWriter)
	return
}

type EncoderCumulativeStats struct {
	AllocatedPackets uint64
	FrameHandlerCumulativeStats
	OutgoingBytes   uint64
	OutgoingPackets uint64
}

func (e *Encoder) CumulativeStats() EncoderCumulativeStats {
	return EncoderCumulativeStats{
		AllocatedPackets:            atomic.LoadUint64(&e.pp.cs.allocatedPackets),
		FrameHandlerCumulativeStats: e.frameHandler.cumulativeStats(),
		OutgoingBytes:               atomic.LoadUint64(&e.pd.cs.outgoingBytes),
		OutgoingPackets:             atomic.LoadUint64(&e.pd.cs.outgoingPackets),
	}
}

func (e *Encoder) DeltaStats() []astikit.DeltaStat {
	ss := e.frameHandler.deltaStats()
	ss = append(ss, e.pp.deltaStats()...)
	ss = append(ss, e.pd.deltaStats()...)
	return ss
}

func (e *Encoder) On(n astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return e.n.On(n, h)
}

func (e *Encoder) Connect(h PacketHandler) error {
	// Get initial outgoing packet descriptor
	pd, err := e.initialOutgoingPacketDescriptor()
	if err != nil {
		return fmt.Errorf("astiavflow: getting initial outgoing packet descriptor failed: %w: encoder needs at least one parent before adding children", err)
	}

	// Callback
	if err := h.OnConnect(pd, e); err != nil {
		return fmt.Errorf("astiavflow: callback failed: %w", err)
	}

	// Connect
	e.n.Connector().Connect(h.NodeConnector())
	return nil
}

func (e *Encoder) initialOutgoingPacketDescriptor() (PacketDescriptor, error) {
	// No codec parameters
	if e.cp == nil {
		return PacketDescriptor{}, errors.New("astiavflow: no codec parameters")
	}

	// No initial incoming frame descriptor
	fd, ok := e.initialIncomingFrameDescriptors.one()
	if !ok {
		return PacketDescriptor{}, errors.New("astiavflow: no initial incoming frame descriptor")
	}
	return PacketDescriptor{
		CodecParameters: e.cp,
		MediaDescriptor: fd.MediaDescriptor,
	}, nil
}

func (e *Encoder) Disconnect(h PacketHandler) {
	// Disconnect
	e.n.Connector().Disconnect(h.NodeConnector())
}

func (e *Encoder) NodeConnector() *astiflow.NodeConnector {
	return e.n.Connector()
}

func (e *Encoder) Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	tc().Do(func() {
		// Make sure to flush the writer
		defer func() {
			if e.w != nil {
				e.flush()
			}
		}()

		// Start frame handler
		e.frameHandler.start(ctx)
	})
}

func (e *Encoder) onConnect(n astiflow.Noder) error {
	// Get initial incoming frame descriptor
	fd, ok := e.initialIncomingFrameDescriptors.get(n)
	if !ok {
		return errors.New("astiavflow: no initial incoming frame descriptor for noder")
	}

	// Refresh writer
	if err := e.refreshWriter(fd); err != nil {
		return fmt.Errorf("astiavflow: refreshing writer failed: %w", err)
	}
	return nil
}

func (e *Encoder) onFrame(acquireFrameFunc frameHandlerAcquireFrameFunc, f *astiav.Frame, fd FrameDescriptor, n astiflow.Noder) {
	// Refresh the writer
	if e.mustRefreshWriter(fd) {
		if err := e.refreshWriter(fd); err != nil {
			dispatchError(e.n, fmt.Errorf("astiavflow: refreshing writer failed: %w", err).Error())
			return
		}
	}

	// Encode
	e.encode(fd, f)
}

func (e *Encoder) mustRefreshWriter(d FrameDescriptor) bool {
	fd, ok := e.previousIncomingFrameDescriptors.one()
	return e.w == nil || (ok && !fd.equal(d))
}

func (e *Encoder) refreshWriter(d FrameDescriptor) error {
	// Handle previous reader
	if e.w != nil {
		// Flush
		e.flush()

		// Close
		e.closeWriter()
	}

	// Create writer
	if err := e.createWriter(d); err != nil {
		return fmt.Errorf("astiavflow: creating writer failed: %w", err)
	}
	return nil
}

func (e *Encoder) createWriter(d FrameDescriptor) (err error) {
	// Find encoder
	var codec *astiav.Codec
	if e.wo.CodecName != "" {
		if codec = astiav.FindEncoderByName(e.wo.CodecName); codec == nil {
			err = fmt.Errorf("astilibav: no encoder found with name %s", e.wo.CodecName)
			return
		}
	} else if e.wo.CodecID > 0 {
		if codec = astiav.FindEncoder(e.wo.CodecID); codec == nil {
			err = fmt.Errorf("astilibav: no encoder found with codec id %s", e.wo.CodecID)
			return
		}
	}

	// No codec
	if codec == nil {
		err = errors.New("astiavflow: empty codec")
		return
	}

	// Create writer
	w := newEncoderWriter(codec)
	if w == nil {
		err = errors.New("astiavflow: empty writer")
		return
	}

	// Make sure to free writer on error
	defer func() {
		if err != nil {
			w.Free()
		}
	}()

	// Update writer
	switch d.MediaType {
	case astiav.MediaTypeAudio:
		w.SetChannelLayout(d.ChannelLayout)
		w.SetSampleFormat(d.SampleFormat)
		w.SetSampleRate(d.SampleRate)
	case astiav.MediaTypeVideo:
		w.SetFramerate(d.MediaDescriptor.FrameRate)
		if e.wo.GopSize != nil {
			w.SetGopSize(e.wo.GopSize(d.MediaDescriptor.FrameRate))
		}
		w.SetHeight(d.Height)
		w.SetPixelFormat(d.PixelFormat)
		w.SetSampleAspectRatio(d.SampleAspectRatio)
		w.SetWidth(d.Width)
	}
	if e.wo.BitRate > 0 {
		w.SetBitRate(e.wo.BitRate)
	}
	if e.wo.Flags > 0 {
		w.SetFlags(e.wo.Flags)
	}
	if e.wo.ThreadCount > 0 {
		w.SetThreadCount(e.wo.ThreadCount)
	}
	if e.wo.ThreadType > 0 {
		w.SetThreadType(e.wo.ThreadType)
	}
	w.SetTimeBase(d.MediaDescriptor.TimeBase)

	// Dictionary
	var dict *dictionary
	if dict, err = e.wo.Dictionary.dictionary(); err != nil {
		err = fmt.Errorf("astiavflow: creating dictionary failed: %w", err)
		return
	}
	defer dict.close()

	// Open
	if err = w.Open(codec, dict.Dictionary); err != nil {
		err = fmt.Errorf("astiavflow: opening writer failed: %w", err)
		return
	}

	// Alloc codec parameters
	cp := astiav.AllocCodecParameters()
	if cp == nil {
		err = errors.New("astiavflow: empty codec parameters")
		return
	}

	// Make sure to free codec parameters on error
	defer func() {
		if err != nil {
			cp.Free()
		}
	}()

	// Initialize codec parameters with writer
	if err = w.ToCodecParameters(cp); err != nil {
		err = fmt.Errorf("astiavflow: initializing codec parameters with writer failed: %w", err)
		return
	}

	// Store codec parameters
	e.cp = cp

	// Store writer
	e.w = w
	classers.set(w, e.n)
	return
}

func (e *Encoder) closeWriter() {
	if e.cp != nil {
		e.cp.Free()
	}
	if e.w != nil {
		classers.del(e.w)
		e.w.Free()
	}
}

func (e *Encoder) flush() {
	fd, ok := e.previousIncomingFrameDescriptors.one()
	if !ok {
		if fd, ok = e.initialIncomingFrameDescriptors.one(); !ok {
			return
		}
	}
	e.encode(fd, nil)
}

func (e *Encoder) encode(d FrameDescriptor, f *astiav.Frame) {
	// Reset frame
	if f != nil {
		f.SetKeyFrame(false)
		f.SetPictureType(astiav.PictureTypeNone)
	}

	// Send frame
	if err := e.w.SendFrame(f); err != nil {
		dispatchError(e.n, fmt.Errorf("astiavflow: sending frame failed: %w", err).Error())
		return
	}

	// Loop
	for {
		// Receive packet
		if stop := e.receivePacket(d.MediaDescriptor); stop {
			return
		}
	}
}

func (e *Encoder) receivePacket(m MediaDescriptor) (stop bool) {
	// Get packet
	pkt := e.pp.get()
	defer e.pp.put(pkt)

	// Receive packet
	if err := e.w.ReceivePacket(pkt); err != nil {
		if !errors.Is(err, astiav.ErrEof) && !errors.Is(err, astiav.ErrEagain) {
			dispatchError(e.n, fmt.Errorf("astiavflow: receiving packet failed: %w", err).Error())
		}
		stop = true
		return
	}

	// Set pkt duration based on framerate
	if f := e.w.Framerate(); f.Num() > 0 {
		pkt.SetDuration(astiav.RescaleQ(int64(1e9/f.Float64()), NanosecondRational, e.w.TimeBase()))
	}

	// TODO Is this ever happening?
	if m.TimeBase.Float64() != e.w.TimeBase().Float64() {
		panic(fmt.Sprintf("astiavflow: so it happens!! m.TimeBase (%s) != e.w.TimeBase() (%s)", m.TimeBase, e.w.TimeBase()))

		// Update packet
		//pkt.RescaleTs(m.TimeBase, e.w.TimeBase())

		// Update media context
		//m.TimeBase = e.w.TimeBase()
	}

	// Dispatch frame
	e.pd.dispatch(Packet{
		Packet: pkt,
		PacketDescriptor: PacketDescriptor{
			CodecParameters: e.cp,
			MediaDescriptor: m,
		},
	})
	return
}

type encoderWriter interface {
	Class() *astiav.Class
	Flags() astiav.CodecContextFlags
	Framerate() astiav.Rational
	Free()
	Open(c *astiav.Codec, d *astiav.Dictionary) error
	ReceivePacket(p *astiav.Packet) error
	SendFrame(f *astiav.Frame) error
	SetBitRate(bitRate int64)
	SetChannelLayout(l astiav.ChannelLayout)
	SetFlags(fs astiav.CodecContextFlags)
	SetFramerate(f astiav.Rational)
	SetGopSize(gopSize int)
	SetHeight(height int)
	SetPixelFormat(pixFmt astiav.PixelFormat)
	SetSampleAspectRatio(r astiav.Rational)
	SetSampleFormat(f astiav.SampleFormat)
	SetSampleRate(sampleRate int)
	SetThreadCount(threadCount int)
	SetThreadType(t astiav.ThreadType)
	SetTimeBase(r astiav.Rational)
	SetWidth(width int)
	TimeBase() astiav.Rational
	ToCodecParameters(cp *astiav.CodecParameters) error
}

var newEncoderWriter = func(c *astiav.Codec) encoderWriter {
	return astiav.AllocCodecContext(c)
}
