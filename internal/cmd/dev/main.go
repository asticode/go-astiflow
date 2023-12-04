package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/astiflow/mocks"
	"github.com/asticode/go-astiflow/pkg/plugins/monitor/server"
	"github.com/asticode/go-astiflow/pkg/stats/psutil"
	"github.com/asticode/go-astikit"
	"github.com/asticode/go-astilog"
)

var l = astilog.New(astilog.Configuration{})

func main() {
	ds, err := psutil.New()
	if err != nil {
		l.Fatal(err)
	}

	w := astikit.NewWorker(astikit.WorkerOptions{Logger: l})
	w.HandleSignals(astikit.TermSignalHandler(w.Stop))

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
		Metadata: astiflow.Metadata{
			Description: "Flow description",
			Name:        "Flow",
		},
		Plugins: []astiflow.Plugin{
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
		l.Fatal(err)
	}
	defer f.Close()

	for i := 1; i <= 5; i++ {
		if _, err := addGroup(f); err != nil {
			l.Fatal(err)
		}
	}

	if err := f.Start(w.Context()); err != nil {
		l.Fatal(err)
	}

	go func() {
		time.Sleep(5 * time.Second)
		g, err := addGroup(f)
		if err != nil {
			l.Fatal(err)
		}
		time.Sleep(5 * time.Second)
		if err = g.Start(); err != nil {
			l.Fatal(err)
		}
		time.Sleep(5 * time.Second)
		if err = g.Stop(); err != nil {
			l.Fatal(err)
		}
	}()

	w.Wait()
}

var groupsCount int

func addGroup(f *astiflow.Flow) (*astiflow.Group, error) {
	groupsCount++

	g, err := f.NewGroup(astiflow.GroupOptions{Metadata: astiflow.Metadata{
		Description: fmt.Sprintf("Group %d description", groupsCount),
		Name:        fmt.Sprintf("Group %d", groupsCount),
	}})
	if err != nil {
		return nil, fmt.Errorf("main: creating group failed: %w", err)
	}

	for i := 1; i <= 10; i++ {
		if err := addNode(g); err != nil {
			return nil, fmt.Errorf("main: adding node failed: %w", err)
		}
	}

	return g, nil
}

var noders = []*mocks.MockedNoder{}

func addNode(g *astiflow.Group) (err error) {
	nr := mocks.NewMockedNoder()
	nr.OnDeltaStats = []astikit.DeltaStat{
		createDeltaStat("1"),
		createDeltaStat("2"),
		createDeltaStat("3"),
		createDeltaStat("4"),
		createDeltaStat("5"),
		createDeltaStat(fmt.Sprintf("6-%d", groupsCount)),
		createDeltaStat(fmt.Sprintf("7-%d", len(noders)+1)),
	}

	if nr.Node, _, err = g.NewNode(astiflow.NodeOptions{
		Metadata: astiflow.Metadata{
			Description: fmt.Sprintf("Node %d description", len(noders)+1),
			Name:        fmt.Sprintf("Node %d", len(noders)+1),
			Tags:        []string{g.Metadata().Name, fmt.Sprintf("Node %d", len(noders)+1)},
		},
		Noder: nr,
	}); err != nil {
		return fmt.Errorf("main: creating node failed: %w", err)
	}

	noders = append(noders, nr)

	if len(noders) > 1 {
		noders[len(noders)-2].Node.Connector().Connect(noders[len(noders)-1].Node.Connector())
	}
	return nil
}

func createDeltaStat(id string) astikit.DeltaStat {
	m := astikit.DeltaStatMetadata{
		Description: fmt.Sprintf("Stat %s description", id),
		Label:       fmt.Sprintf("Stat %s", id),
		Name:        fmt.Sprintf("custom.stat-%s", id),
	}

	var min, max int64
	switch {
	case strings.HasPrefix(id, "7"):
		min = 0
		max = 60
		m.Unit = "pps"
	case strings.HasPrefix(id, "6"):
		min = 0
		max = 30
		m.Unit = "fps"
	case strings.HasPrefix(id, "3"):
		min = 0
		max = 4e9
		m.Unit = "bps"
	case strings.HasPrefix(id, "2"):
		min = 0
		max = int64(4 * time.Hour)
		m.Unit = "ns"
	default:
		min = 0
		max = 100
		m.Unit = "%"
	}

	return astikit.DeltaStat{
		Metadata: m,
		Valuer:   newDeltaStatValuer(min, max),
	}
}

type deltaStatValuer struct {
	max int64
	min int64
}

func newDeltaStatValuer(min, max int64) *deltaStatValuer {
	return &deltaStatValuer{
		max: max,
		min: min,
	}
}

func (v *deltaStatValuer) Value(delta time.Duration) interface{} {
	i, err := rand.Int(rand.Reader, big.NewInt(v.max-v.min))
	if err != nil {
		return v.max
	}
	return v.min + i.Int64()
}
