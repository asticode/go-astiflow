package astiavflow

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

var (
	countDemuxer uint64
)

type Demuxer struct {
	c                *astikit.Closer
	cs               *demuxerCumulativeStats
	flushOnStart     bool
	hss              map[int]map[PacketHandler]bool // Indexed by stream index
	ii               *astiav.IOInterrupter
	l                *demuxerLoop
	mh               sync.Mutex // Locks hss
	ms               sync.Mutex // Locks ss
	n                *astiflow.Node
	onReadFrameError DemuxerReadFrameErrorHandler
	pb               *demuxerProbe
	pd               *packetDispatcher
	pp               *packetPool
	prc              PacketRateController
	r                demuxerReader
	ss               map[int]*demuxerStream // Indexed by stream index
}

type DemuxerOptions struct {
	Group    *astiflow.Group
	Metadata astiflow.Metadata
	Start    DemuxerStartOptions
	Stop     *astiflow.NodeStopOptions
}

type DemuxerStartOptions struct {
	// If true, flushes internal data on start
	Flush bool
	// Demuxer will seek back to the start of the input when eof is reached
	// In this case the packets are restamped
	Loop bool
	// Custom read frame error handler
	// If handled is false, default error handling will be executed
	OnReadFrameError DemuxerReadFrameErrorHandler
}

type DemuxerReadFrameErrorHandler func(d *Demuxer, err error) (stop, handled bool)

func NewDemuxer(o DemuxerOptions) (d *Demuxer, err error) {
	// Create demuxer
	d = &Demuxer{
		cs:               &demuxerCumulativeStats{},
		flushOnStart:     o.Start.Flush,
		hss:              make(map[int]map[PacketHandler]bool),
		l:                o.Start.loop(),
		onReadFrameError: o.Start.OnReadFrameError,
		pb:               newDemuxerProbe(),
		pd:               newPacketDispatcher(),
		pp:               newPacketPool(),
		ss:               make(map[int]*demuxerStream),
	}

	// Create node
	if d.n, d.c, err = o.Group.NewNode(astiflow.NodeOptions{
		Metadata: (&astiflow.Metadata{
			Name: fmt.Sprintf("demuxer_%d", atomic.AddUint64(&countDemuxer, uint64(1))),
			Tags: []string{"demuxer"},
		}).Merge(o.Metadata),
		Noder: d,
		Stop:  o.Stop,
	}); err != nil {
		err = fmt.Errorf("astiavflow: creating node failed: %w", err)
		return
	}

	// Initialize dispatchers and pools
	d.pd.init(d.n)
	d.pp.init(d.c)

	// Make sure to unlink handler from its streams when child is removed
	d.n.On(astiflow.EventNameNodeChildRemoved, func(payload interface{}) (remove bool) {
		// Assert payload
		n, ok := payload.(*astiflow.Node)
		if !ok {
			return
		}

		// Assert noder
		h, ok := n.Noder().(PacketHandler)
		if !ok {
			return
		}

		// Unlink handler from its streams
		d.mh.Lock()
		for k := range d.hss {
			delete(d.hss[k], h)
			if len(d.hss[k]) == 0 {
				delete(d.hss, k)
			}
		}
		d.mh.Unlock()
		return
	})

	// Create new reader
	r := newDemuxerReader()
	classers.set(r, d.n)
	d.c.Add(func() {
		r.Free()
		// Make sure to remove from classers after freeing the object since the free method
		// may use methods needing the classer
		classers.del(r)
	})

	// Store reader
	d.r = r

	// Set io interrupter
	d.ii = astiav.NewIOInterrupter()
	d.c.Add(d.ii.Free)
	d.r.SetIOInterrupter(d.ii)
	return
}

type DemuxerOpenOptions struct {
	Dictionary DictionaryOptions
	Format     *astiav.InputFormat
	IOContext  *astiav.IOContext
	URL        string
}

