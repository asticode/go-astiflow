package astiavflow

import (
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
	"github.com/stretchr/testify/require"
)

func TestPacketPool(t *testing.T) {
	c := astikit.NewCloser()
	pp := newPacketPool().init(c)

	dss := pp.deltaStats()
	require.Len(t, dss, 1)
	require.Equal(t, uint64(0), dss[0].Valuer.Value(time.Second))

	pkt1 := pp.get()
	require.Len(t, pp.ps, 0)
	require.NotNil(t, pkt1)
	require.NoError(t, pkt1.FromData([]byte("test")))
	require.Equal(t, uint64(1), dss[0].Valuer.Value(time.Second))

	pkt2 := pp.get()
	require.Len(t, pp.ps, 0)
	require.NotNil(t, pkt2)
	require.NotEqual(t, pkt1, pkt2)
	require.Equal(t, uint64(2), dss[0].Valuer.Value(time.Second))

	pp.put(pkt1)
	require.Len(t, pp.ps, 1)
	require.Empty(t, pkt1.Data())
	require.Equal(t, uint64(2), dss[0].Valuer.Value(time.Second))

	pkt3 := pp.get()
	require.Len(t, pp.ps, 0)
	require.Equal(t, pkt1, pkt3)
	require.Equal(t, uint64(2), dss[0].Valuer.Value(time.Second))

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
	}, *pp.cumulativeStats)
}
