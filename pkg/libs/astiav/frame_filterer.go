package astiavflow

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/asticode/go-astikit"
)

var _ FrameHandler = (*FrameFilterer)(nil)

var (
	countFrameFilterer uint64
)

type FrameFilterer struct {
	*frameHandler
	aliases                         FrameFiltererAliases
	c                               *astikit.Closer
	content                         string
	emulateRate                     astiav.Rational
	f                               *frameFiltererFilter
	fd                              *frameDispatcher
	mf                              sync.Mutex // Locks f
	n                               *astiflow.Node
	onBeforeDispatch                FrameFiltererOnBeforeDispatchFunc
	onIncomingFrameDescriptorsDelta FrameFiltererOnIncomingFrameDescriptorsDeltaFunc
	outgoingFrameDescriptorAdapter  FrameDescriptorAdapter
	q                               frameFiltererQueuer
	threadCount                     int
	threadType                      astiav.ThreadType
}

type FrameFiltererAliases map[astiflow.Noder]string

type FrameFiltererOnBeforeDispatchFunc func(f *astiav.Frame)

type FrameFiltererOnIncomingFrameDescriptorsDeltaFunc func(d FrameDescriptorsDelta, ff *FrameFilterer)

// TODO Add an option to ignore incoming frames until all parents have dispatched at least one frame
type FrameFiltererOptions struct {
	Filterer         FrameFiltererFiltererOptions
	Group            *astiflow.Group
	Metadata         astiflow.Metadata
	OnBeforeDispatch FrameFiltererOnBeforeDispatchFunc
	Stop             *astiflow.NodeStopOptions
}

type FrameFiltererFiltererOptions struct {
	Aliases                          map[astiflow.Noder]string
	Content                          string
	EmulateRate                      astiav.Rational
	OnIncomingFrameDescriptorsDelta  FrameFiltererOnIncomingFrameDescriptorsDeltaFunc
	OutgoingFrameDescriptorAdapter   FrameDescriptorAdapter
	SynchronizeIncomingFramesWithPTS bool
	ThreadCount                      int
	ThreadType                       astiav.ThreadType
}

func NewFrameFilterer(o FrameFiltererOptions) (ff *FrameFilterer, err error) {
	// Create frame filterer
	ff = &FrameFilterer{
		aliases:                         o.Filterer.Aliases,
		content:                         o.Filterer.Content,
		emulateRate:                     o.Filterer.EmulateRate,
		frameHandler:                    newFrameHandler(),
		fd:                              newFrameDispatcher(),
		onBeforeDispatch:                o.OnBeforeDispatch,
		onIncomingFrameDescriptorsDelta: o.Filterer.OnIncomingFrameDescriptorsDelta,
		outgoingFrameDescriptorAdapter:  o.Filterer.OutgoingFrameDescriptorAdapter,
		threadCount:                     o.Filterer.ThreadCount,
		threadType:                      o.Filterer.ThreadType,
	}

	// Create node
	if ff.n, ff.c, err = o.Group.NewNode(astiflow.NodeOptions{
		Metadata: (&astiflow.Metadata{
			Name: fmt.Sprintf("frame_filterer_%d", atomic.AddUint64(&countFrameFilterer, uint64(1))),
			Tags: []string{"frame_filterer"},
		}).Merge(o.Metadata),
		Noder: ff,
		Stop:  o.Stop,
	}); err != nil {
		err = fmt.Errorf("astiavflow: creating node failed: %w", err)
		return
	}

	// Initialize dispatchers, handlers and pools
	ff.frameHandler.init(frameHandlerInitOptions{
		c:         ff.c,
		n:         ff.n,
		onConnect: ff.onConnect,
		onFrame:   ff.onFrame,
	})
	ff.fd.init(ff.n)
	ff.fp.init(ff.c)

	// Make sure to close filter
	ff.c.Add(ff.closeFilter)

	// Create queuer
	if o.Filterer.SynchronizeIncomingFramesWithPTS && len(ff.aliases) > 0 {
		ff.q = newPTSFrameFiltererQueuer(len(ff.aliases))
	} else {
		ff.q = newDefaultFrameFiltererQueuer(len(ff.aliases))
	}

	// No aliases
	if len(ff.aliases) == 0 {
		// No emulate rate
		if ff.emulateRate.Float64() <= 0 {
			err = errors.New("astiavflow: no aliases but no emulate rate either")
			return
		}

		// Guess filter media type
		var mt astiav.MediaType
		if mt, err = ff.guessFilterMediaType(ff.content); err != nil {
			err = fmt.Errorf("astiavflow: guessing filter media type failed: %w", err)
			return
		}

		// Refresh filter
		if err = ff.refreshFilter(frameFiltererCreateFilterOptions{outgoingMediaType: &mt}); err != nil {
			err = fmt.Errorf("astiavflow: refreshing filter failed: %w", err)
			return
		}
	}
	return
}

