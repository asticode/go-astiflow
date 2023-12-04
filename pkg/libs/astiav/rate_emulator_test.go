package astiavflow

import (
	"context"

	"github.com/asticode/go-astiav"
)

var _ PacketRateController = (*mockedPacketRateController)(nil)

type mockedPacketRateController struct {
	onControlPacketRate func(ctx context.Context, p *astiav.Packet, pd PacketDescriptor)
}

func newMockedPacketRateController(onControlPacketRate func(ctx context.Context, p *astiav.Packet, pd PacketDescriptor)) *mockedPacketRateController {
	return &mockedPacketRateController{onControlPacketRate: onControlPacketRate}
}

func (c *mockedPacketRateController) ControlPacketRate(ctx context.Context, p *astiav.Packet, pd PacketDescriptor) {
	if c.onControlPacketRate != nil {
		c.onControlPacketRate(ctx, p, pd)
	}
}

var _ rateEmulatorItemer = (*mockedRateEmulatorItemer)(nil)

type mockedRateEmulatorItemer struct {
	onDispatch                    func(nanosecondRationalTimestampOffset int64)
	onNanosecondRationalTimestamp func() int64
	onPushed_                     func()
}

func newMockedRateEmulatorItemer() *mockedRateEmulatorItemer {
	return &mockedRateEmulatorItemer{}
}

func (i *mockedRateEmulatorItemer) dispatch(nanosecondRationalTimestampOffset int64) {
	if i.onDispatch != nil {
		i.onDispatch(nanosecondRationalTimestampOffset)
	}
}

func (i *mockedRateEmulatorItemer) onPushed() {
	if i.onPushed_ != nil {
		i.onPushed_()
	}
}

func (i *mockedRateEmulatorItemer) nanosecondRationalTimestamp() int64 {
	if i.onNanosecondRationalTimestamp != nil {
		return i.onNanosecondRationalTimestamp()
	}
	return 0
}
