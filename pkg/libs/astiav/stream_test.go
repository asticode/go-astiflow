package astiavflow

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/stretchr/testify/require"
)

func TestStream(t *testing.T) {
	fc := astiav.AllocFormatContext()
	defer fc.Free()
	s := fc.NewStream(nil)
	s.SetID(1)
	s.SetIndex(2)
	s.SetTimeBase(astiav.NewRational(3, 4))
	require.Equal(t, Stream{
		ID:    1,
		Index: 2,
		PacketDescriptor: PacketDescriptor{
			CodecParameters: s.CodecParameters(),
			MediaDescriptor: MediaDescriptor{TimeBase: astiav.NewRational(3, 4)},
		},
	}, newStream(s))
}