func (ff *FrameFilterer) guessFilterMediaType(content string) (astiav.MediaType, error) {
	// Create grapher
	g := newFrameFiltererGrapher()
	defer g.Free()

	// Parse segment
	s, err := g.ParseSegment(content)
	if err != nil {
		return astiav.MediaTypeUnknown, fmt.Errorf("astiavflow: parsing segment failed: %w", err)
	}
	defer s.Free()

	// Loop through chains
	for _, c := range s.Chains() {
		// Loop through filters
		for _, fp := range c.Filters() {
			// Find filter
			f := newFrameFiltererFilterer(fp.FilterName())
			if f == nil {
				continue
			}

			// Loop through outputs
			for _, o := range f.Outputs() {
				// Check media type
				if t := o.MediaType(); t != astiav.MediaTypeUnknown {
					return t, nil
				}
			}
		}
	}
	return astiav.MediaTypeUnknown, errors.New("astiavflow: no valid media type found")
}

type FrameFiltererCumulativeStats struct {
	FrameHandlerCumulativeStats
	OutgoingFrames uint64
}

func (ff *FrameFilterer) CumulativeStats() FrameFiltererCumulativeStats {
	return FrameFiltererCumulativeStats{
		FrameHandlerCumulativeStats: ff.frameHandler.cumulativeStats(),
		OutgoingFrames:              atomic.LoadUint64(&ff.fd.cs.outgoingFrames),
	}
}

func (ff *FrameFilterer) DeltaStats() []astikit.DeltaStat {
	ss := ff.frameHandler.deltaStats()
	ss = append(ss, ff.fd.deltaStats()...)
	return ss
}

func (ff *FrameFilterer) On(n astikit.EventName, h astikit.EventHandler) astikit.EventRemover {
	return ff.n.On(n, h)
}

func (ff *FrameFilterer) onConnect(n astiflow.Noder) error {
	// Noder is not among aliases
	if _, ok := ff.aliases[n]; !ok {
		return errors.New("astiavflow: noder is not among aliases")
	}

	// Some aliases have still not connected
	if ff.initialIncomingFrameDescriptors.len() < len(ff.aliases) {
		return nil
	}

	// Refresh filter
	if err := ff.refreshFilter(frameFiltererCreateFilterOptions{incomingFrameDescriptors: ff.initialIncomingFrameDescriptors}); err != nil {
		return fmt.Errorf("astiavflow: refreshing filter failed: %w", err)
	}
	return nil
}

func (ff *FrameFilterer) Connect(h FrameHandler) error {
	// Get filter
	ff.mf.Lock()
	fff := ff.f
	ff.mf.Unlock()

	// No filter
	if fff == nil {
		return errors.New("astiavflow: no filter: frame filerer needs all parents to be connected before connecting children")
	}

	// Callback
	if err := h.OnConnect(fff.outgoingFrameDescriptor, ff); err != nil {
		return fmt.Errorf("astiavflow: callback failed: %w", err)
	}

	// Connect
	ff.n.Connector().Connect(h.NodeConnector())
	return nil
}

func (ff *FrameFilterer) Disconnect(h FrameHandler) {
	// Disconnect
	ff.n.Connector().Disconnect(h.NodeConnector())
}

func (ff *FrameFilterer) Start(ctx context.Context, cancel context.CancelFunc, tc astikit.TaskCreator) {
	tc().Do(func() {
		// In case there are no aliases, we emulate rate
		if len(ff.aliases) == 0 {
			// Loop
			emulatePeriod := time.Duration(ff.emulateRate.Invert().Float64() * 1e9)
			nextAt := astikit.Now()
			for {
				// Sleep until next at
				if delta := nextAt.Sub(astikit.Now()); delta > 0 {
					astikit.Sleep(ctx, delta)
				}

				// Check context
				if ctx.Err() != nil {
					return
				}

				// Loop
				for {
					// Pull filtered frame
					ff.mf.Lock()
					if stop := ff.pullFilteredFrameUnlocked(); stop {
						ff.mf.Unlock()
						break
					}
					ff.mf.Unlock()
				}

				// Get next at
				nextAt = nextAt.Add(emulatePeriod)
			}
		}

		// Start frame handler
		ff.frameHandler.start(ctx)
	})
}

