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
	er               *demuxerEmulateRate
	flushOnStart     bool
	ii               astiav.IOInterrupter
	l                *demuxerLoop
	ms               sync.Mutex // Locks ss
	n                *astiflow.Node
	onReadFrameError DemuxerReadFrameErrorHandler
	pa               *demuxerPause
	pb               *demuxerProbe
	pd               *packetDispatcher
	pp               *packetPool
	r                demuxerReader
	ss               map[int]*demuxerStream // Indexed by stream index
	startCtx         context.Context
}

type DemuxerOptions struct {
	Group    *astiflow.Group
	Metadata astiflow.Metadata
	Start    DemuxerStartOptions
	Stop     *astiflow.NodeStopOptions
}

type DemuxerStartOptions struct {
	// Demuxer will start by dispatching without sleeping all packets with negative PTS
	// followed by n seconds of packets so that next nodes (e.g. the decoder) have a sufficient
	// buffer to do their work properly (e.g. decoders don't output frames until their PTS is
	// positive). After that, Demuxer sleeps between packets based on their DTS.
	EmulateRate *DemuxerEmulateRateOptions
	// If true, flushes internal data on start
	Flush bool
	// Demuxer will seek back to the start of the input when eof is reached
	// In this case the packets are restamped
	Loop bool
	// Custom read frame error handler
	// If handled is false, default error handling will be executed
	OnReadFrameError DemuxerReadFrameErrorHandler
}

type DemuxerEmulateRateOptions struct {
	// BufferDuration represents the duration of packets with positive PTS dispatched at the
	// start without sleeping.
	// Defaults to 0
	BufferDuration time.Duration
}

type DemuxerReadFrameErrorHandler func(d *Demuxer, err error) (stop, handled bool)

