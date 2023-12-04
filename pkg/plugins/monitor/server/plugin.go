package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
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
	p   Pusher
	s   *http.Server
}

type PluginOptions struct {
	Addr        string
	API         PluginAPIOptions
	DeltaPeriod time.Duration
	Push        PluginPushOptions
}

type PluginAPIOptions struct {
	Headers     map[string]string
	QueryParams map[string]string
	URL         string
}

type PluginPushOptions struct {
	Pusher      Pusher
	QueryParams map[string]string
	URL         string
}

func New(o PluginOptions) *Plugin {
	return &Plugin{o: o}
}

func (p *Plugin) Metadata() astiflow.Metadata {
	return astiflow.Metadata{Name: "monitor.server"}
}

func (p *Plugin) Init(ctx context.Context, c *astikit.Closer, f *astiflow.Flow) error {
	// Store flow
	p.ctx = ctx
	p.f = f

	// Create monitorer
	p.m = monitorer.New(monitorer.MonitorerOptions{
		Flow:    f,
		OnDelta: p.onDelta,
		Period:  p.o.DeltaPeriod,
	})

	// Get pusher
	p.p = p.o.Push.Pusher
	if p.p == nil {
		p.p = p.newWebsocketPusher()
	}

	// Make sure pusher is properly closed
	if v, ok := p.p.(io.Closer); ok {
		c.AddWithError(v.Close)
	}

	// Addr was provided
	// We need the pusher at that point
	if p.o.Addr != "" {
		// Create http server
		p.s = &http.Server{
			Addr:    p.o.Addr,
			Handler: p.handler(),
		}

		// Make sure http server is closed properly
		c.AddWithError(p.s.Close)
	}
	return nil
}

func (p *Plugin) Start(ctx context.Context, tc astikit.TaskCreator) {
	// Start http server
	if p.s != nil {
		// Do
		tc().Do(func() {
			// Log
			p.f.Logger().InfoCf(p.ctx, "server: serving on %s", p.o.Addr)

			// Serve
			var done = make(chan error)
			go func() {
				if err := p.s.ListenAndServe(); err != nil {
					done <- err
				}
			}()

			// Wait
			select {
			case <-ctx.Done():
			case err := <-done:
				if err != nil {
					p.f.Logger().WarnC(p.ctx, fmt.Errorf("server: serving on %s failed: %w", p.o.Addr, err))
				}
			}

			// Shutdown
			p.f.Logger().InfoCf(p.ctx, "server: shutting down server on %s", p.o.Addr)
			if err := p.s.Shutdown(context.Background()); err != nil {
				p.f.Logger().WarnC(p.ctx, fmt.Errorf("server: shutting down server on %s failed: %w", p.o.Addr, err))
			}
		})
	}

	// Start monitorer
	if p.m != nil {
		tc().Do(func() { p.m.Start(ctx) })
	}
}

func (p *Plugin) handler() http.Handler {
	// Create mux
	m := http.NewServeMux()

	// Add web route
	m.Handle("/", p.ServeWeb())

	// Add api routes
	if strings.HasPrefix(p.o.API.URL, "/") {
		m.Handle(p.o.API.URL+"/catch-up", p.ServeAPICatchUp())
	}

	// Add push route
	if strings.HasPrefix(p.o.Push.URL, "/") {
		m.Handle(p.o.Push.URL, p.ServePush())
	}
	return m
}

type apiCatchUp struct {
	monitorer.Delta
	Flow apiCatchUpFlow `json:"flow"`
}

type apiCatchUpFlow struct {
	Description string `json:"description,omitempty"`
	ID          uint64 `json:"id"`
	Name        string `json:"name,omitempty"`
}

func (p *Plugin) ServeAPICatchUp() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write
		if err := json.NewEncoder(w).Encode(apiCatchUp{
			Delta: p.m.CatchUp(),
			Flow: apiCatchUpFlow{
				Description: p.f.Metadata().Description,
				ID:          p.f.ID(),
				Name:        p.f.Metadata().Name,
			},
		}); err != nil {
			p.f.Logger().WarnC(p.ctx, fmt.Errorf("server: writing api catch up body failed: %w", err))
			return
		}
	})
}

func (p *Plugin) ServePush() http.Handler {
	if h, ok := p.p.(http.Handler); ok {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
}

type webConfig struct {
	API  webAPIConfig  `json:"api"`
	Push webPushConfig `json:"push"`
}

type webAPIConfig struct {
	Headers     map[string]string `json:"headers,omitempty"`
	QueryParams map[string]string `json:"query_params,omitempty"`
	URL         string            `json:"url,omitempty"`
}

type webPushConfig struct {
	QueryParams map[string]string `json:"query_params,omitempty"`
	URL         string            `json:"url,omitempty"`
}

func (p *Plugin) ServeWeb() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Config
		if r.URL.Path == "/config.json" {
			// Write
			if err := json.NewEncoder(w).Encode(webConfig{
				API: webAPIConfig{
					Headers:     p.o.API.Headers,
					QueryParams: p.o.API.QueryParams,
					URL:         p.o.API.URL,
				},
				Push: webPushConfig{
					QueryParams: p.o.Push.QueryParams,
					URL:         p.o.Push.URL,
				},
			}); err != nil {
				p.f.Logger().WarnC(p.ctx, fmt.Errorf("server: writing config body failed: %w", err))
				return
			}
			return
		}

		// Get fs
		fs := http.FS(p.webFS())

		// This is a page
		if path.Ext(r.URL.Path) == "" {
			// Open /index.html
			f, err := fs.Open("/index.html")
			if err != nil {
				p.f.Logger().WarnC(p.ctx, fmt.Errorf("server: opening /index.html failed: %w", err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer f.Close()

			// Stat
			fi, err := f.Stat()
			if err != nil {
				p.f.Logger().WarnC(p.ctx, fmt.Errorf("server: stating /index.html failed: %w", err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			// Serve content
			http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
			return
		}

		// Default
		http.FileServer(fs).ServeHTTP(w, r)
	})
}

func (p *Plugin) onDelta(d monitorer.Delta) {
	// Marshal
	b, err := json.Marshal(pushEvent{
		Name:    pushEventNameDelta,
		Payload: d,
	})
	if err != nil {
		p.f.Logger().WarnC(p.ctx, fmt.Errorf("server: marshaling push event failed: %w", err))
		return
	}

	// Push
	if _, err := p.p.Write(b); err != nil {
		p.f.Logger().WarnC(p.ctx, fmt.Errorf("server: pushing failed: %w", err))
		return
	}

}
