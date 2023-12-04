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

var _ PacketHandler = (*PacketInterceptor)(nil)

var (
	countPacketInterceptor uint64
)

type PacketInterceptor struct {
	*packetHandler
	c        *astikit.Closer
	n        *astiflow.Node
	onPacket PacketInterceptorOnPacket
	pd       *packetDispatcher
}

type PacketInterceptorOnPacket func(p *astiav.Packet, pd PacketDescriptor) (dispatch bool, err error)

type PacketInterceptorOptions struct {
	Group    *astiflow.Group
	Metadata astiflow.Metadata
	OnPacket PacketInterceptorOnPacket
	Stop     *astiflow.NodeStopOptions
}

func NewPacketInterceptor(o PacketInterceptorOptions) (i *PacketInterceptor, err error) {
	// Create packet interceptor
	i = &PacketInterceptor{
		onPacket:      o.OnPacket,
		packetHandler: newPacketHandler(),
		pd:            newPacketDispatcher(),
	}

	// Make sure on packet is always set
	if i.onPacket == nil {
		i.onPacket = func(p *astiav.Packet, pd PacketDescriptor) (dispatch bool, err error) { return }
	}

	// Create node
	if i.n, i.c, err = o.Group.NewNode(astiflow.NodeOptions{
		Metadata: (&astiflow.Metadata{
			Name: fmt.Sprintf("packet_interceptor_%d", atomic.AddUint64(&countPacketInterceptor, uint64(1))),
			Tags: []string{"packet_interceptor"},
		}).Merge(o.Metadata),
		Noder: i,
		Stop:  o.Stop,
	}); err != nil {
		err = fmt.Errorf("astiavflow: creating node failed: %w", err)
		return
	}

	// Initialize dispatchers and handlers
	i.packetHandler.init(packetHandlerInitOptions{
		c:        i.c,
		n:        i.n,
		onPacket: i.onPacket_,
	})
	i.pd.init(i.n)
	return
}

type PacketInterceptorCumulativeStats struct {
	PacketHandlerCumulativeStats
	OutgoingPackets uint64
}

func (i *PacketInterceptor) CumulativeStats() PacketInterceptorCumulativeStats {
	return PacketInterceptorCumulativeStats{
		PacketHandlerCumulativeStats: i.packetHandler.cumulativeStats(),
		OutgoingPackets:              atomic.LoadUint64(&i.pd.cs.outgoingPackets),
	}
}

func (i *PacketInterceptor) DeltaStats() []astikit.DeltaStat {
	ss := i.packetHandler.deltaStats()
	ss = append(ss, i.pd.deltaStats()...)
	return ss
}

func (i *PacketInterceptor) On(n astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return i.n.On(n, h)
}

func (i *PacketInterceptor) Connect(h PacketHandler) error {
	// Get initial outgoing packet descriptor
	d, err := i.initialOutgoingPacketDescriptor()
	if err != nil {
		return fmt.Errorf("astiavflow: getting initial outgoing packet descriptor failed: %w: packet interceptor needs at least one parent before adding children", err)
	}

	// Callback
	if err := h.OnConnect(d, i); err != nil {
		return fmt.Errorf("astiavflow: callback failed: %w", err)
	}

	// Connect
	i.n.Connector().Connect(h.NodeConnector())
	return nil
}

func (i *PacketInterceptor) initialOutgoingPacketDescriptor() (PacketDescriptor, error) {
	// Get one initial incoming packet descriptor
	d, ok := i.initialIncomingPacketDescriptors.one()
	if !ok {
		return PacketDescriptor{}, errors.New("astiavflow: no initial incoming packet descriptor")
	}
	return d, nil
}

func (i *PacketInterceptor) Disconnect(h PacketHandler) {
	// Disconnect
	i.n.Connector().Disconnect(h.NodeConnector())
}

func (i *PacketInterceptor) Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	tc().Do(func() {
		// Start packet handler
		i.packetHandler.start(ctx)
	})
}

func (i *PacketInterceptor) onPacket_(n astiflow.Noder, p *astiav.Packet, pd PacketDescriptor) {
	// Callback
	dispatch, err := i.onPacket(p, pd)
	if err != nil {
		dispatchError(i.n, fmt.Errorf("astiavflow: callback failed: %w", err).Error())
		return
	}

	// Dispatch packet
	if dispatch {
		i.pd.dispatch(Packet{
			Packet:           p,
			PacketDescriptor: pd,
		})
	}
}
