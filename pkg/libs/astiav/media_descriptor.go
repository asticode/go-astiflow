package astiavflow

import "github.com/asticode/go-astiav"

// Information usually sent in stream's metadata and not accessible in packets or frames
type MediaDescriptor struct {
	FrameRate astiav.Rational
	Rotation  float64
	TimeBase  astiav.Rational
}

func newMediaDescriptorFromStream(s *astiav.Stream) MediaDescriptor {
	m := MediaDescriptor{TimeBase: s.TimeBase()}
	if v := s.AvgFrameRate(); v.Num() > 0 {
		m.FrameRate = v
	} else {
		m.FrameRate = s.RFrameRate()
	}
	if m.FrameRate.Float64() == 0 {
		switch s.CodecParameters().MediaType() {
		case astiav.MediaTypeAudio:
			if fs, sr := s.CodecParameters().FrameSize(), s.CodecParameters().SampleRate(); fs > 0 && sr > 0 {
				m.FrameRate = astiav.NewRational(sr, fs)
			}
		}
	}
	if sd := s.CodecParameters().SideData().Get(astiav.PacketSideDataTypeDisplaymatrix); len(sd) > 0 {
		if dm, err := astiav.NewDisplayMatrixFromBytes(sd); err == nil {
			m.Rotation = dm.Rotation()
		}
	}
	return m
}

func (md MediaDescriptor) equal(i MediaDescriptor) bool {
	return md.FrameRate.Float64() == i.FrameRate.Float64() &&
		md.Rotation == i.Rotation &&
		md.TimeBase.Float64() == i.TimeBase.Float64()
}