func (ff *FrameFilterer) refreshFilter(o frameFiltererCreateFilterOptions) error {
	// Lock
	ff.mf.Lock()
	defer ff.mf.Unlock()

	// Refresh filter
	return ff.refreshFilterUnlocked(o)
}

func (ff *FrameFilterer) refreshFilterUnlocked(o frameFiltererCreateFilterOptions) (err error) {
	// We need to refresh the filter
	if frameDescriptorsChanged := ff.f != nil && o.incomingFrameDescriptors != nil && !ff.f.incomingFrameDescriptors.equal(o.incomingFrameDescriptors); ff.f == nil || frameDescriptorsChanged {
		// Frame descriptors changed
		if frameDescriptorsChanged && ff.onIncomingFrameDescriptorsDelta != nil {
			// Callback
			ff.onIncomingFrameDescriptorsDelta(ff.f.incomingFrameDescriptors.delta(o.incomingFrameDescriptors, ff.aliases), ff)
		}

		// Close previous filter
		ff.closeFilterUnlocked()

		// Create filter
		if err = ff.createFilterUnlocked(o); err != nil {
			err = fmt.Errorf("astiavflow: creating filter failed: %w", err)
			return
		}
	}
	return nil
}

type frameFiltererCreateFilterOptions struct {
	incomingFrameDescriptors *frameDescriptorPool
	outgoingMediaType        *astiav.MediaType
}

