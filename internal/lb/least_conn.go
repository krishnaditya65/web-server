package lb

import (
	"errors"
	"time"

	"github.com/krishnaditya65/web-server/internal/types"
)

type LeastConnections struct {
	targets []*types.Upstream
}

func NewLeastConnections(targets []*types.Upstream) *LeastConnections {
	return &LeastConnections{
		targets: targets,
	}
}

func (l *LeastConnections) Next() (*types.Upstream, error) {
	var selected *types.Upstream

	for _, t := range l.targets {
		if !t.Healthy.Load() {
			continue
		}

		if t.CircuitOpen() {
			continue
		}

		if selected == nil {
			selected = t
			continue
		}

		if t.ActiveConns.Load() < selected.ActiveConns.Load() {
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
}

func (l *LeastConnections) MarkFailure(target *types.Upstream) {
	target.FailureCount.Add(1)

	if target.FailureCount.Load() >= 3 {
		target.Healthy.Store(false)
		target.OpenCircuit(30 * time.Second)
	}
}

func (l *LeastConnections) Upstreams() []*types.Upstream {
	return l.targets
}
