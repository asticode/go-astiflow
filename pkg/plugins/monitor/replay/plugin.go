package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/plugins/monitor/monitorer"
	"github.com/asticode/go-astikit"
)

var _ astiflow.Plugin = (*Plugin)(nil)

type Plugin struct {
	ctx context.Context
	f   *astiflow.Flow
	m   *monitorer.Monitorer
	o   PluginOptions
	w   io.Writer
}

type PluginOptions struct {
	DeltaPeriod time.Duration
	Path        string
}

func New(o PluginOptions) *Plugin {
	return &Plugin{o: o}
}

type initPayload struct {
	Flow initPayloadFlow `json:"flow"`
}

type initPayloadFlow struct {
	Description string `json:"description,omitempty"`
	ID          uint64 `json:"id"`
	Name        string `json:"name,omitempty"`
}

func (p *Plugin) Metadata() astiflow.Metadata {
	return astiflow.Metadata{Name: "monitor.replay"}
}

func (p *Plugin) Init(ctx context.Context, c *astikit.Closer, f *astiflow.Flow) error {
	// Create file
	file, err := os.Create(p.o.Path)
	if err != nil {
		return fmt.Errorf("replay: creating %s failed: %w", p.o.Path, err)
	}

	// Make sure to close file
	c.AddWithError(file.Close)

	// Create monitorer
	p.m = monitorer.New(monitorer.MonitorerOptions{
		Flow:    f,
		OnDelta: p.onDelta,
		Period:  p.o.DeltaPeriod,
	})

	// Make sure to close monitorer
	c.Add(p.m.Close)

	// Update plugin
	p.ctx = ctx
	p.f = f
	p.w = file

	// Write init
	p.write(initPayload{Flow: initPayloadFlow{
		Description: p.f.Metadata().Description,
		ID:          p.f.ID(),
		Name:        p.f.Metadata().Name,
	}})
	return nil
}

func (p *Plugin) Start(ctx context.Context, tc astikit.TaskCreator) {
	// Start monitorer
	tc().Do(func() { p.m.Start(ctx) })
}

func (p *Plugin) onDelta(d monitorer.Delta) {
	p.write(d)
}

func (p *Plugin) write(i interface{}) {
	// Marshal
	b, err := json.Marshal(i)
	if err != nil {
		p.f.Logger().WarnC(p.ctx, fmt.Errorf("replay: marshaling failed: %w", err))
		return
	}

	// Append new line
	b = append(b, []byte("\n")...)

	// Write
	if _, err = p.w.Write(b); err != nil {
		p.f.Logger().WarnC(p.ctx, fmt.Errorf("replay: writing in file failed: %w", err))
		return
	}
}
