package lb

import (
	"errors"

	"github.com/krishnaditya65/web-server/internal/types"
)

type LeastConnections struct {
	targets []*types.Upstream
}

func NewLeastConnections(targets []*types.Upstream) *LeastConnections {
	return &LeastConnections{targets: targets}
}

func (l *LeastConnections) Next() (*types.Upstream, error) {
	var selected *types.Upstream

	for _, t := range l.targets {
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

		if selected == nil || t.ActiveConns.Load() < selected.ActiveConns.Load() {
			selected = t
		}
	}

	if selected == nil {
		return nil, errors.New("no healthy upstreams")
	}

	selected.ActiveConns.Add(1)

	return selected, nil
}

func (l *LeastConnections) Release(target *types.Upstream) {
	target.ActiveConns.Add(-1)

	if target.State() == types.CircuitHalfOpen {
		target.RecordSuccess()
	}
}

func (l *LeastConnections) MarkFailure(target *types.Upstream) {
	target.RecordFailure()
}

func (l *LeastConnections) Upstreams() []*types.Upstream {
	return l.targets
}