func (d *Demuxer) Open(ctx context.Context, o DemuxerOpenOptions) (err error) {
	// Dictionary
	var dict *dictionary
	if dict, err = o.Dictionary.dictionary(); err != nil {
		err = fmt.Errorf("astiavflow: creating dictionary failed: %w", err)
		return
	}
	defer dict.close()

	// Make sure to resume interrupt callback
	d.ii.Resume()

	// Process context
	if ctx != nil {
		// Create child context
		childCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Watch child context in a goroutine
		go func() {
			// Wait for child context to be done
			<-childCtx.Done()

			// Context error
			if ctx.Err() != nil {
				// Interrupt
				d.ii.Interrupt()
			}
		}()
	}

	// No url but an io context, we need to set the pb before opening the input
	if o.URL == "" && o.IOContext != nil {
		d.r.SetPb(o.IOContext)
	}

	// Open input
	if err = d.r.OpenInput(o.URL, o.Format, dict.Dictionary); err != nil {
		err = fmt.Errorf("astiavflow: opening input failed: %w", err)
		return
	}
	d.c.Add(func() {
		pb := d.r.Pb()
		d.r.CloseInput()
		// Make sure to remove from classers after freeing the object since the free method
		// may use methods needing the classer
		if pb != nil {
			classers.del(pb)
		}
	})

	// An url and an io context, we need to set the pb after opening the input
	if o.URL != "" && o.IOContext != nil {
		// Make sure the previous pb is closed
		if pb := d.r.Pb(); pb != nil {
			pb.Close()
		}

		// Update
		d.r.SetPb(o.IOContext)
		d.r.SetFlags(d.r.Flags().Add(astiav.FormatContextFlagCustomIo))
	}

	// Store input
	if pb := d.r.Pb(); pb != nil {
		classers.set(pb, d.n)
	}

	// Context error
	if ctx != nil && ctx.Err() != nil {
		err = fmt.Errorf("astiavflow: context error: %w", ctx.Err())
		return
	}

	// Find stream information
	if err = d.r.FindStreamInfo(nil); err != nil {
		err = fmt.Errorf("astiavflow: finding stream info failed: %w", err)
		return
	}

	// Context error
	if ctx != nil && ctx.Err() != nil {
		err = fmt.Errorf("astiavflow: context error: %w", ctx.Err())
		return
	}

	// Loop through streams
	for _, s := range d.r.Streams() {
		// Create stream
		d.ms.Lock()
		d.ss[s.Index()] = d.newDemuxerStream(s)
		d.ms.Unlock()
	}
	return
}

// Probes the starting packets during the provided duration and retrieves
// the first overall PTS and the streams whose first PTS is the same as the first
// overall PTS.
// We don't stop before probeDuration in case the smallest PTS is not in the
// first packet.
func (d *Demuxer) Probe(pd time.Duration) (dp DemuxerProbe, err error) {
	// Probe has already been done
	if d.pb.info != nil {
		dp = *d.pb.info
		return
	}

	// Invalid probe duration
	if pd <= 0 {
		err = errors.New("astiavflow: provided duration <= 0")
		return
	}

	// Loop
	firstPTSs := make(map[*demuxerStream]int64)
	for {
		// Get packet from pool
		pkt := d.pp.get()

		// Read frame
		if errReadFrame := d.r.ReadFrame(pkt); errReadFrame != nil {
			// Make sure to close packet
			d.pp.put(pkt)

			// We've reached eof but we have enough information
			if errors.Is(errReadFrame, astiav.ErrEof) && len(firstPTSs) > 0 {
				break
			}

			// We don't have enough information
			err = fmt.Errorf("astiavflow: reading frame failed: %w", errReadFrame)
			return
		}

		// Add packet to probe data
		d.pb.data = append(d.pb.data, pkt)

		// Invalid timestamps
		// Only frames with PTS >= 0 get out of decoders
		if pkt.Pts() == astiav.NoPtsValue || pkt.Pts() < 0 {
			continue
		}

		// Get stream
		d.ms.Lock()
		s, ok := d.ss[pkt.StreamIndex()]
		d.ms.Unlock()
		if !ok {
			continue
		}

		// Get pts
		pts := pkt.Pts()

		// Process packet side data
		if skippedStart, _ := d.processPacketSideData(pkt, s); skippedStart > 0 {
			// Get duration
			sd, _ := durationToTimeBase(skippedStart, s.s.MediaDescriptor.TimeBase)

			// Update pts
			pts += sd
		}

		// Update first pts
		if firstPTS, ok := firstPTSs[s]; ok {
			if pts < firstPTS {
				firstPTSs[s] = pts
			}
		} else {
			firstPTSs[s] = pts
		}

		// We've reached probe duration
		if time.Duration(astiav.RescaleQ(pts-firstPTSs[s], s.s.MediaDescriptor.TimeBase, NanosecondRational)) > pd {
			break
		}
	}

	// Get first overall PTS in nanosecond timebase
	var firstPTS *int64
	for s, v := range firstPTSs {
		pts := astiav.RescaleQ(v, s.s.MediaDescriptor.TimeBase, NanosecondRational)
		if firstPTS == nil {
			firstPTS = astikit.Int64Ptr(pts)
		} else if pts < *firstPTS {
			*firstPTS = pts
		}
	}

	// Update probe info
	d.pb.info = &DemuxerProbe{FirstPTS: DemuxerProbeFirstPTS{
		streams:  make(map[int]bool),
		Timebase: NanosecondRational,
		Value:    *firstPTS,
	}}
	for s, v := range firstPTSs {
		if pts := astiav.RescaleQ(v, s.s.MediaDescriptor.TimeBase, NanosecondRational); pts == *firstPTS {
			d.pb.info.FirstPTS.streams[s.s.Index] = true
		}
	}
	dp = *d.pb.info
	return
}