func (ff *FrameFilterer) createFilterUnlocked(o frameFiltererCreateFilterOptions) (err error) {
	// Create filter
	f := newFrameFiltererFilter()

	// Make sure to close filter in case of error
	defer func() {
		if err != nil {
			f.close()
		}
	}()

	// Create grapher
	f.g = newFrameFiltererGrapher()
	classers.set(f.g, ff.n)

	// Make sure grapher is freed
	f.c.Add(f.g.Free)
	f.c.Add(func() { classers.del(f.g) })

	// Set thread parameters
	if ff.threadCount > 0 {
		f.g.SetThreadCount(ff.threadCount)
	}
	if ff.threadType != astiav.ThreadTypeUndefined {
		f.g.SetThreadType(ff.threadType)
	}

	// Get outgoing media type
	outgoingMediaType := astiav.MediaTypeUnknown
	if o.outgoingMediaType != nil {
		outgoingMediaType = *o.outgoingMediaType
	} else if o.incomingFrameDescriptors != nil {
		if fd, ok := o.incomingFrameDescriptors.one(); ok {
			outgoingMediaType = fd.MediaType
		}
	}

	// Create buffersrc func and buffersink
	var buffersrcFunc func() frameFiltererFilterer
	var buffersink frameFiltererFilterer
	switch outgoingMediaType {
	case astiav.MediaTypeAudio:
		buffersrcFunc = func() frameFiltererFilterer { return newFrameFiltererFilterer("abuffer") }
		buffersink = newFrameFiltererFilterer("abuffersink")
	case astiav.MediaTypeVideo:
		buffersrcFunc = func() frameFiltererFilterer { return newFrameFiltererFilterer("buffer") }
		buffersink = newFrameFiltererFilterer("buffersink")
	default:
		err = fmt.Errorf("astiavflow: media type %s is not handled by filterer", outgoingMediaType)
		return
	}

	// No buffersink
	if buffersink == nil {
		err = errors.New("astiavflow: buffersink is nil")
		return
	}

	// Create buffersink context
	if f.buffersinkContext, err = f.g.NewBuffersinkFilterContext(buffersink, "out", nil); err != nil {
		err = fmt.Errorf("astiavflow: creating buffersink context failed: %w", err)
		return
	}
	classers.set(f.buffersinkContext.ptr(), ff.n)

	// Make sure buffersink context is freed
	//!\\ g.buffersinkContext.Free() shouldn't be called as g.g.Free() takes care of it
	f.c.Add(func() { classers.del(f.buffersinkContext.ptr()) })

	// Create inputs
	inputs := newFrameFiltererInOuter()
	inputs.SetName("out")
	inputs.SetFilterContext(f.buffersinkContext)
	inputs.SetPadIdx(0)
	inputs.SetNext(nil)

	// Make sure inputs are freed
	defer inputs.Free()

	// Aliases but no incoming frame descriptors
	if len(ff.aliases) > 0 && o.incomingFrameDescriptors == nil {
		err = errors.New("astiavflow: aliases but no incoming frame descriptors")
		return
	}

	// Create outputs
	var outputs frameFiltererInOuter = nil

	// Make sure outputs are freed
	defer func() {
		if outputs != nil {
			outputs.Free()
		}
	}()

	// Loop through aliases
	for n, alias := range ff.aliases {
		// Create buffersrc
		buffersrc := buffersrcFunc()

		// No buffersrc
		if buffersrc == nil {
			err = errors.New("astiavflow: buffersrc is nil")
			return
		}

		// Get frame descriptor
		fd, ok := o.incomingFrameDescriptors.get(n)
		if !ok {
			err = fmt.Errorf("astiavflow: no frame descriptor for noder with alias %s", alias)
			return
		}

		// Create args
		var args astiav.FilterArgs
		switch fd.MediaType {
		case astiav.MediaTypeAudio:
			args = astiav.FilterArgs{
				"channel_layout": fd.ChannelLayout.String(),
				"sample_fmt":     fd.SampleFormat.String(),
				"sample_rate":    strconv.Itoa(fd.SampleRate),
				"time_base":      fd.MediaDescriptor.TimeBase.String(),
			}
		case astiav.MediaTypeVideo:
			args = astiav.FilterArgs{
				"colorspace": fd.ColorSpace.String(),
				"height":     strconv.Itoa(fd.Height),
				"pix_fmt":    strconv.Itoa(int(fd.PixelFormat)),
				"range":      fd.ColorRange.String(),
				"sar":        fd.SampleAspectRatio.String(),
				"time_base":  fd.MediaDescriptor.TimeBase.String(),
				"width":      strconv.Itoa(fd.Width),
			}
			if fd.MediaDescriptor.FrameRate.Float64() > 0 {
				args["frame_rate"] = fd.MediaDescriptor.FrameRate.String()
			}
		default:
			err = fmt.Errorf("astiavflow: media type %s is not handled by filterer", fd.MediaType)
			return
		}

		// Create buffersrc ctx
		var buffersrcContext frameFiltererBuffersrcContexter
		if buffersrcContext, err = f.g.NewBuffersrcFilterContext(buffersrc, "in", args); err != nil {
			err = fmt.Errorf("astiavflow: creating buffersrc context failed: %w", err)
			return
		}
		classers.set(buffersrcContext.ptr(), ff.n)

		// Make sure buffersrc context is freed
		//!\\ g.buffersrcContext.Free() shouldn't be called as g.g.Free() takes care of it
		f.c.Add(func() { classers.del(buffersrcContext.ptr()) })

		// Create outputs
		o := newFrameFiltererInOuter()
		o.SetName(alias)
		o.SetFilterContext(buffersrcContext)
		o.SetPadIdx(0)
		o.SetNext(outputs)

		// Update outputs
		outputs = o

		// Store source information
		f.buffersrcContexts[n] = buffersrcContext
		f.incomingFrameDescriptors.set(fd, n)
	}

	// Parse content
	if err = f.g.Parse(ff.content, inputs, outputs); err != nil {
		err = fmt.Errorf("astiavflow: parsing content failed: %w", err)
		return
	}

	// Configure graph
	if err = f.g.Configure(); err != nil {
		err = fmt.Errorf("astiavflow: configuring graph failed: %w", err)
		return
	}

	// Create outgoing frame descriptor
	f.outgoingFrameDescriptor = newFrameDescriptorFromFrameFiltererBuffersinkContexter(f.buffersinkContext)
	if ff.emulateRate.Float64() != 0 {
		f.outgoingFrameDescriptor.MediaDescriptor.FrameRate = ff.emulateRate
	}

	// Adapt outgoing frame descriptor
	if ff.outgoingFrameDescriptorAdapter != nil {
		ff.outgoingFrameDescriptorAdapter(&f.outgoingFrameDescriptor)
	}

	// Store filter
	ff.f = f
	return
}

func (ff *FrameFilterer) closeFilter() {
	// Lock
	ff.mf.Lock()
	defer ff.mf.Unlock()

	// Close filter
	ff.closeFilterUnlocked()
}

func (ff *FrameFilterer) closeFilterUnlocked() {
	if ff.f != nil {
		ff.f.close()
		ff.f = nil
	}
}

