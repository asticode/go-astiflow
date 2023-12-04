package astiavflow

import (
	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
)

type Frame struct {
	*astiav.Frame
	FrameDescriptor
	Noder astiflow.Noder
}
