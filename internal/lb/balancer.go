package lb

import "github.com/krishnaditya65/web-server/internal/types"

type Balancer interface {
	Next() (*types.Upstream, error)
}
