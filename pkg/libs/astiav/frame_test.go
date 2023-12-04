package astiavflow

import (
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
)

type mockedFrameHandler struct {
	*mocks.MockedNoder
	handleFrameFunc func(f Frame)
}

var _ FrameHandler = (*mockedFrameHandler)(nil)

func newMockedFrameHandler() *mockedFrameHandler {
	return &mockedFrameHandler{
		MockedNoder: mocks.NewMockedNoder(),
	}
}

func (h *mockedFrameHandler) HandleFrame(f Frame) {
	if h.handleFrameFunc != nil {
		h.handleFrameFunc(f)
	}
}

func (h *mockedFrameHandler) NodeConnector() *astiflow.NodeConnector {
	return h.Node.Connector()
}
