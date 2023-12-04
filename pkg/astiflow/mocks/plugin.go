package mocks

import (
	"context"
	"errors"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

type MockedPlugin struct {
	Context     context.Context
	Initialized bool
	Started     bool
}

var _ astiflow.Plugin = (*MockedPlugin)(nil)

func NewMockedPlugin() *MockedPlugin {
	return &MockedPlugin{}
}

func (p *MockedPlugin) Init(ctx context.Context, c *astikit.Closer, f *astiflow.Flow) error {
	p.Context = ctx
	if p.Initialized {
		return errors.New("already initialized")
	}
	p.Initialized = true
	return nil
}

func (p *MockedPlugin) Metadata() astiflow.Metadata {
	return astiflow.Metadata{}
}

func (p *MockedPlugin) Start(ctx context.Context, tc astikit.TaskCreator) {
	p.Started = true
}