func (d *Demuxer) processPacketSideData(pkt *astiav.Packet, s *demuxerStream) (skippedStart, skippedEnd time.Duration) {
	switch {
	case s.s.CodecParameters.MediaType() == astiav.MediaTypeAudio:
		skippedStart, skippedEnd = d.processPacketSideDataSkipSamples(pkt, s.s.CodecParameters.SampleRate())
	}
	return
}

func (d *Demuxer) processPacketSideDataSkipSamples(pkt *astiav.Packet, sampleRate int) (skippedStart, skippedEnd time.Duration) {
	// Get skip samples side data
	ss, ok := pkt.SideData().SkipSamples().Get()
	if !ok {
		return
	}

	// Skipped start
	if ss.SkipStart > 0 {
		skippedStart = time.Duration(float64(ss.SkipStart) / float64(sampleRate) * float64(1e9))
	}

	// Skipped end
	if ss.SkipEnd > 0 {
		skippedEnd = time.Duration(float64(ss.SkipEnd) / float64(sampleRate) * float64(1e9))
	}
	return
}

type DemuxerCumulativeStats struct {
	AllocatedPackets uint64
	IncomingBytes    uint64
	IncomingPackets  uint64
	OutgoingBytes    uint64
	OutgoingPackets  uint64
}

func (d *Demuxer) CumulativeStats() DemuxerCumulativeStats {
	return DemuxerCumulativeStats{
		AllocatedPackets: atomic.LoadUint64(&d.pp.cs.allocatedPackets),
		IncomingBytes:    atomic.LoadUint64(&d.cs.incomingBytes),
		IncomingPackets:  atomic.LoadUint64(&d.cs.incomingPackets),
		OutgoingBytes:    atomic.LoadUint64(&d.pd.cs.outgoingBytes),
		OutgoingPackets:  atomic.LoadUint64(&d.pd.cs.outgoingPackets),
	}
}

type demuxerCumulativeStats struct {
	incomingBytes   uint64
	incomingPackets uint64
}

func (d *Demuxer) DeltaStats() []astikit.DeltaStat {
	ss := d.pd.deltaStats()
	ss = append(ss, d.pp.deltaStats()...)
	ss = append(ss,
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of bytes coming in per second",
				Label:       "Incoming byte rate",
				Name:        astiflow.DeltaStatNameIncomingByteRate,
				Unit:        "Bps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&d.cs.incomingBytes),
		},
		astikit.DeltaStat{
			Metadata: astikit.DeltaStatMetadata{
				Description: "Number of packets coming in per second",
				Label:       "Incoming rate",
				Name:        astiflow.DeltaStatNameIncomingRate,
				Unit:        "pps",
			},
			Valuer: astikit.NewAtomicUint64RateDeltaStat(&d.cs.incomingPackets),
		},
	)
	return ss
}

