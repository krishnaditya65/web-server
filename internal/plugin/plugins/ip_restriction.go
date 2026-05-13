package plugins

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/krishnaditya65/web-server/internal/plugin"
)

// IPRestrictionPlugin allows or denies requests based on client IP.
// Config keys:
//
//	"allow"  []string  (CIDR ranges or exact IPs; if non-empty, only these pass)
//	"deny"   []string  (CIDR ranges or exact IPs to always block)
type IPRestrictionPlugin struct{}

func (IPRestrictionPlugin) Name() string { return "ip-restriction" }

func (IPRestrictionPlugin) New(cfg map[string]interface{}) (plugin.Middleware, error) {
	allowNets, err := parseCIDRList(cfg, "allow")
	if err != nil {
		return nil, fmt.Errorf("ip-restriction: allow: %w", err)
	}

	denyNets, err := parseCIDRList(cfg, "deny")
	if err != nil {
		return nil, fmt.Errorf("ip-restriction: deny: %w", err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := parseClientIP(r)

			if containsIP(denyNets, ip) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			if len(allowNets) > 0 && !containsIP(allowNets, ip) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

func parseCIDRList(cfg map[string]interface{}, key string) ([]*net.IPNet, error) {
	raw, _ := cfg[key].([]interface{})
	var nets []*net.IPNet

	for _, entry := range raw {
		s, ok := entry.(string)
		if !ok {
			continue
		}

		s = strings.TrimSpace(s)

		// Treat bare IPs as /32 or /128.
		if !strings.Contains(s, "/") {
			if strings.Contains(s, ":") {
				s += "/128"
			} else {
				s += "/32"
			}
		}

		_, ipNet, err := net.ParseCIDR(s)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", s, err)
		}

		nets = append(nets, ipNet)
	}

	return nets, nil
}

func containsIP(nets []*net.IPNet, ip net.IP) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}

	return false
}

func parseClientIP(r *http.Request) net.IP {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
			return ip
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.ParseIP(r.RemoteAddr)
	}

	return net.ParseIP(host)
}
