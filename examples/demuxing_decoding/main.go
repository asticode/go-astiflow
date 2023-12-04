package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	astiavflow "github.com/asticode/go-astiflow/pkg/libs/astiav"
	"github.com/asticode/go-astiflow/pkg/plugins/monitor/server"
	"github.com/asticode/go-astiflow/pkg/stats/psutil"
	"github.com/asticode/go-astikit"
	"github.com/asticode/go-astilog"
)

var (
	input = flag.String("i", "", "the input path")
)

func main() {
	// Parse flags
	flag.Parse()

	// Usage
	if *input == "" {
		log.Println("Usage: <binary path> -i <input path>")
		return
	}

	// Create logger
	l := astilog.New(astilog.Configuration{})

	// Create psutil delta stat
	ds, err := psutil.New()
	if err != nil {
		l.Fatal(err)
	}

	// Create worker
	w := astikit.NewWorker(astikit.WorkerOptions{Logger: l})
	w.HandleSignals(astikit.TermSignalHandler(w.Stop))

	// Create flow
	f, err := astiflow.NewFlow(astiflow.FlowOptions{
		ContextAdapters: astiflow.FlowContextAdaptersOptions{
			Flow: func(ctx context.Context, f *astiflow.Flow) context.Context {
				return astilog.ContextWithFields(ctx, map[string]interface{}{
					"flow": f.String(),
				})
			},
			Group: func(ctx context.Context, f *astiflow.Flow, g *astiflow.Group) context.Context {
				return astilog.ContextWithFields(ctx, map[string]interface{}{
					"flow":  f.String(),
					"group": g.String(),
				})
			},
			Node: func(ctx context.Context, f *astiflow.Flow, g *astiflow.Group, n *astiflow.Node) context.Context {
				return astilog.ContextWithFields(ctx, map[string]interface{}{
					"flow":  f.String(),
					"group": g.String(),
					"node":  n.String(),
				})
			},
			Plugin: func(ctx context.Context, f *astiflow.Flow, p astiflow.Plugin) context.Context {
				return astilog.ContextWithFields(ctx, map[string]interface{}{
					"flow":   f.String(),
					"plugin": p.Metadata().Name,
				})
			},
		},
		DeltaStats: []astikit.DeltaStat{ds},
		Logger:     l,
		Plugins: []astiflow.Plugin{
			astiavflow.NewLogInterceptor(astiavflow.LogInterceptorOptions{
				Level: astiav.LogLevelInfo,
				Merge: astiavflow.LogInterceptorMergeOptions{
					AllowedCount: 5,
					Buffer:       10 * time.Second,
				},
			}),
			server.New(server.PluginOptions{
				Addr:        "127.0.0.1:4000",
				API:         server.PluginAPIOptions{URL: "/api"},
				DeltaPeriod: 2 * time.Second,
				Push:        server.PluginPushOptions{URL: "/push"},
			}),
		},
		Worker: w,
	})
	if err != nil {
		l.Fatal(fmt.Errorf("main: creating flow failed: %w", err))
	}
	defer f.Close()

	// Create group
	g, err := f.NewGroup(astiflow.GroupOptions{})
	if err != nil {
		l.Fatal(fmt.Errorf("main: creating group failed: %w", err))
	}

	// Create demuxer
	dmx, err := astiavflow.NewDemuxer(astiavflow.DemuxerOptions{
		Group: g,
		Start: astiavflow.DemuxerStartOptions{
			EmulateRate: &astiavflow.DemuxerEmulateRateOptions{},
			Loop:        true,
		},
	})
	if err != nil {
		l.Fatal(fmt.Errorf("main: creating demuxer failed: %w", err))
	}

	// Create decoder
	dec, err := astiavflow.NewDecoder(astiavflow.DecoderOptions{Group: g})
	if err != nil {
		l.Fatal(fmt.Errorf("main: creating decoder failed: %w", err))
	}

	// Connect
	dmx.Connect(dec, func(pkt astiavflow.Packet) (skip bool) { return pkt.StreamIndex() == 1 })

	// Open
	if err := dmx.Open(context.Background(), astiavflow.DemuxerOpenOptions{URL: *input}); err != nil {
		l.Fatal(fmt.Errorf("main: opening failed: %w", err))
	}

	// Start flow
	if err := f.Start(w.Context()); err != nil {
		l.Fatal(fmt.Errorf("main: starting flow failed: %w", err))
	}

	// Wait
	w.Wait()
}