func (d *Demuxer) Loop(loop bool) {
	atomic.StoreUint32(&d.l.enabled, astikit.BoolToUInt32(loop))
}

func (d *Demuxer) Streams() (ss []Stream) {
	// Lock
	d.ms.Lock()
	defer d.ms.Unlock()

	// Get indexes
	var idxs []int
	for idx := range d.ss {
		idxs = append(idxs, idx)
	}

	// Sort indexes
	sort.Ints(idxs)

	// Loop through indexes
	for _, idx := range idxs {
		ss = append(ss, d.ss[idx].s)
	}
	return
}

func (d *Demuxer) SetPacketRateController(c PacketRateController) {
	d.prc = c
}

func (d *Demuxer) On(n astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return d.n.On(n, h)
}

func (d *Demuxer) Connect(h PacketHandler, s Stream) error {
	// Callback
	if err := h.OnConnect(s.PacketDescriptor, d); err != nil {
		return fmt.Errorf("astiavflow: callback failed: %w", err)
	}

	// Link stream to handler
	d.mh.Lock()
	if _, ok := d.hss[s.Index]; !ok {
		d.hss[s.Index] = make(map[PacketHandler]bool)
	}
	d.hss[s.Index][h] = true
	d.mh.Unlock()

	// Connect
	d.n.Connector().Connect(h.NodeConnector())
	return nil
}

func (d *Demuxer) Disconnect(h PacketHandler) {
	// Disconnect
	d.n.Connector().Disconnect(h.NodeConnector())

	// Hanlder's streams are unlinked using the "child.removed" event since developer may use
	// other ways to disconnect nodes (e.g. through group)
}

func (d *Demuxer) Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	tc().Do(func() {
		// Make sure to cancel context when we leave this function (e.g. eof or custom read frame error)
		defer cancel()

		// Watch context in a goroutine
		go func() {
			// Wait for context to be done
			<-ctx.Done()

			// Interrupt
			d.ii.Interrupt()
		}()

		// Flush
		if d.flushOnStart {
			if err := d.r.Flush(); err != nil {
				dispatchError(d.n, fmt.Errorf("astiavflow: flushing failed: %w", err).Error())
			}
		}

		// Loop
		for {
			// Read frame
			if stop := d.readFrame(ctx); stop {
				break
			}

			// Check context
			if ctx.Err() != nil {
				break
			}
		}
	})
}

func (d *Demuxer) readFrame(ctx context.Context) bool {
	// Get next packet
	pkt, handle, stop := d.nextPacket()

	// First, make sure packet is properly closed
	defer d.pp.put(pkt)

	// Stop
	if stop {
		return true
	} else if !handle {
		return false
	}

	// Increment stats
	atomic.AddUint64(&d.cs.incomingBytes, uint64(pkt.Size()))
	atomic.AddUint64(&d.cs.incomingPackets, 1)

	// Handle packet
	d.handlePacket(ctx, pkt)
	return false
}

func (d *Demuxer) nextPacket() (pkt *astiav.Packet, handle, stop bool) {
	// Check probe data first
	if len(d.pb.data) > 0 {
		pkt = d.pb.data[0]
		d.pb.data = d.pb.data[1:]
		handle = true
		return
	}

	// Get packet from pool
	pkt = d.pp.get()

	// Read frame
	if err := d.r.ReadFrame(pkt); err != nil {
		if atomic.LoadUint32(&d.l.enabled) > 0 && errors.Is(err, astiav.ErrEof) {
			// Loop
			d.loop()

			// Seek to start
			if err = d.r.SeekFrame(d.l.seekStreamIndex, d.l.seekTimestamp, astiav.NewSeekFlags(astiav.SeekFlagBackward)); err != nil {
				dispatchError(d.n, fmt.Errorf("astiavflow: seeking failed: %w", err).Error())
				stop = true
			}
		} else {
			// Custom error handler
			if d.onReadFrameError != nil {
				var handled bool
				if stop, handled = d.onReadFrameError(d, err); handled {
					return
				}
			}

			// Default error handling
			if !errors.Is(err, astiav.ErrEof) {
				dispatchError(d.n, fmt.Errorf("astiavflow: reading frame failed: %w", err).Error())
			}
			stop = true
		}
		return
	}

	// Packet should be handled
	handle = true
	return
}

