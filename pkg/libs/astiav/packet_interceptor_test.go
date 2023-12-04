package astiavflow

import (
	"errors"
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestNewPacketInterceptor(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		countPacketInterceptor = 0
		i, err := NewPacketInterceptor(PacketInterceptorOptions{Group: g})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{Name: "packet_interceptor_1", Tags: []string{"packet_interceptor"}}, i.n.Metadata())
		var emitted bool
		i.On(astiflow.EventNameNodeClosed, func(payload interface{}) (delete bool) {
			emitted = true
			return
		})
		g.Close()
		require.True(t, emitted)
	})

	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		i, err := NewPacketInterceptor(PacketInterceptorOptions{
			Group:    g,
			Metadata: astiflow.Metadata{Description: "d", Name: "n", Tags: []string{"t"}},
		})
		require.NoError(t, err)
		require.Equal(t, astiflow.Metadata{
			Description: "d",
			Name:        "n",
			Tags:        []string{"packet_interceptor", "t"},
		}, i.n.Metadata())
	})
}

func TestPacketInterceptorOnConnect(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		i, err := NewPacketInterceptor(PacketInterceptorOptions{Group: g})
		require.NoError(t, err)

		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		d := PacketDescriptor{CodecParameters: cp}
		require.NoError(t, i.OnConnect(d, nil))
	})
}

func TestPacketInterceptorConnect(t *testing.T) {
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		h := newMockedPacketHandler()
		var err error
		h.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: h})
		require.NoError(t, err)

		i, err := NewPacketInterceptor(PacketInterceptorOptions{Group: g})
		require.NoError(t, err)

		e := errors.New("test")
		h.onConnect = func(d PacketDescriptor, n astiflow.Noder) error { return e }
		err = i.Connect(h)
		require.Error(t, err)
		require.NotErrorIs(t, err, e)

		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		ad := PacketDescriptor{
			CodecParameters: cp,
			MediaDescriptor: MediaDescriptor{Rotation: 1},
		}
		require.NoError(t, i.OnConnect(ad, nil))
		require.ErrorIs(t, i.Connect(h), e)

		var ed PacketDescriptor
		h.onConnect = func(d PacketDescriptor, n astiflow.Noder) error {
			ed = d
			return nil
		}
		require.NoError(t, i.Connect(h))
		require.Equal(t, 1.0, ed.MediaDescriptor.Rotation)
	})
}

func TestPacketInterceptorStart(t *testing.T) {
	// Callback should be handled properly, stats should be correct
	withGroup(t, func(f *astiflow.Flow, g *astiflow.Group, w *astikit.Worker) {
		type packet struct {
			md  MediaDescriptor
			n   astiflow.Noder
			pts int64
		}
		ps1 := []packet{}
		ph1 := newMockedPacketHandler()
		ph1.onPacket = func(p Packet) {
			ps1 = append(ps1, packet{
				md:  p.MediaDescriptor,
				n:   p.Noder,
				pts: p.Pts(),
			})
		}
		var err error
		ph1.Node, _, err = g.NewNode(astiflow.NodeOptions{
			Noder: ph1,
			Stop:  &astiflow.NodeStopOptions{WhenAllParentsAreDone: true},
		})
		require.NoError(t, err)
		ps2 := []packet{}
		ph2 := newMockedPacketHandler()
		ph2.onPacket = func(p Packet) {
			ps2 = append(ps2, packet{
				md:  p.MediaDescriptor,
				n:   p.Noder,
				pts: p.Pts(),
			})
		}
		ph2.Node, _, err = g.NewNode(astiflow.NodeOptions{Noder: ph2})
		require.NoError(t, err)

		count := 0
		var i *PacketInterceptor
		i, err = NewPacketInterceptor(PacketInterceptorOptions{
			Group: g,
			OnPacket: func(p *astiav.Packet, pd PacketDescriptor) (dispatch bool, err error) {
				count++
				p.SetPts(int64(count))
				switch count {
				case 1:
					return false, nil
				case 2:
					return true, errors.New("test")
				case 3:
					return true, nil
				case 4:
					i.Disconnect(ph2)
					return true, nil
				default:
					require.NoError(t, g.Stop())
					return true, nil
				}
			},
		})
		require.NoError(t, err)

		dss := i.DeltaStats()

		cp := astiav.AllocCodecParameters()
		defer cp.Free()
		cp.SetCodecID(astiav.CodecIDMjpeg)
		md := MediaDescriptor{Rotation: 1}
		pd := PacketDescriptor{
			CodecParameters: cp,
			MediaDescriptor: md,
		}
		pkt := astiav.AllocPacket()
		defer pkt.Free()
		require.NoError(t, pkt.AllocPayload(1))
		packetSize := 2
		pkt.SetSize(packetSize)
		require.NoError(t, i.OnConnect(pd, nil))
		require.NoError(t, i.Connect(ph1))
		require.NoError(t, i.Connect(ph2))

		for idx := 0; idx < 5; idx++ {
			i.HandlePacket(Packet{
				Packet:           pkt,
				PacketDescriptor: pd,
			})
		}

		require.NoError(t, f.Start(w.Context()))
		defer f.Stop() //nolint: errcheck

		require.Eventually(t, func() bool { return i.n.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
		cs := i.CumulativeStats()
		require.Greater(t, cs.WorkedDuration, time.Duration(0))
		cs.WorkedDuration = 0
		require.Equal(t, PacketInterceptorCumulativeStats{
			PacketHandlerCumulativeStats: PacketHandlerCumulativeStats{
				AllocatedCodecParameters: 6,
				AllocatedPackets:         5,
				IncomingBytes:            uint64(5 * packetSize),
				IncomingPackets:          5,
				ProcessedBytes:           uint64(5 * packetSize),
				ProcessedPackets:         5,
			},
			OutgoingPackets: 3,
		}, cs)
		requireDeltaStats(t, map[string]interface{}{
			DeltaStatNameAllocatedCodecParameters:   uint64(6),
			DeltaStatNameAllocatedPackets:           uint64(5),
			astiflow.DeltaStatNameIncomingByteRate:  float64(5 * packetSize),
			astiflow.DeltaStatNameIncomingRate:      float64(5),
			astiflow.DeltaStatNameOutgoingByteRate:  float64(3 * packetSize),
			astiflow.DeltaStatNameOutgoingRate:      float64(3),
			astiflow.DeltaStatNameProcessedByteRate: float64(5 * packetSize),
			astiflow.DeltaStatNameProcessedRate:     float64(5),
			astikit.StatNameWorkedRatio:             nil,
		}, dss)
		require.Equal(t, []packet{
			{
				md:  md,
				n:   i,
				pts: 3,
			},
			{
				md:  md,
				n:   i,
				pts: 4,
			},
			{
				md:  md,
				n:   i,
				pts: 5,
			},
		}, ps1)
		require.Equal(t, []packet{
			{
				md:  md,
				n:   i,
				pts: 3,
			},
		}, ps2)
	})
}
