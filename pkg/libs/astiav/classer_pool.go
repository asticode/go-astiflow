package astiavflow

import (
	"sync"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiflow/pkg/astiflow"
)

var classers = newClasserPool()

type classerPool struct {
	m sync.Mutex
	p map[astiav.Classer]*astiflow.Node
}

func newClasserPool() *classerPool {
	return &classerPool{p: make(map[astiav.Classer]*astiflow.Node)}
}

func (p *classerPool) set(c astiav.Classer, n *astiflow.Node) {
	p.m.Lock()
	defer p.m.Unlock()
	p.p[c] = n
}

func (p *classerPool) del(c astiav.Classer) {
	p.m.Lock()
	defer p.m.Unlock()
	delete(p.p, c)
}

func (p *classerPool) get(c astiav.Classer) (*astiflow.Node, bool) {
	p.m.Lock()
	defer p.m.Unlock()
	n, ok := p.p[c]
	return n, ok
}
