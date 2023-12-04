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
	emulateRate  = flag.Bool("er", false, "emulate rate")
	input        = flag.String("i", "", "input path")
	loop         = flag.Bool("l", false, "loop")
	output       = flag.String("o", "", "output path")
	videoBitrate = flag.Int64("vb", 1e6, "video bitrate")
	videoCodec   = flag.String("vc", "libx264", "video codec")
)

func main() {
	// Parse flags
	flag.Parse()

	// Usage
	if *input == "" || *output == "" {
		log.Println("Usage: <binary path> -i <input path> -o <output path>")
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
		Metadata:   astiflow.Metadata{Name: "Filtering example"},
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
	g, err := f.NewGroup(astiflow.GroupOptions{Metadata: astiflow.Metadata{Name: "Filtering"}})
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

	// Create muxer
	mx, err := astiavflow.NewMuxer(astiavflow.MuxerOptions{
		Group: g,
		Stop:  &astiflow.NodeStopOptions{WhenAllParentsAreDone: true},
	})
	if err != nil {
		l.Error(fmt.Errorf("main: creating muxer failed: %w", err))
		return
	}

	// Open muxer
	if err := mx.Open(astiavflow.MuxerOpenOptions{URL: *output}); err != nil {
		l.Error(fmt.Errorf("main: opening muxer failed: %w", err))
		return
	}

	// Loop through streams
	for _, s := range dmx.Streams() {
		// Switch on media type
		switch s.CodecParameters.MediaType() {
		case astiav.MediaTypeAudio:
			// We remux the audio

			// Create packet interceptor
			i, err := astiavflow.NewPacketInterceptor(astiavflow.PacketInterceptorOptions{
				Group:    g,
				OnPacket: func(p *astiav.Packet, pd astiavflow.PacketDescriptor) (dispatch bool, err error) { return true, nil },
				Stop:     &astiflow.NodeStopOptions{WhenAllParentsAreDone: true},
			})
			if err != nil {
				l.Error(fmt.Errorf("main: creating %s packet interceptor failed: %w", s.CodecParameters.MediaType(), err))
				return
			}

			// Connect demuxer to packet interceptor
			if err = dmx.Connect(i, s); err != nil {
				l.Error(fmt.Errorf("main: connecting demuxer to %s packet interceptor failed: %w", s.CodecParameters.MediaType(), err))
				return
			}

			// Connect packet interceptor to muxer
			if err = i.Connect(mx); err != nil {
				l.Error(fmt.Errorf("main: connecting %s packet interceptor to muxer failed: %w", s.CodecParameters.MediaType(), err))
				return
			}
		case astiav.MediaTypeVideo:
			// We rotate the video

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

			// Create frame filterer
			ff, err := astiavflow.NewFrameFilterer(astiavflow.FrameFiltererOptions{
				Filterer: astiavflow.FrameFiltererFiltererOptions{
					Aliases: map[astiflow.Noder]string{dec: "in"},
					Content: "[in]transpose=cclock",
				},
				Group: g,
				Stop:  &astiflow.NodeStopOptions{WhenAllParentsAreDone: true},
			})
			if err != nil {
				l.Error(fmt.Errorf("main: creating %s frame filterer failed: %w", s.CodecParameters.MediaType(), err))
				return
			}

			// Connect decoder to frame filterer
			if err = dec.Connect(ff); err != nil {
				l.Error(fmt.Errorf("main: connecting %s decoder to %s frame filterer failed: %w", s.CodecParameters.MediaType(), s.CodecParameters.MediaType(), err))
				return
			}

			// Create encoder
			enc, err := astiavflow.NewEncoder(astiavflow.EncoderOptions{
				Group: g,
				Stop:  &astiflow.NodeStopOptions{WhenAllParentsAreDone: true},
				Writer: astiavflow.EncoderWriterOptions{
					BitRate:   *videoBitrate,
					CodecName: *videoCodec,
					Flags:     astiav.NewCodecContextFlags(astiav.CodecContextFlagGlobalHeader),
					GopSize:   func(framerate astiav.Rational) int { return int(2 * framerate.Float64()) },
				},
			})
			if err != nil {
				l.Error(fmt.Errorf("main: creating %s encoder failed: %w", s.CodecParameters.MediaType(), err))
				return
			}

			// Connect frame filterer to encoder
			if err = ff.Connect(enc); err != nil {
				l.Error(fmt.Errorf("main: connecting %s frame filterer to %s encoder failed: %w", s.CodecParameters.MediaType(), s.CodecParameters.MediaType(), err))
				return
			}

			// Connect encoder to muxer
			if err = enc.Connect(mx); err != nil {
				l.Error(fmt.Errorf("main: connecting %s encoder to muxer failed: %w", s.CodecParameters.MediaType(), err))
				return
			}
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
