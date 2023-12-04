package astiavflow

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestPacketPool(t *testing.T) {
	c := astikit.NewCloser()
	pp := newPacketPool().init(c)

	dss := pp.deltaStats()

	pkt1 := pp.get()
	require.Len(t, pp.ps, 0)
	require.NotNil(t, pkt1)
	require.NoError(t, pkt1.FromData([]byte("test")))
	requireDeltaStats(t, map[string]interface{}{DeltaStatNameAllocatedPackets: uint64(1)}, dss)

	pkt2 := pp.get()
	require.Len(t, pp.ps, 0)
	require.NotNil(t, pkt2)
	require.NotSame(t, pkt1, pkt2)
	requireDeltaStats(t, map[string]interface{}{DeltaStatNameAllocatedPackets: uint64(2)}, dss)

	pp.put(pkt1)
	require.Len(t, pp.ps, 1)
	require.Empty(t, pkt1.Data())
	requireDeltaStats(t, map[string]interface{}{DeltaStatNameAllocatedPackets: uint64(2)}, dss)

	pkt3 := pp.get()
	require.Len(t, pp.ps, 0)
	require.Equal(t, pkt1, pkt3)
	requireDeltaStats(t, map[string]interface{}{DeltaStatNameAllocatedPackets: uint64(2)}, dss)

	pkt4 := astiav.AllocPacket()
	defer pkt4.Free()
	pkt4.SetDts(8)
	pkt5, err := pp.copy(pkt4)
	require.NoError(t, err)
	require.Equal(t, int64(8), pkt5.Dts())

	c.Close()
	require.Panics(t, func() { pkt1.Data() })

	require.Equal(t, packetPoolCumulativeStats{
		allocatedPackets: 3,
	}, *pp.cs)
}
