package lb

import (
	"errors"
	"sync/atomic"

	"github.com/krishnaditya65/web-server/internal/types"
)

type RoundRobin struct {
	targets []*types.Upstream
	counter uint64
}

func NewRoundRobin(targets []*types.Upstream) *RoundRobin {
	return &RoundRobin{
		targets: targets,
	}
}

func (r *RoundRobin) Next() (*types.Upstream, error) {
	var healthy []*types.Upstream

	for _, t := range r.targets {
		if t.Healthy.Load() {
			healthy = append(healthy, t)
		}
	}

	if len(healthy) == 0 {
		return nil, errors.New("no healthy upstreams")
	}

	i := atomic.AddUint64(&r.counter, 1)

	return healthy[i%uint64(len(healthy))], nil
}
