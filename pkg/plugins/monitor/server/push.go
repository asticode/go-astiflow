package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/asticode/go-astiws"
)

type Pusher interface {
	io.Writer
}

type pushEventName string

const (
	pushEventNameDelta pushEventName = "delta"
	pushEventNamePing  pushEventName = "ping"
)

type pushEvent struct {
	Name    pushEventName `json:"name"`
	Payload interface{}   `json:"payload"`
}

type websocketPusher struct {
	p *Plugin
	s *astiws.Server
}

func (p *Plugin) newWebsocketPusher() *websocketPusher {
	w := &websocketPusher{p: p}
	w.s = astiws.NewServer(astiws.ServerOptions{
		ClientAdapter:  w.clientAdapter,
		Logger:         p.f.Logger(),
		MaxMessageSize: 1e6,
	})
	return w
}

func (p *websocketPusher) Close() error {
	return p.s.Close()
}

func (p *websocketPusher) clientAdapter(c *astiws.Client) error {
	// Replace context
	*c = *c.WithContext(p.p.ctx)

	// Set message handler
	c.SetMessageHandler(func(m []byte) error {
		// Unmarshal
		var e pushEvent
		if err := json.Unmarshal(m, &e); err != nil {
			return fmt.Errorf("astiws: unmarshaling message failed: %w", err)
		}

		// Switch
		switch e.Name {
		case pushEventNamePing:
			// Extend connection
			if err := c.ExtendConnection(); err != nil {
				return fmt.Errorf("server: extending notifier connection failed: %w", err)
			}
		}
		return nil
	})
	return nil
}

func (p *websocketPusher) Write(b []byte) (int, error) {
	// Loop through clients
	for _, c := range p.s.Clients() {
		// Write
		if err := c.WriteText(b); err != nil {
			return 0, fmt.Errorf("server: writing to websocket client failed: %w", err)
		}
	}
	return len(b), nil
}

func (p *websocketPusher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.s.ServeHTTP(w, r)
}