func NewDemuxer(o DemuxerOptions) (d *Demuxer, err error) {
	// Create demuxer
	d = &Demuxer{
		cs:               &demuxerCumulativeStats{},
		er:               o.Start.emulateRate(),
		flushOnStart:     o.Start.Flush,
		l:                o.Start.loop(),
		onReadFrameError: o.Start.OnReadFrameError,
		pa:               newDemuxerPause(),
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

	// Make sure to remove packet dispatcher's skipper when child is removed
	d.n.On(astiflow.EventNameNodeChildRemoved, func(payload interface{}) (delete bool) {
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

		// Delete skipper
		d.pd.delSkipper(h)
		return
	})

	// Make sure to close pause
	d.c.Add(d.pa.close)

	// Create new reader
	r := newDemuxerReader()
	d.c.Add(r.Free)

	// Store reader
	d.r = r
	classers.set(r, d.n)
	d.c.Add(func() { classers.del(r) })

	// Set interrupt callback
	d.ii = d.r.SetInterruptCallback()
	return
}

type DemuxerOpenOptions struct {
	Dictionary DictionaryOptions
	Format     *astiav.InputFormat
	IOContext  *astiav.IOContext
	// In order to emulate rate or loop properly, Demuxer needs to probe data.
	// ProbeDuration represents the duration the Demuxer will probe.
	// Defaults to 1s
	ProbeDuration time.Duration
	URL           string
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
	d.c.Add(d.r.CloseInput)

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
		d.c.Add(func() { classers.del(pb) })
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

	// If either emulate rate or loop is enabled we need to probe the input
	if d.er.enabled || atomic.LoadUint32(&d.l.enabled) > 0 {
		// Probe
		if err = d.probe(o.ProbeDuration); err != nil {
			err = fmt.Errorf("astiavflow: probing failed: %w", err)
			return
		}
	}
	return
}

// Probes the starting packets of a duration equivalent to probeDuration to retrieve
// the first overall PTS and the streams whose first PTS is the same as the first
// overall PTS
// We don't stop before probeDuration in case the smallest PTS is not in the
// first packet.
func (d *Demuxer) probe(pd time.Duration) (err error) {
	// Probe duration default
	if pd <= 0 {
		pd = time.Second
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
	sd := pkt.SideData().Get(astiav.PacketSideDataTypeSkipSamples)
	if sd == nil {
		return
	}

	// Get number of skipped samples
	skippedSamplesStart, skippedSamplesEnd := astiav.RL32WithOffset(sd, 0), astiav.RL32WithOffset(sd, 4)

	// Skipped start
	if skippedSamplesStart > 0 {
		skippedStart = time.Duration(float64(skippedSamplesStart) / float64(sampleRate) * float64(1e9))
	}

	// Skipped end
	if skippedSamplesEnd > 0 {
		skippedEnd = time.Duration(float64(skippedSamplesEnd) / float64(sampleRate) * float64(1e9))
	}
	return
}

func (d *Demuxer) Probe() *DemuxerProbe {
	return d.pb.info
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

func (d *Demuxer) Pause() {
	d.pa.pause(d.startCtx)
}

func (d *Demuxer) Paused() bool {
	return d.pa.paused()
}

func (d *Demuxer) Resume() {
	d.pa.resume(func(dr time.Duration) {
		if d.er.timeReference != nil {
			d.er.timeReference.AddToTime(dr)
		}
	})
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

func (d *Demuxer) On(n astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return d.n.On(n, h)
}

func (d *Demuxer) Connect(h PacketHandler, s Stream) error {
	// Since that would overwrite previous skipper, a handler shouldn't be connected
	// to more that one stream at once.
	var found bool
	for _, c := range d.n.Children() {
		if n, ok := h.(astiflow.Noder); ok && c.Noder() == n {
			found = true
			break
		}
	}
	if found {
		return errors.New("astiavflow: handler shouldn't be connected to more than one stream at once")
	}

	// Callback
	if err := h.OnConnect(s.PacketDescriptor, d); err != nil {
		return fmt.Errorf("astiavflow: callback failed: %w", err)
	}

	// Set skipper
	d.pd.setSkipper(h, func(p Packet) (skip bool) { return p.StreamIndex() != s.Index })

	// Connect
	d.n.Connector().Connect(h.NodeConnector())
	return nil
}

func (d *Demuxer) Disconnect(h PacketHandler) {
	// Disconnect
	d.n.Connector().Disconnect(h.NodeConnector())

	// Packet dispatcher's skipper is removed using the "child.removed" event since developer may use
	// other ways to disconnect nodes (e.g. through group)
}

func (d *Demuxer) Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	tc().Do(func() {
		// Make sure to cancel context when we leave this function (e.g. eof or custom read frame error)
		defer cancel()

		// Update start context
		d.startCtx = ctx

		// Watch context in a goroutine
		go func() {
			// Wait for context to be done
			<-ctx.Done()

			// Interrupt
			d.ii.Interrupt()
		}()

		// Update emulate rate time reference
		if d.er.enabled && d.pb.info != nil {
			d.er.timeReference = d.pb.info.TimeReference()
		}

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

			// Handle pause
			d.pa.wait()

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

			// Get seek information
			seekStreamIdx := -1
			seekTimestamp := d.r.StartTime()
			if d.pb.info != nil {
				// Loop through streams
				for idx := range d.pb.info.FirstPTS.streams {
					// Get stream
					d.ms.Lock()
					s, ok := d.ss[idx]
					d.ms.Unlock()
					if !ok {
						continue
					}

					// Update
					seekStreamIdx = s.s.Index
					seekTimestamp = astiav.RescaleQ(d.pb.info.FirstPTS.Value, d.pb.info.FirstPTS.Timebase, s.s.MediaDescriptor.TimeBase)
					break
				}
			}

			// Seek to start
			if err = d.r.SeekFrame(seekStreamIdx, seekTimestamp, astiav.NewSeekFlags(astiav.SeekFlagBackward)); err != nil {
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
		}
		d.ms.Unlock()
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

		// Emulate rate
		if d.er.enabled {
			// Get pkt at
			pktAt := d.er.timeReference.TimeFromTimestamp(pkt.Dts(), s.s.MediaDescriptor.TimeBase).Add(-d.er.bufferDuration)

			// Wait if there are too many packets in the emulate rate buffer
			if delta := pktAt.Sub(astikit.Now()); delta > 0 {
				astikit.Sleep(ctx, delta)
			}
		}
	}

	// Dispatch packet
	d.pd.dispatch(Packet{
		Packet:           pkt,
		PacketDescriptor: s.s.PacketDescriptor,
	})
}

func (o DemuxerStartOptions) emulateRate() *demuxerEmulateRate {
	r := &demuxerEmulateRate{}
	if o.EmulateRate != nil {
		r.bufferDuration = o.EmulateRate.BufferDuration
		if r.bufferDuration <= 0 {
			r.bufferDuration = 0
		}
		r.enabled = true
	}
	return r
}

type demuxerEmulateRate struct {
	bufferDuration time.Duration
	enabled        bool
	timeReference  *TimeReference
}

func (o DemuxerStartOptions) loop() *demuxerLoop {
	return &demuxerLoop{enabled: astikit.BoolToUInt32(o.Loop)}
}

type demuxerLoop struct {
	// Number of time it has looped
	cycleCount uint
	// Duration of one loop cycle
	cycleDuration time.Duration
	enabled       uint32
}

type demuxerPause struct {
	at     time.Time
	cancel context.CancelFunc
	ctx    context.Context
	m      sync.Mutex
}

func newDemuxerPause() *demuxerPause {
	return &demuxerPause{}
}

func (p *demuxerPause) close() {
	// Lock
	p.m.Lock()
	defer p.m.Unlock()

	// Make sure to cancel context
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *demuxerPause) pause(ctx context.Context) {
	// Lock
	p.m.Lock()
	defer p.m.Unlock()

	// Already paused
	if p.ctx != nil {
		return
	}

	// Create context
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Store at
	p.at = astikit.Now()
}

func (p *demuxerPause) resume(f func(time.Duration)) {
	// Lock
	p.m.Lock()
	defer p.m.Unlock()

	// Not paused
	if p.cancel == nil {
		return
	}

	// Callback
	if f != nil {
		f(astikit.Now().Sub(p.at))
	}

	// Cancel context
	p.cancel()

	// Reset
	p.at = time.Time{}
	p.cancel = nil
	p.ctx = nil
}

func (p *demuxerPause) paused() bool {
	p.m.Lock()
	defer p.m.Unlock()
	return p.ctx != nil
}

func (p *demuxerPause) wait() {
	// Get context
	p.m.Lock()
	ctx := p.ctx
	p.m.Unlock()

	// Wait
	if ctx != nil {
		<-ctx.Done()
	}
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
	SetInterruptCallback() astiav.IOInterrupter
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
