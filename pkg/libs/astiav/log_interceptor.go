package astiavflow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

var _ astiflow.Plugin = (*LogInterceptor)(nil)

type LogInterceptor struct {
	c             *astikit.Closer
	ctx           context.Context
	f             *astiflow.Flow
	items         map[string]*logInterceptorItem // Indexed by key
	mi            sync.Mutex                     // Locks items
	o             LogInterceptorOptions
	previousLevel *astiav.LogLevel
}

type logInterceptorItem struct {
	count     uint
	createdAt time.Time
	ctx       context.Context
	fmt       string
	key       string
	ll        astikit.LoggerLevel
	msg       string
	written   uint
}

func newLogInterceptorItem(ctx context.Context, ll astikit.LoggerLevel, fmt, key, msg string) *logInterceptorItem {
	return &logInterceptorItem{
		count:     1,
		createdAt: astikit.Now(),
		ctx:       ctx,
		fmt:       fmt,
		key:       key,
		ll:        ll,
		msg:       msg,
		written:   1,
	}
}

type LogInterceptorOptions struct {
	Level     astiav.LogLevel
	LevelFunc func(l astiav.LogLevel) (ll astikit.LoggerLevel, processed, stop bool)
	Merge     LogInterceptorMergeOptions
}

type LogInterceptorMergeOptions struct {
	AllowedCount uint
	Buffer       time.Duration
}

func NewLogInterceptor(o LogInterceptorOptions) *LogInterceptor {
	return &LogInterceptor{
		items: make(map[string]*logInterceptorItem),
		o:     o,
	}
}

func (li *LogInterceptor) Metadata() astiflow.Metadata {
	return astiflow.Metadata{Name: "astiavflow.log_interceptor"}
}

func (li *LogInterceptor) Init(ctx context.Context, c *astikit.Closer, f *astiflow.Flow) error {
	// Update plugin
	li.c = c
	li.ctx = ctx
	li.f = f

	// Listen to flow
	f.On(astiflow.EventNameGroupCreated, func(payload interface{}) (delete bool) {
		// Assert payload
		g, ok := payload.(*astiflow.Group)
		if !ok {
			return
		}

		// Listen to groups
		g.On(astiflow.EventNameNodeCreated, func(payload interface{}) (delete bool) {
			// Assert payload
			n, ok := payload.(*astiflow.Node)
			if !ok {
				return
			}

			// Update log interceptors
			logInterceptors.set(n, li)

			// Listen to node
			n.On(astiflow.EventNameNodeDone, func(payload interface{}) (delete bool) {
				// Update log interceptors
				logInterceptors.del(n)
				return
			})
			return
		})
		return
	})
	return nil
}

func (li *LogInterceptor) close() {
	if li.previousLevel != nil {
		astiav.SetLogLevel(*li.previousLevel)
		li.previousLevel = nil
	}
	astiav.ResetLogCallback()
	li.purge()
}

func (li *LogInterceptor) callback(c astiav.Classer, level astiav.LogLevel, fmt, msg string) {
	// Process message
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}

	// Process format
	fmt = strings.TrimSpace(fmt)
	if fmt == "%s" {
		fmt = msg
	}

	// Get context
	ctx := li.ctx

	// Process classer
	if c != nil {
		if cl := c.Class(); cl != nil {
			msg += ": " + cl.String()
		}
		if n, ok := classers.get(c); ok {
			ctx = n.Context()
		}
	}

	// Get log level
	var ll astikit.LoggerLevel
	var processed bool
	if li.o.LevelFunc != nil {
		var stop bool
		if ll, processed, stop = li.o.LevelFunc(level); stop {
			return
		}
	}
	if !processed {
		switch level {
		case astiav.LogLevelDebug, astiav.LogLevelVerbose:
			ll = astikit.LoggerLevelDebug
		case astiav.LogLevelInfo:
			ll = astikit.LoggerLevelInfo
		case astiav.LogLevelError, astiav.LogLevelFatal, astiav.LogLevelPanic:
			if level == astiav.LogLevelFatal {
				msg = "FATAL! " + msg
			} else if level == astiav.LogLevelPanic {
				msg = "PANIC! " + msg
			}
			ll = astikit.LoggerLevelError
		case astiav.LogLevelWarning:
			ll = astikit.LoggerLevelWarn
		default:
			return
		}
	}

	// Add prefix
	fmt = "libav: " + fmt
	msg = "libav: " + msg

	// Write
	li.write(ctx, ll, fmt, msg)
}