func (d *Demuxer) loop() {
	// This is the first time it's looping
	if d.l.cycleCount == 0 {
		// Loop through streams
		var seekPTS *int64
		var seekStream *demuxerStream
		d.ms.Lock()
		for _, s := range d.ss {
			// No first packet pts
			if s.l.cycleFirstPacketPTS == nil {
				continue
			}

			// Get duration
			// Since we can't get more precise than nanoseconds, if there's precision loss here, there's nothing
			// we can do about it
			ld := s.l.cycleLastPacketDuration + time.Duration(astiav.RescaleQ(s.l.cycleLastPacketPTS-*s.l.cycleFirstPacketPTS, s.s.MediaDescriptor.TimeBase, NanosecondRational)) - s.l.cycleFirstPacketPTSRemainder

			// Update loop cycle duration
			if d.l.cycleDuration < ld {
				d.l.cycleDuration = ld
			}

			// Update seek information
			pts := astiav.RescaleQ(*s.l.cycleFirstPacketPTS, s.s.MediaDescriptor.TimeBase, NanosecondRational)
			if seekPTS == nil || pts < *seekPTS {
				seekStream = s
				seekPTS = &pts
			}
		}
		d.ms.Unlock()

		// Update seek information
		d.l.seekStreamIndex = -1
		d.l.seekTimestamp = d.r.StartTime()
		if seekStream != nil && seekPTS != nil {
			d.l.seekStreamIndex = seekStream.s.Index
			d.l.seekTimestamp = *seekStream.l.cycleFirstPacketPTS
		}
	}

	// Increment loop cycle count
	d.l.cycleCount++
}

func (d *Demuxer) handlePacket(ctx context.Context, pkt *astiav.Packet) {
	// Get stream
	d.ms.Lock()
	s, ok := d.ss[pkt.StreamIndex()]
	d.ms.Unlock()
	if !ok {
		return
	}

	// Timestamps are valid
	if pkt.Dts() != astiav.NoPtsValue && pkt.Pts() != astiav.NoPtsValue {
		// Process packet duration
		// Do it before processing side data
		// Since we can't get more precise than nanoseconds, if there's precision loss here, there's nothing
		// we can do about it
		if d.l.cycleCount == 0 {
			s.l.cycleLastPacketDuration = time.Duration(astiav.RescaleQ(pkt.Duration(), s.s.MediaDescriptor.TimeBase, NanosecondRational))
		}

		// Process packet side data
		skippedStart, skippedEnd := d.processPacketSideData(pkt, s)

		// Skipped start
		var skippedStartRemainder time.Duration
		if skippedStart > 0 {
			// Get duration
			var skippedStartDuration int64
			skippedStartDuration, skippedStartRemainder = durationToTimeBase(skippedStart, s.s.MediaDescriptor.TimeBase)

			// Restamp
			pkt.SetDts(pkt.Dts() + skippedStartDuration)
			pkt.SetPts(pkt.Pts() + skippedStartDuration)

			// Store remainder
			if d.l.cycleCount == 0 {
				// Only frames with PTS >= 0 get out of decoders
				if s.l.cycleFirstPacketPTS == nil && pkt.Pts() >= 0 {
					s.l.cycleFirstPacketPTSRemainder = skippedStartRemainder
				}
			}
		}

		// Skipped end
		if skippedEnd > 0 {
			if d.l.cycleCount == 0 {
				s.l.cycleLastPacketDuration -= skippedEnd
			}
		}

		// Process packet pts
		// Do it after processing side data
		if d.l.cycleCount == 0 {
			// Only frames with PTS >= 0 get out of decoders
			if s.l.cycleFirstPacketPTS == nil && pkt.Pts() >= 0 {
				s.l.cycleFirstPacketPTS = astikit.Int64Ptr(pkt.Pts())
			}
			s.l.cycleLastPacketPTS = pkt.Pts()
		}

		// Loop restamp
		if atomic.LoadUint32(&d.l.enabled) > 0 && d.l.cycleCount > 0 {
			// Get duration
			var dl int64
			dl, s.l.restampRemainder = durationToTimeBase(time.Duration(d.l.cycleCount)*d.l.cycleDuration+s.l.restampRemainder+skippedStartRemainder, s.s.MediaDescriptor.TimeBase)

			// Restamp
			pkt.SetDts(pkt.Dts() + dl)
			pkt.SetPts(pkt.Pts() + dl)
		}

		// Control packet rate
		if d.prc != nil {
			d.prc.ControlPacketRate(ctx, pkt, s.s.PacketDescriptor)
		}
	}

	// Check context since it may have been canceled during packet rate control
	if ctx.Err() != nil {
		return
	}

	// Dispatch packet
	d.pd.dispatch(Packet{
		Packet:           pkt,
		PacketDescriptor: s.s.PacketDescriptor,
	}, func(h PacketHandler) (skip bool) {
		d.mh.Lock()
		defer d.mh.Unlock()
		if hs, ok := d.hss[s.s.Index]; ok {
			return !hs[h]
		}
		return true
	})
}

