package astiavflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

var (
	_ FrameHandler         = (*FrameRateEmulator)(nil)
	_ PacketRateController = (*FrameRateEmulator)(nil)
)

var (
	countFrameRateEmulator uint64
)

type FrameRateEmulator struct {
	*frameHandler
	fd  *frameDispatcher
	hns map[astiflow.Noder]map[FrameHandler]bool
	mh  sync.Mutex // Locks hns
	re  *rateEmulator
}

type FrameRateEmulatorOptions struct {
	BufferDuration time.Duration
	FlushOnStop    bool
	Group          *astiflow.Group
	Metadata       astiflow.Metadata
	Stop           *astiflow.NodeStopOptions
	TimeReference  *TimeReference
}

func NewFrameRateEmulator(o FrameRateEmulatorOptions) (e *FrameRateEmulator, err error) {
	// Create frame rate emulator
	e = &FrameRateEmulator{
		frameHandler: newFrameHandler(),
		fd:           newFrameDispatcher(),
		hns:          make(map[astiflow.Noder]map[FrameHandler]bool),
		re: newRateEmulator(o.BufferDuration, o.FlushOnStop, func(p *astiav.Packet, pd PacketDescriptor) int64 {
			return astiav.RescaleQ(p.Pts(), pd.MediaDescriptor.TimeBase, NanosecondRational)
		}, o.TimeReference),
	}

	// Create node
	if e.n, e.c, err = o.Group.NewNode(astiflow.NodeOptions{
		Metadata: (&astiflow.Metadata{
			Name: fmt.Sprintf("frame_rate_emulator_%d", atomic.AddUint64(&countFrameRateEmulator, uint64(1))),
			Tags: []string{"frame_rate_emulator"},
		}).Merge(o.Metadata),
		Noder: e,
		Stop:  o.Stop,
	}); err != nil {
		err = fmt.Errorf("astiavflow: creating node failed: %w", err)
		return
	}

	// Initialize dispatchers, handlers and rate emulator
	e.frameHandler.init(frameHandlerInitOptions{
		c:       e.c,
		n:       e.n,
		onFrame: e.onFrame,
	})
	e.fd.init(e.n)
	e.re.init(e.c)

	// Make sure to unlink handler from its noders when child is removed
	e.n.On(astiflow.EventNameNodeChildRemoved, func(payload interface{}) (remove bool) {
		// Assert payload
		n, ok := payload.(*astiflow.Node)
		if !ok {
			return
		}

		// Assert noder
		h, ok := n.Noder().(FrameHandler)
		if !ok {
			return
		}

		// Unlink handler from its noders
		e.mh.Lock()
		for k := range e.hns {
			delete(e.hns[k], h)
			if len(e.hns[k]) == 0 {
				delete(e.hns, k)
			}
		}
		e.mh.Unlock()
		return
	})
	return
}

type FrameRateEmulatorCumulativeStats struct {
	FrameHandlerCumulativeStats
	OutgoingFrames uint64
}

func (e *FrameRateEmulator) CumulativeStats() FrameRateEmulatorCumulativeStats {
	return FrameRateEmulatorCumulativeStats{
		FrameHandlerCumulativeStats: e.frameHandler.cumulativeStats(),
		OutgoingFrames:              atomic.LoadUint64(&e.fd.cs.outgoingFrames),
	}
}

func (e *FrameRateEmulator) DeltaStats() []astikit.DeltaStat {
	ss := e.frameHandler.deltaStats()
	ss = append(ss, e.fd.deltaStats()...)
	return ss
}

func (e *FrameRateEmulator) On(n astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return e.n.On(n, h)
}