func (li *LogInterceptor) write(ctx context.Context, ll astikit.LoggerLevel, fmt, msg string) {
	// Merge
	if li.o.Merge.Buffer > 0 {
		// Add item
		if write := li.addItem(ctx, ll, fmt, msg); !write {
			return
		}
	}

	// Write
	li.f.Logger().WriteC(ctx, ll, msg)
}

func (li *LogInterceptor) key(ll astikit.LoggerLevel, fmt string) string {
	return ll.String() + ":" + fmt
}

func (li *LogInterceptor) addItem(ctx context.Context, ll astikit.LoggerLevel, fmt, msg string) (write bool) {
	// Lock
	li.mi.Lock()
	defer li.mi.Unlock()

	// Create key
	key := li.key(ll, fmt)

	// Check whether item exists
	i, ok := li.items[key]
	if ok {
		i.count++
		if write = li.o.Merge.AllowedCount > 0 && i.count <= li.o.Merge.AllowedCount; write {
			i.written++
		}
		return
	}

	// Create item
	li.items[key] = newLogInterceptorItem(ctx, ll, fmt, key, msg)
	return true
}

func (li *LogInterceptor) Start(ctx context.Context, tc astikit.TaskCreator) {
	// Set log level
	ll := astiav.GetLogLevel()
	li.previousLevel = &ll
	astiav.SetLogLevel(li.o.Level)

	// Set log callback
	astiav.SetLogCallback(li.callback)

	// Make sure interceptor is closed properly
	li.c.Add(li.close)

	// Start merger
	if li.o.Merge.Buffer > 0 {
		tc().Do(func() {
			// Tick
			astikit.Tick(ctx, li.o.Merge.Buffer/10, li.tick)
		})
	}
}

func (li *LogInterceptor) tick(t time.Time) {
	// Lock
	li.mi.Lock()
	defer li.mi.Unlock()

	// Loop through items
	for _, i := range li.items {
		// Period has been reached
		if t.Sub(i.createdAt) >= li.o.Merge.Buffer {
			// Remove item
			li.removeItemUnlocked(i)
		}
	}
}

func (li *LogInterceptor) removeItemUnlocked(i *logInterceptorItem) {
	repeated := i.count - i.written
	if repeated > 1 {
		li.f.Logger().WriteC(i.ctx, i.ll, fmt.Sprintf("astiavflow: pattern repeated %d times: %s", repeated, i.fmt))
	} else if repeated == 1 {
		li.f.Logger().WriteC(i.ctx, i.ll, "astiavflow: pattern repeated once: "+i.fmt)
	}
	delete(li.items, i.key)
}

func (li *LogInterceptor) purge() {
	// Lock
	li.mi.Lock()
	defer li.mi.Unlock()

	// Loop through items
	for _, i := range li.items {
		li.removeItemUnlocked(i)
	}
}

var logInterceptors = newLogInterceptorPool()

type logInterceptorPool struct {
	m sync.Mutex
	p map[*astiflow.Node]*LogInterceptor
}

func newLogInterceptorPool() *logInterceptorPool {
	return &logInterceptorPool{p: make(map[*astiflow.Node]*LogInterceptor)}
}

func (p *logInterceptorPool) set(n *astiflow.Node, li *LogInterceptor) {
	p.m.Lock()
	defer p.m.Unlock()
	p.p[n] = li
}

func (p *logInterceptorPool) del(n *astiflow.Node) {
	p.m.Lock()
	defer p.m.Unlock()
	delete(p.p, n)
}

func (p *logInterceptorPool) get(n *astiflow.Node) (*LogInterceptor, bool) {
	p.m.Lock()
	defer p.m.Unlock()
	li, ok := p.p[n]
	return li, ok
}
