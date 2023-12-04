package astiavflow

import (
	"context"
	"sync"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
)

type PacketRateController interface {
	ControlPacketRate(ctx context.Context, p *astiav.Packet, pd PacketDescriptor)
}

// TODO Test
type rateEmulator struct {
	bufferDuration                        time.Duration
	flushOnStop                           bool
	items                                 []rateEmulatorItemer
	m                                     sync.Mutex // Locks items, nextAt, preStartItems, reloadChan and startContext
	mt                                    sync.Mutex // Locks timeReference
	nextAt                                time.Time
	p                                     *rateEmulatorPause
	packetNanosecondRationalTimestampFunc rateEmulatorPacketNanosecondRationalTimestampFunc
	preStartItems                         []rateEmulatorItemer
	reloadChan                            chan bool
	startContext                          context.Context
	timeReference                         *TimeReference
}

type rateEmulatorItemer interface {
	dispatch(nanosecondRationalTimestampOffset int64)
	onPushed()
	nanosecondRationalTimestamp() int64
}

type rateEmulatorPacketNanosecondRationalTimestampFunc func(p *astiav.Packet, pd PacketDescriptor) int64

func newRateEmulator(bufferDuration time.Duration, flushOnStop bool, packetNanosecondRationalTimestampFunc rateEmulatorPacketNanosecondRationalTimestampFunc, timeReference *TimeReference) *rateEmulator {
	// Create rate emulator
	e := &rateEmulator{
		bufferDuration:                        bufferDuration,
		flushOnStop:                           flushOnStop,
		p:                                     newRateEmulatorPause(),
		packetNanosecondRationalTimestampFunc: packetNanosecondRationalTimestampFunc,
		timeReference:                         timeReference,
	}

	// Default buffer duration
	if e.bufferDuration <= 0 {
		e.bufferDuration = time.Second
	}
	return e
}

func (e *rateEmulator) init(c *astikit.Closer) {
	// Make sure to close pause
	c.Add(e.p.close)
}

func (e *rateEmulator) setFlushOnStop(v bool) {
	e.flushOnStop = v
}

func (e *rateEmulator) start(ctx context.Context) {
	// Store start context
	e.m.Lock()
	e.startContext = ctx

	// Process pre start items
	for _, i := range e.preStartItems {
		e.pushUnlocked(i)
	}
	e.preStartItems = []rateEmulatorItemer{}
	e.m.Unlock()

	// Loop
	for {
		// Tick
		if stop := e.tick(); stop {
			break
		}
	}

	// Flush
	if e.flushOnStop {
		// Loop
		for {
			// Tick
			if stop := e.flushTick(); stop {
				break
			}
		}
	}
}

func (e *rateEmulator) tick() (stop bool) {
	// Get next at
	e.m.Lock()
	nextAt := e.nextAt

	// Create reload chan
	e.reloadChan = make(chan bool)
	e.m.Unlock()

	// Make sure to close reload chan
	defer func() {
		// Lock
		e.m.Lock()
		defer e.m.Unlock()

		// Close
		if e.reloadChan != nil {
			close(e.reloadChan)
			e.reloadChan = nil
		}
	}()

	// No next at
	if nextAt.IsZero() {
		// Select
		select {
		case <-e.startContext.Done():
			stop = true
			return
		case <-e.reloadChan:
			return
		}
	}

	// Get duration
	d := nextAt.Sub(astikit.Now())

	// Pop immediatly
	if d <= 0 {
		e.pop()
		return
	}

	// Create context
	ctx, cancel := context.WithCancel(e.startContext)
	defer cancel()

	// Cancel context when reload chan has been closed
	go func() {
		defer cancel()
		<-e.reloadChan
	}()

	// Sleep
	astikit.Sleep(ctx, d)

	// Rate emulator has stopped
	if e.startContext.Err() != nil {
		stop = true
		return
	}

	// Context was canceled which means reload chan was closed
	if ctx.Err() != nil {
		return
	}

	// Pop
	e.pop()
	return
}

func (e *rateEmulator) push(i rateEmulatorItemer) {
	// Lock
	e.m.Lock()
	defer e.m.Unlock()

	// Push
	e.pushUnlocked(i)
}

func (e *rateEmulator) pushUnlocked(i rateEmulatorItemer) {
	// Make sure to execute callback if item has been pushed
	var pushed bool
	defer func() {
		if pushed {
			i.onPushed()
		}
	}()

	// Item should be pushed for later
	if e.startContext == nil {
		e.preStartItems = append(e.preStartItems, i)
		pushed = true
		return
	}

	// Rate emulator has been stopped, ignore item
	if e.startContext.Err() != nil {
		return
	}

	// Try to insert item
	var inserted bool
	for idx := range e.items {
		if i.nanosecondRationalTimestamp() < e.items[idx].nanosecondRationalTimestamp() {
			e.items = append(e.items[:idx], append([]rateEmulatorItemer{i}, e.items[idx:]...)...)
			inserted = true
			break
		}
	}

	// No insert was made, we need to append
	if !inserted {
		e.items = append(e.items, i)
	}

	// Update pushed
	pushed = true

	// Get next at
	nextAt := e.mustTimeFromTimestamp(e.items[0].nanosecondRationalTimestamp() + e.p.timestampOffset)

	// Next at hasn't change
	if e.nextAt.Equal(nextAt) {
		return
	}

	// Update next at
	e.nextAt = nextAt

	// Reload
	if e.reloadChan != nil {
		close(e.reloadChan)
		e.reloadChan = nil
	}
}

