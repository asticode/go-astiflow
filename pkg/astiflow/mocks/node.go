package mocks

import (
	"context"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

type MockedNoder struct {
	Node         *astiflow.Node
	OnDeltaStats []astikit.DeltaStat
	OnStart      func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator)
}

var _ astiflow.Noder = (*MockedNoder)(nil)

func NewMockedNoder() *MockedNoder {
	return &MockedNoder{OnStart: func(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
		tc().Do(func() { <-ctx.Done() })
	}}
}

func (n *MockedNoder) DeltaStats() []astikit.DeltaStat { return n.OnDeltaStats }

func (n *MockedNoder) Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	n.OnStart(ctx, cancel, tc)
}