func (e *FrameRateEmulator) Connect(h FrameHandler, n astiflow.Noder) error {
	// Get initial incoming packet descriptor
	d, ok := e.initialIncomingFrameDescriptors.get(n)
	if !ok {
		return errors.New("astiavflow: source noder should be connected before connecting destination handler")
	}

	// Callback
	if err := h.OnConnect(d, e); err != nil {
		return fmt.Errorf("astiavflow: callback failed: %w", err)
	}

	// Link noder to handler
	e.mh.Lock()
	if _, ok := e.hns[n]; !ok {
		e.hns[n] = make(map[FrameHandler]bool)
	}
	e.hns[n][h] = true
	e.mh.Unlock()

	// Connect
	e.n.Connector().Connect(h.NodeConnector())
	return nil
}

func (e *FrameRateEmulator) Disconnect(h FrameHandler) {
	// Disconnect
	e.n.Connector().Disconnect(h.NodeConnector())

	// Handler's noders are unlinked using the "child.removed" event since developer may use other ways
	// to disconnect nodes (e.g. through group)
}

func (e *FrameRateEmulator) Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	// Start rate emulator
	tc().Do(func() { e.re.start(ctx) })

	// Start frame handler
	tc().Do(func() { e.frameHandler.start(ctx) })
}

func (e *FrameRateEmulator) onFrame(acquireFrameFunc frameHandlerAcquireFrameFunc, f *astiav.Frame, fd FrameDescriptor, n astiflow.Noder) {
	// Create item
	i := newFrameRateEmulatorItem(func() {
		e.fd.dispatch(Frame{
			Frame:           f,
			FrameDescriptor: fd,
		}, func(h FrameHandler) (skip bool) {
			e.mh.Lock()
			defer e.mh.Unlock()
			if hs, ok := e.hns[n]; ok {
				return !hs[h]
			}
			return true
		})
	}, f, fd)
	i.onPushedFunc = func() { i.releaseFunc = acquireFrameFunc() }

	// Push in rate emulator
	e.re.push(i)
}

func (e *FrameRateEmulator) FlushOnStop(v bool) {
	e.re.setFlushOnStop(v)
}

func (e *FrameRateEmulator) Pause() {
	e.re.pause()
}

func (e *FrameRateEmulator) Paused() bool {
	return e.re.paused()
}

func (e *FrameRateEmulator) Resume() {
	e.re.resume()
}

func (e *FrameRateEmulator) ControlPacketRate(ctx context.Context, p *astiav.Packet, pd PacketDescriptor) {
	e.re.controlPacketRate(ctx, p, pd)
}

var _ rateEmulatorItemer = (*frameRateEmulatorItem)(nil)

type frameRateEmulatorItem struct {
	dispatchFunc frameRateEmulatorItemDispatchFunc
	f            *astiav.Frame
	fd           FrameDescriptor
	onPushedFunc func()
	releaseFunc  frameHandlerReleaseFrameFunc
	ts           int64
}

type frameRateEmulatorItemDispatchFunc func()

func newFrameRateEmulatorItem(dispatchFunc frameRateEmulatorItemDispatchFunc, f *astiav.Frame, fd FrameDescriptor) *frameRateEmulatorItem {
	return &frameRateEmulatorItem{
		dispatchFunc: dispatchFunc,
		f:            f,
		fd:           fd,
		ts:           astiav.RescaleQ(f.Pts(), fd.MediaDescriptor.TimeBase, NanosecondRational),
	}
}

func (i *frameRateEmulatorItem) dispatch(nanosecondRationalTimestampOffset int64) {
	// Make sure to release
	defer i.releaseFunc()

	// Restamp frame
	if nanosecondRationalTimestampOffset > 0 {
		i.f.SetPts(i.f.Pts() + astiav.RescaleQ(nanosecondRationalTimestampOffset, NanosecondRational, i.fd.MediaDescriptor.TimeBase))
	}

	// Dispatch
	i.dispatchFunc()
}

func (i *frameRateEmulatorItem) onPushed() {
	i.onPushedFunc()
}

func (i *frameRateEmulatorItem) nanosecondRationalTimestamp() int64 {
	return i.ts
}