func (e *rateEmulator) pop() {
	// No items
	e.m.Lock()
	if len(e.items) == 0 {
		e.m.Unlock()
		return
	}

	// Get first item
	i := e.items[0]

	// Remove first item
	if len(e.items) > 1 {
		// Remove from slice
		e.items = e.items[1:]

		// Get next at
		e.nextAt = e.mustTimeFromTimestamp(e.items[0].nanosecondRationalTimestamp() + e.p.timestampOffset)
	} else {
		// Clear
		e.items = []rateEmulatorItemer{}
		e.nextAt = time.Time{}
	}
	e.m.Unlock()

	// Handle pause
	e.p.wait(nil) //nolint: all

	// Dispatch
	i.dispatch(e.p.timestampOffset)
}

func (e *rateEmulator) flushTick() (stop bool) {
	// Get next at
	e.m.Lock()
	nextAt := e.nextAt
	e.m.Unlock()

	// No next at
	if nextAt.IsZero() {
		stop = true
		return
	}

	// Get duration
	d := nextAt.Sub(astikit.Now())

	// Pop immediatly
	if d <= 0 {
		e.pop()
		return
	}

	// Sleep
	astikit.Sleep(context.Background(), d)

	// Pop
	e.pop()
	return
}

func (e *rateEmulator) controlPacketRate(parentCtx context.Context, p *astiav.Packet, pd PacketDescriptor) {
	// Get start context
	e.m.Lock()
	startContext := e.startContext
	e.m.Unlock()

	// Get pkt at
	pktAt := e.mustTimeFromTimestamp(e.packetNanosecondRationalTimestampFunc(p, pd) + e.p.timestampOffset).Add(-e.bufferDuration)

	// Wait
	if delta := pktAt.Sub(astikit.Now()); delta > 0 {
		// Sleep
		if startContext != nil && parentCtx != nil {
			astikit.Sleep(parentCtx, delta, startContext)
		} else if startContext != nil {
			astikit.Sleep(startContext, delta)
		} else if parentCtx != nil {
			astikit.Sleep(parentCtx, delta)
		} else {
			astikit.Sleep(context.Background(), delta)
		}
	}

	// Handle pause
	e.p.wait(parentCtx)
}

func (e *rateEmulator) mustTimeReference(timestamp int64) *TimeReference {
	// Lock
	e.mt.Lock()
	defer e.mt.Unlock()

	// Make sure time reference exists
	if e.timeReference == nil {
		e.timeReference = NewTimeReference().Update(timestamp, astikit.Now(), NanosecondRational)
	}
	return e.timeReference
}

func (e *rateEmulator) mustTimeFromTimestamp(timestamp int64) time.Time {
	return e.mustTimeReference(timestamp).TimeFromTimestamp(timestamp, NanosecondRational)
}

func (e *rateEmulator) pause() {
	e.p.pause(e.startContext)
}

func (e *rateEmulator) paused() bool {
	return e.p.paused()
}

func (e *rateEmulator) resume() {
	e.p.resume()
}

type rateEmulatorPause struct {
	at              time.Time
	cancel          context.CancelFunc
	ctx             context.Context
	m               sync.Mutex
	timestampOffset int64
}

func newRateEmulatorPause() *rateEmulatorPause {
	return &rateEmulatorPause{}
}

func (p *rateEmulatorPause) close() {
	// Lock
	p.m.Lock()
	defer p.m.Unlock()

	// Make sure to cancel context
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *rateEmulatorPause) pause(ctx context.Context) {
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

func (p *rateEmulatorPause) resume() {
	// Lock
	p.m.Lock()
	defer p.m.Unlock()

	// Not paused
	if p.cancel == nil {
		return
	}

	// Update timestamp offset
	p.timestampOffset += int64(astikit.Now().Sub(p.at))

	// Cancel context
	p.cancel()

	// Reset
	p.at = time.Time{}
	p.cancel = nil
	p.ctx = nil
}

func (p *rateEmulatorPause) paused() bool {
	p.m.Lock()
	defer p.m.Unlock()
	return p.ctx != nil
}

func (p *rateEmulatorPause) wait(parentCtx context.Context) {
	// Get context
	p.m.Lock()
	ctx := p.ctx
	p.m.Unlock()

	// No context
	if ctx == nil {
		return
	}

	// Wait
	if parentCtx != nil {
		select {
		case <-ctx.Done():
		case <-parentCtx.Done():
		}
	} else {
		<-ctx.Done()
	}
}