func (o DemuxerStartOptions) loop() *demuxerLoop {
	return &demuxerLoop{enabled: astikit.BoolToUInt32(o.Loop)}
}

type demuxerLoop struct {
	// Number of time it has looped
	cycleCount uint
	// Duration of one loop cycle
	cycleDuration   time.Duration
	enabled         uint32
	seekStreamIndex int
	seekTimestamp   int64
}

type demuxerProbe struct {
	data []*astiav.Packet
	info *DemuxerProbe
}

func newDemuxerProbe() *demuxerProbe {
	return &demuxerProbe{}
}

type DemuxerProbe struct {
	FirstPTS DemuxerProbeFirstPTS
}

type DemuxerProbeFirstPTS struct {
	// Streams whose first pts is the same as the overall first pts.
	// Indexed by stream index
	streams  map[int]bool
	Timebase astiav.Rational
	Value    int64
}

func (fp DemuxerProbeFirstPTS) IsStream(s Stream) bool {
	_, ok := fp.streams[s.Index]
	return ok
}

func (i DemuxerProbe) TimeReference() *TimeReference {
	return NewTimeReference().Update(i.FirstPTS.Value, astikit.Now(), i.FirstPTS.Timebase)
}

type demuxerReader interface {
	Class() *astiav.Class
	CloseInput()
	FindStreamInfo(d *astiav.Dictionary) error
	Flags() astiav.FormatContextFlags
	Flush() error
	Free()
	OpenInput(url string, fmt *astiav.InputFormat, d *astiav.Dictionary) error
	Pb() *astiav.IOContext
	ReadFrame(p *astiav.Packet) error
	SeekFrame(streamIndex int, timestamp int64, f astiav.SeekFlags) error
	SetFlags(f astiav.FormatContextFlags)
	SetPb(i *astiav.IOContext)
	SetIOInterrupter(*astiav.IOInterrupter)
	StartTime() int64
	Streams() []*astiav.Stream
}

var newDemuxerReader = func() demuxerReader {
	return astiav.AllocFormatContext()
}

type demuxerStream struct {
	l *demuxerStreamLoop
	s Stream
}

func (d *Demuxer) newDemuxerStream(s *astiav.Stream) *demuxerStream {
	return &demuxerStream{
		l: newDemuxerStreamLoop(),
		s: newStream(s),
	}
}

type demuxerStreamLoop struct {
	cycleFirstPacketPTS          *int64
	cycleFirstPacketPTSRemainder time.Duration
	cycleLastPacketDuration      time.Duration
	cycleLastPacketPTS           int64
	restampRemainder             time.Duration
}

func newDemuxerStreamLoop() *demuxerStreamLoop {
	return &demuxerStreamLoop{}
}
