package astiflow

import (
	"context"

	"github.com/asticode/go-astikit"
)

type Plugin interface {
	Init(ctx context.Context, c *astikit.Closer, f *Flow) error
	Metadata() Metadata
	Start(ctx context.Context, tc astikit.TaskCreator)
}
