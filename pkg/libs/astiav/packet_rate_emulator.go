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
	_ PacketHandler        = (*PacketRateEmulator)(nil)
	_ PacketRateController = (*PacketRateEmulator)(nil)
)

var (
	countPacketRateEmulator uint64
)

// TODO Test
type PacketRateEmulator struct {
	*packetHandler
	hns map[astiflow.Noder]map[PacketHandler]bool
	mh  sync.Mutex // Locks hns
	pd  *packetDispatcher
	re  *rateEmulator
}

type PacketRateEmulatorOptions struct {
	BufferDuration time.Duration
	FlushOnStop    bool
	Group          *astiflow.Group
	Metadata       astiflow.Metadata
	Stop           *astiflow.NodeStopOptions
	TimeReference  *TimeReference
}

func NewPacketRateEmulator(o PacketRateEmulatorOptions) (e *PacketRateEmulator, err error) {
	// Create packet rate emulator
	e = &PacketRateEmulator{
		hns:           make(map[astiflow.Noder]map[PacketHandler]bool),
		packetHandler: newPacketHandler(),
		pd:            newPacketDispatcher(),
		re: newRateEmulator(o.BufferDuration, o.FlushOnStop, func(p *astiav.Packet, pd PacketDescriptor) int64 {
			return astiav.RescaleQ(p.Dts(), pd.MediaDescriptor.TimeBase, NanosecondRational)
		}, o.TimeReference),
	}

	// Create node
	if e.n, e.c, err = o.Group.NewNode(astiflow.NodeOptions{
		Metadata: (&astiflow.Metadata{
			Name: fmt.Sprintf("packet_rate_emulator_%d", atomic.AddUint64(&countPacketRateEmulator, uint64(1))),
			Tags: []string{"packet_rate_emulator"},
		}).Merge(o.Metadata),
		Noder: e,
		Stop:  o.Stop,
	}); err != nil {
		err = fmt.Errorf("astiavflow: creating node failed: %w", err)
		return
	}

	// Initialize dispatchers, handlers and rate emulator
	e.packetHandler.init(packetHandlerInitOptions{
		c:        e.c,
		n:        e.n,
		onPacket: e.onPacket,
	})
	e.pd.init(e.n)
	e.re.init(e.c)

	// Make sure to unlink handler from its noders when child is removed
	e.n.On(astiflow.EventNameNodeChildRemoved, func(payload interface{}) (remove bool) {
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

type PacketRateEmulatorCumulativeStats struct {
	PacketHandlerCumulativeStats
	OutgoingPackets uint64
}

func (e *PacketRateEmulator) CumulativeStats() PacketRateEmulatorCumulativeStats {
	return PacketRateEmulatorCumulativeStats{
		PacketHandlerCumulativeStats: e.packetHandler.cumulativeStats(),
		OutgoingPackets:              atomic.LoadUint64(&e.pd.cs.outgoingPackets),
	}
}

func (e *PacketRateEmulator) DeltaStats() []astikit.DeltaStat {
	ss := e.packetHandler.deltaStats()
	ss = append(ss, e.pd.deltaStats()...)
	return ss
}

func (e *PacketRateEmulator) On(n astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return e.n.On(n, h)
}

func (e *PacketRateEmulator) Connect(h PacketHandler, n astiflow.Noder) error {
	// Get initial incoming packet descriptor
	d, ok := e.initialIncomingPacketDescriptors.get(n)
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
		e.hns[n] = make(map[PacketHandler]bool)
	}
	e.hns[n][h] = true
	e.mh.Unlock()

	// Connect
	e.n.Connector().Connect(h.NodeConnector())
	return nil
}

func (e *PacketRateEmulator) Disconnect(h PacketHandler) {
	// Disconnect
	e.n.Connector().Disconnect(h.NodeConnector())

	// Handler's noders are unlinked using the "child.removed" event since developer may use other ways
	// to disconnect nodes (e.g. through group)
}

func (e *PacketRateEmulator) Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	// Start rate emulator
	tc().Do(func() { e.re.start(ctx) })

	// Start packet handler
	tc().Do(func() { e.packetHandler.start(ctx) })
}

func (e *PacketRateEmulator) onPacket(acquirePacketFunc packetHandlerAcquirePacketFunc, p *astiav.Packet, pd PacketDescriptor, n astiflow.Noder) {
	// Create item
	i := newPacketRateEmulatorItem(func() {
		e.pd.dispatch(Packet{
			Packet:           p,
			PacketDescriptor: pd,
		}, func(h PacketHandler) (skip bool) {
			e.mh.Lock()
			defer e.mh.Unlock()
			if hs, ok := e.hns[n]; ok {
				return !hs[h]
			}
			return true
		})
	}, p, pd)
	i.onPushedFunc = func() { i.releaseFunc = acquirePacketFunc() }

	// Push in rate emulator
	e.re.push(i)
}

func (e *PacketRateEmulator) FlushOnStop(v bool) {
	e.re.setFlushOnStop(v)
}

func (e *PacketRateEmulator) Pause() {
	e.re.pause()
}

func (e *PacketRateEmulator) Paused() bool {
	return e.re.paused()
}

func (e *PacketRateEmulator) Resume() {
	e.re.resume()
}

func (e *PacketRateEmulator) ControlPacketRate(ctx context.Context, p *astiav.Packet, pd PacketDescriptor) {
	e.re.controlPacketRate(ctx, p, pd)
}

var _ rateEmulatorItemer = (*packetRateEmulatorItem)(nil)

type packetRateEmulatorItem struct {
	dispatchFunc packetRateEmulatorItemDispatchFunc
	onPushedFunc func()
	p            *astiav.Packet
	pd           PacketDescriptor
	releaseFunc  packetHandlerReleasePacketFunc
	ts           int64
}

type packetRateEmulatorItemDispatchFunc func()

func newPacketRateEmulatorItem(dispatchFunc packetRateEmulatorItemDispatchFunc, p *astiav.Packet, pd PacketDescriptor) *packetRateEmulatorItem {
	return &packetRateEmulatorItem{
		dispatchFunc: dispatchFunc,
		p:            p,
		pd:           pd,
		ts:           astiav.RescaleQ(p.Dts(), pd.MediaDescriptor.TimeBase, NanosecondRational),
	}
}

func (i *packetRateEmulatorItem) dispatch(nanosecondRationalTimestampOffset int64) {
	// Make sure to release
	defer i.releaseFunc()

	// Restamp packet
	if nanosecondRationalTimestampOffset > 0 {
		i.p.SetDts(i.p.Dts() + astiav.RescaleQ(nanosecondRationalTimestampOffset, NanosecondRational, i.pd.MediaDescriptor.TimeBase))
		i.p.SetPts(i.p.Pts() + astiav.RescaleQ(nanosecondRationalTimestampOffset, NanosecondRational, i.pd.MediaDescriptor.TimeBase))
	}

	// Dispatch
	i.dispatchFunc()
}

func (i *packetRateEmulatorItem) onPushed() {
	i.onPushedFunc()
}

func (i *packetRateEmulatorItem) nanosecondRationalTimestamp() int64 {
	return i.ts
}
