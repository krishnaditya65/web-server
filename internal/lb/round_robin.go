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
	return &RoundRobin{targets: targets}
}

func (r *RoundRobin) Next() (*types.Upstream, error) {
	var healthy []*types.Upstream

	for _, t := range r.targets {
		if !t.Healthy.Load() {
			continue
		}

		switch t.State() {
		case types.CircuitOpen:
			if !t.TryHalfOpen() {
				continue
			}
		case types.CircuitHalfOpen:
			if !t.TryHalfOpen() {
				continue
			}
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

	if target.State() == types.CircuitHalfOpen {
		target.RecordSuccess()
	}
}

func (r *RoundRobin) MarkFailure(target *types.Upstream) {
	target.RecordFailure()
}

func (r *RoundRobin) Upstreams() []*types.Upstream {
	return r.targets
}
