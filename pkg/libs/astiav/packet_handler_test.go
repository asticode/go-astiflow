package astiavflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

type mockedPacketHandler struct {
	*mocks.MockedNoder
	onConnect func(d PacketDescriptor, n astiflow.Noder) error
	onPacket  func(p Packet)
}

var _ PacketHandler = (*mockedPacketHandler)(nil)

func newMockedPacketHandler() *mockedPacketHandler {
	return &mockedPacketHandler{
		MockedNoder: mocks.NewMockedNoder(),
	}
}

func (h *mockedPacketHandler) HandlePacket(p Packet) {
	if h.onPacket != nil {
		h.onPacket(p)
	}
}

func (h *mockedPacketHandler) NodeConnector() *astiflow.NodeConnector {
	return h.Node.Connector()
}

func (h *mockedPacketHandler) OnConnect(d PacketDescriptor, n astiflow.Noder) error {
	if h.onConnect != nil {
		return h.onConnect(d, n)
	}
	return nil
}

func TestPacketHandler(t *testing.T) {
	// Connect works properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		nr1 := mocks.NewMockedNoder()
		n1, _, err := g.NewNode(astiflow.NodeOptions{Noder: nr1})
		require.NoError(t, err)
		nr2 := mocks.NewMockedNoder()
		n2, c, err := g.NewNode(astiflow.NodeOptions{Noder: nr2})
		require.NoError(t, err)

		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		cp.SetHeight(1)

		count := 0
		pd := PacketDescriptor{CodecParameters: cp}
		var cn astiflow.Noder
		h := newPacketHandler()
		h.init(packetHandlerInitOptions{
			c: c,
			n: n2,
			onConnect: func(n astiflow.Noder) error {
				count++
				switch count {
				case 1:
					return errors.New("test")
				default:
					cn = n
					return nil
				}
			},
		})

		n1.Connector().Connect(h.NodeConnector())
		require.Error(t, h.OnConnect(pd, nr1))
		require.Len(t, h.cpp.cpp.cps, 1)
		require.Len(t, h.initialIncomingPacketDescriptors.p, 0)
		require.NoError(t, h.OnConnect(pd, nr1))
		require.Equal(t, nr1, cn)
		require.Len(t, h.cpp.cpp.cps, 0)
		v, ok := h.initialIncomingPacketDescriptors.get(nr1)
		require.True(t, ok)
		require.Equal(t, cp.Height(), v.CodecParameters.Height())
		require.Error(t, h.OnConnect(pd, nr1))
		require.Len(t, h.cpp.cpp.cps, 0)
		n1.Connector().Disconnect(h.NodeConnector())
		_, ok = h.initialIncomingPacketDescriptors.get(nr1)
		require.False(t, ok)
		require.Len(t, h.cpp.cpp.cps, 1)
	})

	// Handling packets works properly, stats are correct, packet and descriptor are copied and closed properly
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		nr1 := mocks.NewMockedNoder()
		nr2 := mocks.NewMockedNoder()
		n, c, err := g.NewNode(astiflow.NodeOptions{Noder: nr2})
		require.NoError(t, err)

		cp1 := astiav.AllocCodecParameters()
		defer cp1.Free()
		cp1.SetHeight(1)
		cp2 := astiav.AllocCodecParameters()
		defer cp2.Free()
		cp2.SetHeight(2)

		pkt := astiav.AllocPacket()
		defer pkt.Free()
		require.NoError(t, pkt.AllocPayload(1))
		packetSize := 2
		pkt.SetSize(packetSize)

		h := newPacketHandler()
		count := 0
		type packet struct {
			ccpHeight int
			ccpSame1  bool
			ccpSame2  bool
			n         astiflow.Noder
			pcpHeight int
			pcpSame1  bool
			pcpSame2  bool
			pts       int64
			samePkt   bool
		}
		ps := []packet{}
		h.init(packetHandlerInitOptions{
			c: c,
			n: n,
			onPacket: func(n astiflow.Noder, i *astiav.Packet, pd PacketDescriptor) {
				count++
				p := packet{
					n:       n,
					pts:     i.Pts(),
					samePkt: i == pkt,
				}
				p.ccpHeight = pd.CodecParameters.Height()
				p.ccpSame1 = pd.CodecParameters == cp1
				p.ccpSame2 = pd.CodecParameters == cp2
				if pd, ok := h.previousIncomingPacketDescriptors.get(n); ok {
					p.pcpHeight = pd.CodecParameters.Height()
					p.pcpSame1 = pd.CodecParameters == cp1
					p.pcpSame2 = pd.CodecParameters == cp2
				}
				ps = append(ps, p)
				require.Len(t, h.cpp.cpp.cps, 0)
				if count == 2 {
					require.Len(t, h.pp.ps, 1)
					require.NoError(t, g.Stop())
				}
			},
		})
		nr2.OnStart = func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
			tc().Do(func() { h.start(ctx) })
		}
		dss := h.deltaStats()

		for _, v := range []struct {
			cp  *astiav.CodecParameters
			pts int64
		}{
			{
				cp:  cp1,
				pts: 1,
			},
			{
				cp:  cp2,
				pts: 2,
			},
		} {
			pkt.SetPts(v.pts)
			h.HandlePacket(Packet{
				Packet:           pkt,
				PacketDescriptor: PacketDescriptor{CodecParameters: v.cp},
				Noder:            nr1,
			})
		}

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		cs := h.cumulativeStats()
		require.Greater(t, cs.WorkedDuration, time.Duration(0))
		cs.WorkedDuration = 0
		require.Equal(t, PacketHandlerCumulativeStats{
			AllocatedCodecParameters: 2,
			AllocatedPackets:         2,
			IncomingBytes:            uint64(2 * packetSize),
			IncomingPackets:          2,
			ProcessedBytes:           uint64(2 * packetSize),
			ProcessedPackets:         2,
		}, cs)
		requireDeltaStats(t, map[string]interface{}{
			DeltaStatNameAllocatedCodecParameters:   uint64(2),
			DeltaStatNameAllocatedPackets:           uint64(2),
			astiflow.DeltaStatNameIncomingByteRate:  float64(2 * packetSize),
			astiflow.DeltaStatNameIncomingRate:      float64(2),
			astiflow.DeltaStatNameProcessedByteRate: float64(2 * packetSize),
			astiflow.DeltaStatNameProcessedRate:     float64(2),
			astikit.StatNameWorkedRatio:             nil,
		}, dss)
		require.Equal(t, []packet{
			{
				ccpHeight: 1,
				ccpSame1:  false,
				n:         nr1,
				pts:       1,
				samePkt:   false,
			},
			{
				ccpHeight: 2,
				ccpSame2:  false,
				n:         nr1,
				pcpHeight: 1,
				pcpSame1:  false,
				pts:       2,
				samePkt:   false,
			},
		}, ps)
	})

	// Handling packet is protected from the closer
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		nr1 := mocks.NewMockedNoder()
		n1, c1, err := g.NewNode(astiflow.NodeOptions{Noder: nr1})
		require.NoError(t, err)
		nr2 := mocks.NewMockedNoder()
		n2, c2, err := g.NewNode(astiflow.NodeOptions{Noder: nr2})
		require.NoError(t, err)
		nr3 := mocks.NewMockedNoder()
		n3, c3, err := g.NewNode(astiflow.NodeOptions{Noder: nr3})
		require.NoError(t, err)

		cp := astiav.AllocCodecParameters()
		defer cp.Free()

		pkt := astiav.AllocPacket()
		defer pkt.Free()
		require.NoError(t, pkt.AllocPayload(1))

		h1 := newPacketHandler()
		count1 := 0
		h1.init(packetHandlerInitOptions{
			c:        c1,
			n:        n1,
			onPacket: func(n astiflow.Noder, p *astiav.Packet, pd PacketDescriptor) { count1++ },
		})
		nr1.OnStart = func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
			tc().Do(func() { h1.start(ctx) })
		}
		h2 := newPacketHandler()
		count2 := 0
		h2.init(packetHandlerInitOptions{
			c:        c2,
			n:        n2,
			onPacket: func(n astiflow.Noder, p *astiav.Packet, pd PacketDescriptor) { count2++ },
		})
		nr2.OnStart = func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
			tc().Do(func() { h2.start(ctx) })
		}
		h3 := newPacketHandler()
		count3 := 0
		h3.init(packetHandlerInitOptions{
			c: c3,
			n: n3,
			onPacket: func(n astiflow.Noder, p *astiav.Packet, pd PacketDescriptor) {
				count3++
				require.NoError(t, g.Stop())
			},
		})
		nr3.OnStart = func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
			tc().Do(func() { h3.start(ctx) })
		}

		c1.Close()
		for _, h := range []*packetHandler{h1, h2, h3} {
			h.HandlePacket(Packet{
				Packet:           pkt,
				PacketDescriptor: PacketDescriptor{CodecParameters: cp},
			})
		}

		c2.Close()
		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return n1.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Eventually(t, func() bool { return n2.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Eventually(t, func() bool { return n3.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		require.Equal(t, 0, count1)
		require.Equal(t, 0, count2)
		require.Equal(t, 1, count3)
	})
}
