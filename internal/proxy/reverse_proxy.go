package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/krishnaditya65/web-server/internal/config"
	"github.com/krishnaditya65/web-server/internal/lb"
)

type Proxy struct {
	lb lb.Balancer
}

func New(cfg *config.Config) *httputil.ReverseProxy {
	var urls []*url.URL

	for _, u := range cfg.Proxy.Upstreams {
		parsed, err := url.Parse(u)
		if err == nil {
			urls = append(urls, parsed)
		}
	}

	balancer := lb.NewRoundRobin(urls)

	p := &Proxy{
		lb: balancer,
	}

	return &httputil.ReverseProxy{
		Director: p.director,
	}
}

func (p *Proxy) director(req *http.Request) {
	target, err := p.lb.Next()
	if err != nil {
		return
	}

	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	req.Host = target.Host
}
