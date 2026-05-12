package lb

import (
	"errors"
	"net/url"
	"sync/atomic"
)

type RoundRobin struct {
	targets []*url.URL
	counter uint64
}

func NewRoundRobin(targets []*url.URL) *RoundRobin {
	return &RoundRobin{
		targets: targets,
	}
}

func (r *RoundRobin) Next() (*url.URL, error) {
	if len(r.targets) == 0 {
		return nil, errors.New("no upstreams available")
	}

	i := atomic.AddUint64(&r.counter, 1)
	return r.targets[i%uint64(len(r.targets))], nil
}
