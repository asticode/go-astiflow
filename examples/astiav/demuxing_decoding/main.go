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
	emulateRate = flag.Bool("er", false, "emulate rate")
	input       = flag.String("i", "", "input path")
	loop        = flag.Bool("l", false, "loop")
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
		l.Error(err)
		return
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
		Metadata:   astiflow.Metadata{Name: "Demuxing & Decoding example"},
		Plugins: []astiflow.Plugin{
			astiavflow.NewLogInterceptor(astiavflow.LogInterceptorOptions{
				Level: astiav.LogLevelInfo,
				Merge: astiavflow.LogInterceptorMergeOptions{
					AllowedCount: 5,
					Buffer:       10 * time.Second,
				},
			}),
			server.New(server.PluginOptions{
				Addr:        ":4000",
				API:         server.PluginAPIOptions{URL: "/api"},
				DeltaPeriod: 2 * time.Second,
				Push:        server.PluginPushOptions{URL: "/push"},
			}),
		},
		Stop:   &astiflow.FlowStopOptions{WhenAllGroupsAreDone: true},
		Worker: w,
	})
	if err != nil {
		l.Error(fmt.Errorf("main: creating flow failed: %w", err))
		return
	}
	defer f.Close()

	// Stop worker once flow is done
	f.On(astiflow.EventNameFlowDone, func(payload interface{}) (delete bool) {
		w.Stop()
		return false
	})

	// Create group
	g, err := f.NewGroup(astiflow.GroupOptions{Metadata: astiflow.Metadata{Name: "Demuxing & Decoding"}})
	if err != nil {
		l.Error(fmt.Errorf("main: creating group failed: %w", err))
		return
	}

	// Create demuxer start options
	so := astiavflow.DemuxerStartOptions{Loop: *loop}
	if *emulateRate {
		so.EmulateRate = &astiavflow.DemuxerEmulateRateOptions{}
	}

	// Create demuxer
	dmx, err := astiavflow.NewDemuxer(astiavflow.DemuxerOptions{
		Group: g,
		Start: so,
	})
	if err != nil {
		l.Error(fmt.Errorf("main: creating demuxer failed: %w", err))
		return
	}

	// Open demuxer
	if err := dmx.Open(context.Background(), astiavflow.DemuxerOpenOptions{URL: *input}); err != nil {
		l.Error(fmt.Errorf("main: opening demuxer failed: %w", err))
		return
	}

	// Loop through streams
	for _, s := range dmx.Streams() {
		// Only process audio and video
		if s.CodecParameters.MediaType() != astiav.MediaTypeAudio && s.CodecParameters.MediaType() != astiav.MediaTypeVideo {
			continue
		}

		// Create decoder
		dec, err := astiavflow.NewDecoder(astiavflow.DecoderOptions{
			Group: g,
			Stop:  &astiflow.NodeStopOptions{WhenAllParentsAreDone: true},
		})
		if err != nil {
			l.Error(fmt.Errorf("main: creating %s decoder failed: %w", s.CodecParameters.MediaType(), err))
			return
		}

		// Connect demuxer to decoder
		if err = dmx.Connect(dec, s); err != nil {
			l.Error(fmt.Errorf("main: connecting demuxer to %s decoder failed: %w", s.CodecParameters.MediaType(), err))
			return
		}

		// Create frame interceptor
		fi, err := astiavflow.NewFrameInterceptor(astiavflow.FrameInterceptorOptions{
			Group: g,
			OnFrame: func(d astiavflow.FrameDescriptor, f *astiav.Frame) (dispatch bool, err error) {
				l.Infof("main: new frame: stream: %d - pts: %d", s.Index, f.Pts())
				return false, nil
			},
			Stop: &astiflow.NodeStopOptions{WhenAllParentsAreDone: true},
		})
		if err != nil {
			l.Error(fmt.Errorf("main: creating frame interceptor failed: %w", err))
			return
		}

		// Connect decoder to frame interceptor
		if err = dec.Connect(fi); err != nil {
			l.Error(fmt.Errorf("main: connecting %s decoder to frame interceptor failed: %w", s.CodecParameters.MediaType(), err))
			return
		}
	}

	// Start flow
	if err := f.Start(w.Context()); err != nil {
		l.Error(fmt.Errorf("main: starting flow failed: %w", err))
		return
	}

	// Wait
	w.Wait()
}
