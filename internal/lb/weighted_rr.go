package lb

import (
	"errors"
	"sync/atomic"

	"github.com/krishnaditya65/web-server/internal/types"
)

type WeightedRoundRobin struct {
	targets []*types.Upstream
	counter uint64
}

func NewWeightedRoundRobin(targets []*types.Upstream) *WeightedRoundRobin {
	return &WeightedRoundRobin{targets: targets}
}

func (w *WeightedRoundRobin) Next() (*types.Upstream, error) {
	var expanded []*types.Upstream

	for _, t := range w.targets {
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

		repeat := t.Weight
		if repeat <= 0 {
			repeat = 1
		}

		for i := 0; i < repeat; i++ {
			expanded = append(expanded, t)
		}
	}

	if len(expanded) == 0 {
		return nil, errors.New("no healthy upstreams")
	}

	i := atomic.AddUint64(&w.counter, 1)
	target := expanded[i%uint64(len(expanded))]
	target.ActiveConns.Add(1)

	return target, nil
}

func (w *WeightedRoundRobin) Release(target *types.Upstream) {
	target.ActiveConns.Add(-1)

	if target.State() == types.CircuitHalfOpen {
		target.RecordSuccess()
	}
}

func (w *WeightedRoundRobin) MarkFailure(target *types.Upstream) {
	target.RecordFailure()
}

func (w *WeightedRoundRobin) Upstreams() []*types.Upstream {
	return w.targets
}
