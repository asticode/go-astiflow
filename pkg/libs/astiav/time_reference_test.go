package astiavflow

import (
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/stretchr/testify/require"
)

func TestTimeReference(t *testing.T) {
	r := NewTimeReference()
	require.True(t, r.isZeroUnsafe())
	r.Update(2, time.Unix(1, 0), astiav.NewRational(1, 2))
	require.False(t, r.isZeroUnsafe())
	require.Equal(t, time.Unix(2, 0), r.TimeFromTimestamp(8, astiav.NewRational(1, 4)))
	require.Equal(t, int64(8), r.TimestampFromTime(time.Unix(2, 0), astiav.NewRational(1, 4)))
	require.Equal(t, time.Unix(4, 0), r.AddToTime(2*time.Second).TimeFromTimestamp(8, astiav.NewRational(1, 4)))
}
