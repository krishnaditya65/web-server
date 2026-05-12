package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/krishnaditya65/web-server/internal/config"
	"github.com/krishnaditya65/web-server/internal/health"
	"github.com/krishnaditya65/web-server/internal/lb"
	"github.com/krishnaditya65/web-server/internal/types"
)

type Proxy struct {
	lb lb.Balancer
}

func New(cfg *config.Config) *httputil.ReverseProxy {
	var upstreams []*types.Upstream

	for _, u := range cfg.Proxy.Upstreams {
		parsed, err := url.Parse(u)
		if err != nil {
			log.Printf("invalid upstream URL skipped: %s", u)
			continue
		}

		upstream := &types.Upstream{
			URL:    parsed,
			Weight: 1,
		}

		// optimistic startup assumption
		upstream.Healthy.Store(true)

		upstreams = append(upstreams, upstream)
	}

	health.StartHealthChecks(
		upstreams,
		cfg.Health.Path,
		time.Duration(cfg.Health.IntervalSeconds)*time.Second,
		time.Duration(cfg.Health.TimeoutSeconds)*time.Second,
	)

	balancer := lb.NewRoundRobin(upstreams)

	p := &Proxy{
		lb: balancer,
	}

	transport := &http.Transport{
		MaxIdleConns:          1000,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}

	return &httputil.ReverseProxy{
		Director:  p.director,
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("proxy error: %v", err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}
}

func (p *Proxy) director(req *http.Request) {
	target, err := p.lb.Next()
	if err != nil {
		log.Printf("load balancer error: %v", err)

		req.URL.Scheme = "http"
		req.URL.Host = "invalid.local"

		return
	}

	req.URL.Scheme = target.URL.Scheme
	req.URL.Host = target.URL.Host
	req.Host = target.URL.Host

	req.Header.Set("X-Forwarded-Host", req.Host)
	req.Header.Set("X-Forwarded-Proto", req.URL.Scheme)
}
