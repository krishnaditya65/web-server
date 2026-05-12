package types

import (
	"net/url"
	"sync/atomic"
)

type Upstream struct {
	URL         *url.URL
	Healthy     atomic.Bool
	ActiveConns atomic.Int64
	Weight      int
}
