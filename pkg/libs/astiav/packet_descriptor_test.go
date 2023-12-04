package astiavflow

import (
	"testing"

	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestPacketDescriptorPool(t *testing.T) {
	c := astikit.NewCloser()
	defer c.Close()
	cpp := newCodecParametersPoolWithRereferenceCounter()
	cpp.init(c)
	p := newPacketDescriptorPool(cpp)
	cp1 := cpp.cpp.get()
	d1 := PacketDescriptor{CodecParameters: cp1}
	n1 := mocks.NewMockedNoder()
	cp2 := cpp.cpp.get()
	d2 := PacketDescriptor{CodecParameters: cp2}
	n2 := mocks.NewMockedNoder()
	cp3 := cpp.cpp.get()
	d3 := PacketDescriptor{CodecParameters: cp3}
	p.set(d1, n1)
	p.set(d3, n2)
	require.Len(t, cpp.cpp.cps, 0)
	require.Len(t, cpp.p, 2)
	d, ok := p.get(n1)
	require.True(t, ok)
	require.Equal(t, d1, d)
	d, ok = p.one()
	require.True(t, ok)
	require.Contains(t, []PacketDescriptor{d1, d3}, d)
	p.set(d2, n2)
	require.Len(t, cpp.cpp.cps, 1)
	require.Len(t, cpp.p, 3)
	p.del(n1)
	require.Len(t, cpp.cpp.cps, 2)
	_, ok = p.get(n1)
	require.False(t, ok)
	d, ok = p.one()
	require.True(t, ok)
	require.Equal(t, d2, d)
	p.del(n2)
	require.Len(t, cpp.cpp.cps, 3)
	_, ok = p.one()
	require.False(t, ok)
}
