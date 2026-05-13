package lb

import (
	"errors"
	"sync/atomic"
	"time"

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
		if !t.Healthy.Load() {
			continue
		}

		if t.CircuitOpen() {
			continue
		}

		healthy = append(healthy, t)
	}

	if len(healthy) == 0 {
		return nil, errors.New("no healthy upstreams")
	}

	i := atomic.AddUint64(&r.counter, 1)

	target := healthy[i%uint64(len(healthy))]
	target.ActiveConns.Add(1)

	return target, nil
}

func (r *RoundRobin) Release(target *types.Upstream) {
	target.ActiveConns.Add(-1)
}

func (r *RoundRobin) MarkFailure(target *types.Upstream) {
	target.FailureCount.Add(1)

	if target.FailureCount.Load() >= 3 {
		target.Healthy.Store(false)
		target.OpenCircuit(30 * time.Second)
	}
}

func (r *RoundRobin) Upstreams() []*types.Upstream {
	return r.targets
}
