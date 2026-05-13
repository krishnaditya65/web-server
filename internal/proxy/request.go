package proxy

import (
	"context"
	"net"
	"net/http"

	"github.com/krishnaditya65/web-server/internal/types"
)

func cloneRequest(ctx context.Context, in *http.Request) *http.Request {
	out := in.Clone(ctx)

	// Clone does shallow-copy Body reference.
	// Fine for phase 1 because we only retry replay-safe requests.
	out.Header = in.Header.Clone()

	return out
}

func rewriteRequest(out *http.Request, upstream *types.Upstream) {
	out.URL.Scheme = upstream.URL.Scheme
	out.URL.Host = upstream.URL.Host

	// nginx-style behavior:
	// upstream sees upstream host, not gateway host
	out.Host = upstream.URL.Host
}

func prepareOutboundRequest(out *http.Request) {
	removeHopByHopHeaders(out.Header)
	appendForwardHeaders(out)
}

func appendForwardHeaders(r *http.Request) {
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		prior := r.Header.Get("X-Forwarded-For")

		if prior == "" {
			r.Header.Set("X-Forwarded-For", ip)
		} else {
			r.Header.Set("X-Forwarded-For", prior+", "+ip)
		}
	}

	if r.TLS != nil {
		r.Header.Set("X-Forwarded-Proto", "https")
	} else {
		r.Header.Set("X-Forwarded-Proto", "http")
	}

	if r.Host != "" {
		r.Header.Set("X-Forwarded-Host", r.Host)
	}
}