func (ff *FrameFilterer) onFrame(acquireFrameFunc frameHandlerAcquireFrameFunc, f *astiav.Frame, fd FrameDescriptor, n astiflow.Noder) {
	// Lock
	ff.mf.Lock()
	defer ff.mf.Unlock()

	// Push in the queuer
	ff.q.push(f, fd, n, acquireFrameFunc())

	// Loop
	for {
		// We use a closure to ease closing closer
		if stop := func() bool {
			// Pop out of the queuer
			c, fds, fs, stop := ff.q.pop()
			if stop {
				return true
			}

			// Make sure closer is closed
			defer c.Close()

			// Refresh filter
			if err := ff.refreshFilterUnlocked(frameFiltererCreateFilterOptions{incomingFrameDescriptors: fds}); err != nil {
				dispatchError(ff.n, fmt.Errorf("astiavflow: refreshing filter if needed failed: %w", err).Error())
				return false
			}

			// Loop through frames
			for n, f := range fs {
				// Get buffersrc context
				buffersrcContext, ok := ff.f.buffersrcContexts[n]
				if !ok {
					ff.closeFilterUnlocked()
					dispatchError(ff.n, fmt.Sprintf("astiavflow: no buffersrc context for noder with alias %s", ff.aliases[n]))
					return false
				}

				// Add frame
				if err := buffersrcContext.AddFrame(f, astiav.NewBuffersrcFlags(astiav.BuffersrcFlagKeepRef)); err != nil {
					ff.closeFilterUnlocked()
					dispatchError(ff.n, fmt.Errorf("astiavflow: adding frame to buffersrc of noder with alias %s failed: %w", ff.aliases[n], err).Error())
					return false
				}
			}

			// Loop
			for {
				// Pull filtered frame
				if stop = ff.pullFilteredFrameUnlocked(); stop {
					return false
				}
			}
		}(); stop {
			break
		}
	}
}

func (ff *FrameFilterer) pullFilteredFrameUnlocked() (stop bool) {
	// Get frame
	f := ff.fp.get()
	defer ff.fp.put(f)

	// Pull filtered frame from graph
	if err := ff.f.buffersinkContext.GetFrame(f, astiav.NewBuffersinkFlags()); err != nil {
		if !errors.Is(err, astiav.ErrEof) && !errors.Is(err, astiav.ErrEagain) {
			dispatchError(ff.n, fmt.Errorf("astiavflow: getting frame from buffersink failed: %w", err).Error())
		}
		stop = true
		return
	}

	// Queuer callback
	ff.q.onBeforeDispatch(f)

	// Callback
	if ff.onBeforeDispatch != nil {
		ff.onBeforeDispatch(f)
	}

	// Dispatch frame
	ff.fd.dispatch(Frame{
		Frame:           f,
		FrameDescriptor: ff.f.outgoingFrameDescriptor,
	})
	return
}

type frameFiltererFilter struct {
	buffersinkContext        frameFiltererBuffersinkContexter
	buffersrcContexts        map[astiflow.Noder]frameFiltererBuffersrcContexter
	c                        *astikit.Closer
	incomingFrameDescriptors *frameDescriptorPool
	outgoingFrameDescriptor  FrameDescriptor
	g                        frameFiltererGrapher
}

func newFrameFiltererFilter() *frameFiltererFilter {
	return &frameFiltererFilter{
		buffersrcContexts:        make(map[astiflow.Noder]frameFiltererBuffersrcContexter),
		c:                        astikit.NewCloser(),
		incomingFrameDescriptors: newFrameDescriptorPool(),
	}
}

func (f *frameFiltererFilter) close() {
	f.c.Close()
}

type frameFiltererGrapher interface {
	Class() *astiav.Class
	Configure() error
	Free()
	NewBuffersinkFilterContext(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersinkContexter, error)
	NewBuffersrcFilterContext(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersrcContexter, error)
	Parse(content string, inputs, outputs frameFiltererInOuter) error
	ParseSegment(content string) (frameFiltererSegmenter, error)
	SetThreadCount(int)
	SetThreadType(astiav.ThreadType)
}

var newFrameFiltererGrapher = func() frameFiltererGrapher {
	if g := newDefaultFrameFiltererFilterGraph(astiav.AllocFilterGraph()); g != nil {
		return g
	}
	return nil
}

var _ frameFiltererGrapher = (*defaultFrameFiltererGrapher)(nil)

type defaultFrameFiltererGrapher struct {
	*astiav.FilterGraph
}

func newDefaultFrameFiltererFilterGraph(fg *astiav.FilterGraph) *defaultFrameFiltererGrapher {
	if fg == nil {
		return nil
	}
	return &defaultFrameFiltererGrapher{FilterGraph: fg}
}

func (g *defaultFrameFiltererGrapher) NewBuffersinkFilterContext(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersinkContexter, error) {
	if f == nil {
		return nil, errors.New("empty filter")
	}
	fc, err := g.FilterGraph.NewBuffersinkFilterContext(f.ptr(), name, args)
	if err != nil {
		return nil, err
	}
	return newDefaultFrameFiltererBuffersinkContexter(fc), nil
}

