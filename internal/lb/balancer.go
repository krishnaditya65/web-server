package lb

import "net/url"

type Balancer interface {
	Next() (*url.URL, error)
}
