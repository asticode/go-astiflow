package astiavflow

import "github.com/asticode/go-astiav"

// Information usually sent in stream's metadata and not accessible in packets or frames
type MediaContext struct {
	FrameRate astiav.Rational
	Rotation  float64
	TimeBase  astiav.Rational
}

func newMediaContextFromStream(s *astiav.Stream) MediaContext {
	m := MediaContext{TimeBase: s.TimeBase()}
	if v := s.AvgFrameRate(); v.Num() > 0 {
		m.FrameRate = v
	} else {
		m.FrameRate = s.RFrameRate()
	}
	if sd := s.SideData(astiav.PacketSideDataTypeDisplaymatrix); len(sd) > 0 {
		if dm, err := astiav.NewDisplayMatrixFromBytes(sd); err == nil {
			m.Rotation = dm.Rotation()
		}
	}
	return m
}