func (g *defaultFrameFiltererGrapher) NewBuffersrcFilterContext(f frameFiltererFilterer, name string, args astiav.FilterArgs) (frameFiltererBuffersrcContexter, error) {
	if f == nil {
		return nil, errors.New("empty filter")
	}
	fc, err := g.FilterGraph.NewBuffersrcFilterContext(f.ptr(), name, args)
	if err != nil {
		return nil, err
	}
	return newDefaultFrameFiltererBuffersrcContexter(fc), nil
}

func (g *defaultFrameFiltererGrapher) Parse(content string, inputs, outputs frameFiltererInOuter) error {
	var is, os *astiav.FilterInOut
	if inputs != nil {
		is = inputs.ptr()
	}
	if outputs != nil {
		os = outputs.ptr()
	}
	return g.FilterGraph.Parse(content, is, os)
}

func (g *defaultFrameFiltererGrapher) ParseSegment(content string) (frameFiltererSegmenter, error) {
	s, err := g.FilterGraph.ParseSegment(content)
	if err != nil {
		return nil, err
	}
	return newDefaultFrameFiltererSegmenter(s), nil
}

type frameFiltererContexter interface {
	ptr() *astiav.FilterContext
}

type frameFiltererBuffersinkContexter interface {
	frameFiltererContexter
	ChannelLayout() astiav.ChannelLayout
	ColorRange() astiav.ColorRange
	ColorSpace() astiav.ColorSpace
	FrameRate() astiav.Rational
	GetFrame(f *astiav.Frame, fs astiav.BuffersinkFlags) error
	Height() int
	MediaType() astiav.MediaType
	PixelFormat() astiav.PixelFormat
	SampleAspectRatio() astiav.Rational
	SampleFormat() astiav.SampleFormat
	SampleRate() int
	TimeBase() astiav.Rational
	Width() int
}

var _ frameFiltererBuffersinkContexter = (*defaultFrameFiltererBuffersinkContexter)(nil)

type defaultFrameFiltererBuffersinkContexter struct {
	*astiav.BuffersinkFilterContext
}

func newDefaultFrameFiltererBuffersinkContexter(bfc *astiav.BuffersinkFilterContext) *defaultFrameFiltererBuffersinkContexter {
	if bfc == nil {
		return nil
	}
	return &defaultFrameFiltererBuffersinkContexter{BuffersinkFilterContext: bfc}
}

func (bfc *defaultFrameFiltererBuffersinkContexter) ptr() *astiav.FilterContext {
	return bfc.BuffersinkFilterContext.FilterContext()
}

type frameFiltererBuffersrcContexter interface {
	frameFiltererContexter
	AddFrame(f *astiav.Frame, fs astiav.BuffersrcFlags) error
}

var _ frameFiltererBuffersrcContexter = (*defaultFrameFiltererBuffersrcContexter)(nil)

type defaultFrameFiltererBuffersrcContexter struct {
	*astiav.BuffersrcFilterContext
}

func newDefaultFrameFiltererBuffersrcContexter(bfc *astiav.BuffersrcFilterContext) *defaultFrameFiltererBuffersrcContexter {
	if bfc == nil {
		return nil
	}
	return &defaultFrameFiltererBuffersrcContexter{BuffersrcFilterContext: bfc}
}

func (bfc *defaultFrameFiltererBuffersrcContexter) ptr() *astiav.FilterContext {
	return bfc.BuffersrcFilterContext.FilterContext()
}

type frameFiltererInOuter interface {
	Free()
	SetFilterContext(frameFiltererContexter)
	SetName(string)
	SetNext(frameFiltererInOuter)
	SetPadIdx(int)

	ptr() *astiav.FilterInOut
}

var newFrameFiltererInOuter = func() frameFiltererInOuter {
	if v := newDefaultFrameFiltererInOuter(astiav.AllocFilterInOut()); v != nil {
		return v
	}
	return nil
}

var _ frameFiltererInOuter = (*defaultFrameFiltererInOuter)(nil)

type defaultFrameFiltererInOuter struct {
	*astiav.FilterInOut
}

func newDefaultFrameFiltererInOuter(fio *astiav.FilterInOut) *defaultFrameFiltererInOuter {
	if fio == nil {
		return nil
	}
	return &defaultFrameFiltererInOuter{FilterInOut: fio}
}

func (fio *defaultFrameFiltererInOuter) SetFilterContext(fc frameFiltererContexter) {
	var fcc *astiav.FilterContext
	if fc != nil {
		fcc = fc.ptr()
	}
	fio.FilterInOut.SetFilterContext(fcc)
}

