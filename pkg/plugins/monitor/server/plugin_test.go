package server_test

import (
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astiflow/pkg/plugins/monitor/server"
	"github.com/asticode/go-astikit"
	"github.com/asticode/go-astiws"
	"github.com/stretchr/testify/require"
)

type mockedPusher struct {
	bs     [][]byte
	closed bool
	served bool
}

func newMockedPusher() *mockedPusher {
	return &mockedPusher{}
}

func (p *mockedPusher) ServeHTTP(http.ResponseWriter, *http.Request) {
	p.served = true
}

func (p *mockedPusher) Close() error {
	p.closed = true
	return nil
}

func (p *mockedPusher) Write(b []byte) (int, error) {
	p.bs = append(p.bs, b)
	return len(b), nil
}

type mockedResponseWriter struct {
	b          []byte
	statusCode int
}

func newMockedResponseWriter() *mockedResponseWriter {
	return &mockedResponseWriter{}
}

func (rw *mockedResponseWriter) Header() http.Header {
	return http.Header{}
}

func (rw *mockedResponseWriter) Write(b []byte) (int, error) {
	rw.b = append(rw.b, b...)
	return len(b), nil
}

func (rw *mockedResponseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
}

func TestPlugin(t *testing.T) {
	defer astikit.MockNow(func() time.Time {
		return time.Unix(5, 0)
	}).Close()

	w := astikit.NewWorker(astikit.WorkerOptions{})
	ps := newMockedPusher()
	p := server.New(server.PluginOptions{
		API: server.PluginAPIOptions{
			Headers:     map[string]string{"h": "v"},
			QueryParams: map[string]string{"q": "v"},
			URL:         "https://test.com/api",
		},
		DeltaPeriod: time.Millisecond,
		Push: server.PluginPushOptions{
			Pusher:      ps,
			QueryParams: map[string]string{"q": "v"},
			URL:         "https://test.com/push",
		},
	})
	const addr = "127.0.0.1:40000"
	f, err := astiflow.NewFlow(astiflow.FlowOptions{
		DeltaStats: []astikit.DeltaStat{{
			Metadata: astikit.DeltaStatMetadata{Name: "sn"},
			Valuer: astikit.DeltaStatValuerFunc(func(d time.Duration) interface{} {
				return 1
			}),
		}},
		Metadata: astiflow.Metadata{
			Description: "fd",
			Name:        "fn",
		},
		Plugins: []astiflow.Plugin{
			server.New(server.PluginOptions{
				Addr: addr,
				API: server.PluginAPIOptions{
					Headers:     map[string]string{"h": "v"},
					QueryParams: map[string]string{"q": "v"},
					URL:         "/api",
				},
				DeltaPeriod: time.Millisecond,
				Push: server.PluginPushOptions{
					QueryParams: map[string]string{"q": "v"},
					URL:         "/push",
				},
			}),
			p,
		},
		Worker: w,
	})
	require.NoError(t, err)
	defer f.Close()

	require.NoError(t, f.Start(w.Context()))

	s := time.Now()
	const httpAddr = "http://" + addr
	for {
		n := time.Now()
		if n.Sub(s) > time.Second {
			t.Fatalf("waiting for %s timed out", httpAddr)
		}

		resp, err := http.Get(httpAddr)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	for _, v := range []struct {
		f func() (*http.Response, error)
		s string
	}{
		{
			f: func() (*http.Response, error) { return http.Get(httpAddr + "/config.json") },
			s: `{"api":{"headers":{"h":"v"},"query_params":{"q":"v"},"url":"/api"},"push":{"query_params":{"q":"v"},"url":"/push"}}
`,
		},
		{
			f: func() (*http.Response, error) { return http.Get(httpAddr + "/img/logo.svg") },
		},
		{
			f: func() (*http.Response, error) { return http.Get(httpAddr + "/api/catch-up") },
			s: `{"at":5,"new_stats":[{"id":1,"metadata":{"name":"sn"}}],"stat_values":{"1":1},"flow":{"description":"fd","id":1,"name":"fn"}}
`,
		},
	} {
		func() {
			resp, err := v.f()
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			b, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			if v.s != "" {
				require.Equal(t, v.s, string(b))
			}
		}()
	}

	const wsAddr = "ws://" + addr + "/push"
	c := astiws.NewClient(astiws.ClientConfiguration{MaxMessageSize: 1e3}, astikit.AdaptTestLogger(t))
	defer c.Close()
	var cbs [][]byte
	c.SetMessageHandler(func(m []byte) error {
		cbs = append(cbs, m)
		return nil
	})
	c.DialAndRead(w, astiws.DialAndReadOptions{Addr: wsAddr})

	s = time.Now()
	for {
		n := time.Now()
		if n.Sub(s) > time.Second {
			t.Fatalf("waiting for messages on %s timed out", wsAddr)
		}

		if len(cbs) > 0 {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	require.Equal(t, `{"name":"delta","payload":{"at":5,"stat_values":{"1":1}}}`, string(cbs[0]))

	for _, v := range []struct {
		f    func()
		h    http.Handler
		path string
		s    string
	}{
		{
			h: p.ServeAPICatchUp(),
			s: `{"at":5,"new_stats":[{"id":1,"metadata":{"name":"sn"}}],"stat_values":{"1":1},"flow":{"description":"fd","id":1,"name":"fn"}}
`,
		},
		{
			f: func() {
				require.True(t, ps.served)
				require.True(t, len(ps.bs) > 1)
				require.Equal(t, `{"name":"delta","payload":{"at":5,"new_stats":[{"id":1,"metadata":{"name":"sn"}}],"stat_values":{"1":1}}}`, string(ps.bs[0]))
				require.Equal(t, `{"name":"delta","payload":{"at":5,"stat_values":{"1":1}}}`, string(ps.bs[1]))
			},
			h: p.ServePush(),
		},
		{
			h:    p.ServeWeb(),
			path: "/config.json",
			s: `{"api":{"headers":{"h":"v"},"query_params":{"q":"v"},"url":"https://test.com/api"},"push":{"query_params":{"q":"v"},"url":"https://test.com/push"}}
`,
		},
		{
			h:    p.ServeWeb(),
			path: "/img/logo.svg",
		},
	} {
		rw := newMockedResponseWriter()
		v.h.ServeHTTP(rw, &http.Request{URL: &url.URL{Path: v.path}})
		if v.f != nil {
			v.f()
		} else if v.s != "" {
			require.Equal(t, v.s, string(rw.b))
		}
	}

	w.Stop()

	require.Eventually(t, func() bool { return f.Status() == astiflow.StatusDone }, time.Second, 10*time.Millisecond)
	require.True(t, ps.closed)
}
