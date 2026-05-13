package types

import (
	"net/url"
	"sync/atomic"
	"time"
)

type Upstream struct {
	URL *url.URL

	Healthy atomic.Bool

	ActiveConns atomic.Int64

	Weight int

	FailureCount atomic.Int64

	CircuitOpenUntil atomic.Int64
}

func (u *Upstream) CircuitOpen() bool {
	until := u.CircuitOpenUntil.Load()

	if until == 0 {
		return false
	}

	return time.Now().Unix() < until
}

func (u *Upstream) OpenCircuit(duration time.Duration) {
	u.CircuitOpenUntil.Store(
		time.Now().Add(duration).Unix(),
	)
}