func (fio *defaultFrameFiltererInOuter) SetNext(n frameFiltererInOuter) {
	var nc *astiav.FilterInOut
	if n != nil {
		nc = n.ptr()
	}
	fio.FilterInOut.SetNext(nc)
}

func (fio *defaultFrameFiltererInOuter) ptr() *astiav.FilterInOut {
	return fio.FilterInOut
}

type frameFiltererFilterer interface {
	Outputs() []frameFiltererPader

	ptr() *astiav.Filter
}

var newFrameFiltererFilterer = func(name string) frameFiltererFilterer {
	if f := newDefaultFrameFiltererFilterer(astiav.FindFilterByName(name)); f != nil {
		return f
	}
	return nil
}

var _ frameFiltererFilterer = (*defaultFrameFiltererFilterer)(nil)

type defaultFrameFiltererFilterer struct {
	*astiav.Filter
}

func newDefaultFrameFiltererFilterer(f *astiav.Filter) *defaultFrameFiltererFilterer {
	if f == nil {
		return nil
	}
	return &defaultFrameFiltererFilterer{Filter: f}
}

func (f defaultFrameFiltererFilterer) Outputs() (ps []frameFiltererPader) {
	for _, o := range f.Filter.Outputs() {
		var v frameFiltererPader = nil
		if o != nil {
			v = o
		}
		ps = append(ps, v)
	}
	return
}

func (f defaultFrameFiltererFilterer) ptr() *astiav.Filter {
	return f.Filter
}

type frameFiltererPader interface {
	MediaType() astiav.MediaType
}

type frameFiltererSegmenter interface {
	Chains() []frameFiltererChainer
	Free()
}

var _ frameFiltererSegmenter = (*defaultFrameFiltererSegmenter)(nil)

type defaultFrameFiltererSegmenter struct {
	*astiav.FilterGraphSegment
}

func newDefaultFrameFiltererSegmenter(s *astiav.FilterGraphSegment) *defaultFrameFiltererSegmenter {
	if s == nil {
		return nil
	}
	return &defaultFrameFiltererSegmenter{FilterGraphSegment: s}
}

func (s *defaultFrameFiltererSegmenter) Chains() (cs []frameFiltererChainer) {
	for _, c := range s.FilterGraphSegment.Chains() {
		var v frameFiltererChainer = nil
		if t := newDefaultFrameFiltererChainer(c); t != nil {
			v = t
		}
		cs = append(cs, v)
	}
	return
}

var _ frameFiltererChainer = (*defaultFrameFiltererChainer)(nil)

type frameFiltererChainer interface {
	Filters() []frameFiltererParamers
}

type defaultFrameFiltererChainer struct {
	*astiav.FilterChain
}

func newDefaultFrameFiltererChainer(c *astiav.FilterChain) *defaultFrameFiltererChainer {
	if c == nil {
		return nil
	}
	return &defaultFrameFiltererChainer{FilterChain: c}
}

func (c *defaultFrameFiltererChainer) Filters() (ps []frameFiltererParamers) {
	for _, f := range c.FilterChain.Filters() {
		var v frameFiltererParamers = nil
		if f != nil {
			v = f
		}
		ps = append(ps, v)
	}
	return
}

type frameFiltererParamers interface {
	FilterName() string
}

type frameFiltererQueuer interface {
	onBeforeDispatch(f *astiav.Frame)
	pop() (c *astikit.Closer, fds *frameDescriptorPool, fs map[astiflow.Noder]*astiav.Frame, stop bool)
	push(f *astiav.Frame, fd FrameDescriptor, n astiflow.Noder, releaseFrameFunc frameHandlerReleaseFrameFunc)
}

var _ frameFiltererQueuer = (*defaultFrameFiltererQueuer)(nil)

type defaultFrameFiltererQueuer struct {
	aliasesCount int
	slots        []map[astiflow.Noder]*defaultFrameFiltererQueuerItem
}

type defaultFrameFiltererQueuerItem struct {
	f                *astiav.Frame
	fd               FrameDescriptor
	releaseFrameFunc frameHandlerReleaseFrameFunc
}

func newDefaultFrameFiltererQueuer(aliasesCount int) *defaultFrameFiltererQueuer {
	return &defaultFrameFiltererQueuer{aliasesCount: aliasesCount}
}

func (q *defaultFrameFiltererQueuer) onBeforeDispatch(f *astiav.Frame) {}

func (q *defaultFrameFiltererQueuer) pop() (c *astikit.Closer, fds *frameDescriptorPool, fs map[astiflow.Noder]*astiav.Frame, stop bool) {
	// Initialize
	c = astikit.NewCloser()
	fds = newFrameDescriptorPool()
	fs = make(map[astiflow.Noder]*astiav.Frame)

	// First slot is not complete
	if len(q.slots) == 0 || len(q.slots[0]) < q.aliasesCount {
		stop = true
		return
	}

	// Loop through first slot's items
	for n, qi := range q.slots[0] {
		// Update frame descriptor pool
		fds.set(qi.fd, n)

		// Update frame pool
		fs[n] = qi.f

		// Make sure to release frame
		c.Add((func())(qi.releaseFrameFunc))
	}

	// Remove first slot
	q.slots = q.slots[1:]
	return
}

func (q *defaultFrameFiltererQueuer) push(f *astiav.Frame, fd FrameDescriptor, n astiflow.Noder, releaseFrameFunc frameHandlerReleaseFrameFunc) {
	// Create item
	qi := &defaultFrameFiltererQueuerItem{
		f:                f,
		fd:               fd,
		releaseFrameFunc: releaseFrameFunc,
	}

	// Loop through slots
	for i := 0; i < len(q.slots); i++ {
		// Noder is already in slot
		if _, ok := q.slots[i][n]; ok {
			continue
		}

		// Insert item
		q.slots[i][n] = qi
		return
	}

	// Item has not been inserted, we need to append a new slot with the item in it
	q.slots = append(q.slots, map[astiflow.Noder]*defaultFrameFiltererQueuerItem{n: qi})
}

var _ frameFiltererQueuer = (*ptsFrameFiltererQueuer)(nil)

type ptsFrameFiltererQueuer struct {
	aliasesCount int
	lastPopPts   int64
	slots        []*ptsFrameFiltererQueuerSlot
}

type ptsFrameFiltererQueuerSlot struct {
	items map[astiflow.Noder]*ptsFrameFiltererQueuerItem
	pts   int64
}

type ptsFrameFiltererQueuerItem struct {
	f                *astiav.Frame
	fd               FrameDescriptor
	releaseFrameFunc frameHandlerReleaseFrameFunc
}

func newPTSFrameFiltererQueuer(aliasesCount int) *ptsFrameFiltererQueuer {
	return &ptsFrameFiltererQueuer{aliasesCount: aliasesCount}
}

func (q *ptsFrameFiltererQueuer) onBeforeDispatch(f *astiav.Frame) {
	f.SetPts(q.lastPopPts)
}

func (q *ptsFrameFiltererQueuer) pop() (c *astikit.Closer, fds *frameDescriptorPool, fs map[astiflow.Noder]*astiav.Frame, stop bool) {
	// Initialize
	c = astikit.NewCloser()
	fds = newFrameDescriptorPool()
	fs = make(map[astiflow.Noder]*astiav.Frame)

	// Loop through slots
	for i, qs := range q.slots {
		// Check whether slot is complete
		complete := len(qs.items) == q.aliasesCount

		// Loop through items
		for n, qi := range qs.items {
			// Slot is complete
			if complete {
				// Update frame descriptor pool
				fds.set(qi.fd, n)

				// Update frame pool
				fs[n] = qi.f
			}

			// Make sure to release frame
			c.Add((func())(qi.releaseFrameFunc))
		}

		// Slot is complete
		if complete {
			// Update last pop pts
			q.lastPopPts = qs.pts

			// Remove slots
			q.slots = q.slots[i+1:]
			return
		}
	}
	stop = true
	return
}

func (q *ptsFrameFiltererQueuer) push(f *astiav.Frame, fd FrameDescriptor, n astiflow.Noder, releaseFrameFunc frameHandlerReleaseFrameFunc) {
	// Create item and slot
	qi := &ptsFrameFiltererQueuerItem{
		f:                f,
		fd:               fd,
		releaseFrameFunc: releaseFrameFunc,
	}
	qs := &ptsFrameFiltererQueuerSlot{
		items: map[astiflow.Noder]*ptsFrameFiltererQueuerItem{n: qi},
		pts:   f.Pts(),
	}

	// Loop through slots
	for i := 0; i < len(q.slots); i++ {
		// Check pts
		if f.Pts() > q.slots[i].pts {
			continue
		} else if f.Pts() == q.slots[i].pts {
			q.slots[i].items[n] = qi
		} else {
			q.slots = append(q.slots[:i], append([]*ptsFrameFiltererQueuerSlot{qs}, q.slots[i:]...)...)
		}
		return
	}

	// Item has not been inserted, we need to append a new slot with the item in it
	q.slots = append(q.slots, qs)
}
