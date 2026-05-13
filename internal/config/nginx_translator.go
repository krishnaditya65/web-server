package config

import (
	"fmt"
	"strconv"
	"strings"
)

// FromNginx translates a parsed nginx AST into a *Config.
// Unknown directives are silently skipped so partial nginx.conf files work.
func FromNginx(nodes []*nginxNode) (*Config, error) {
	cfg := &Config{}

	// Named upstream groups defined at the top level.
	upstreamGroups := map[string][]UpstreamConfig{}
	upstreamAlgorithms := map[string]string{}

	for _, node := range nodes {
		switch node.Directive {
		case "upstream":
			if len(node.Args) == 0 {
				continue
			}
			name := node.Args[0]
			upstreams, algo := parseUpstreamBlock(node.Block)
			upstreamGroups[name] = upstreams
			if algo != "" {
				upstreamAlgorithms[name] = algo
			}

		case "server":
			if err := applyServerBlock(cfg, node.Block, upstreamGroups, upstreamAlgorithms); err != nil {
				return nil, err
			}
		}
	}

	return cfg, nil
}

// parseUpstreamBlock extracts server entries and the load-balancing algorithm
// from an nginx upstream { ... } block.
func parseUpstreamBlock(nodes []*nginxNode) ([]UpstreamConfig, string) {
	var upstreams []UpstreamConfig
	algorithm := ""

	for _, n := range nodes {
		switch n.Directive {
		case "server":
			if len(n.Args) == 0 {
				continue
			}
			addr := n.Args[0]
			// Ensure the URL has a scheme.
			if !strings.Contains(addr, "://") {
				addr = "http://" + addr
			}
			uc := UpstreamConfig{URL: addr, Weight: 1}

			for _, arg := range n.Args[1:] {
				if strings.HasPrefix(arg, "weight=") {
					if w, err := strconv.Atoi(strings.TrimPrefix(arg, "weight=")); err == nil {
						uc.Weight = w
					}
				}
			}

			upstreams = append(upstreams, uc)

		case "least_conn":
			algorithm = "least_conn"
		case "ip_hash":
			algorithm = "round_robin" // closest equivalent
		case "random":
			algorithm = "round_robin"
		}
	}

	return upstreams, algorithm
}

// applyServerBlock maps nginx server { ... } directives onto *Config.
func applyServerBlock(
	cfg *Config,
	nodes []*nginxNode,
	upstreamGroups map[string][]UpstreamConfig,
	upstreamAlgorithms map[string]string,
) error {
	var serverHost string
	var serverMaxBodyBytes int64

	for _, n := range nodes {
		switch n.Directive {

		case "listen":
			if len(n.Args) == 0 {
				continue
			}
			portStr := n.Args[0]
			isTLS := len(n.Args) > 1 && n.Args[1] == "ssl"
			port, err := parsePort(portStr)
			if err != nil {
				continue
			}
			if isTLS {
				cfg.Server.HTTPSPort = port
			} else {
				cfg.Server.Port = port
			}

		case "server_name":
			if len(n.Args) > 0 && n.Args[0] != "_" {
				serverHost = n.Args[0]
			}

		case "ssl_certificate":
			if len(n.Args) > 0 {
				cfg.Server.TLSCertFile = n.Args[0]
			}

		case "ssl_certificate_key":
			if len(n.Args) > 0 {
				cfg.Server.TLSKeyFile = n.Args[0]
			}

		case "client_max_body_size":
			if len(n.Args) > 0 {
				serverMaxBodyBytes = parseSize(n.Args[0])
			}

		case "gzip":
			if len(n.Args) > 0 && n.Args[0] == "on" {
				cfg.Gzip.Enabled = true
			}

		case "gzip_min_length":
			if len(n.Args) > 0 {
				if v, err := strconv.Atoi(n.Args[0]); err == nil {
					cfg.Gzip.MinLength = v
				}
			}

		case "gzip_types":
			cfg.Gzip.ContentTypes = append(cfg.Gzip.ContentTypes, n.Args...)

		case "gzip_comp_level":
			if len(n.Args) > 0 {
				if v, err := strconv.Atoi(n.Args[0]); err == nil {
					cfg.Gzip.Level = v
				}
			}

		case "limit_req_zone":
			// e.g.: limit_req_zone $binary_remote_addr zone=name:10m rate=20r/s
			cfg.Rate.RequestsPerSecond = parseRate(n.Args)
			if cfg.Rate.Burst == 0 {
				cfg.Rate.Burst = int(cfg.Rate.RequestsPerSecond) * 5
			}

		case "location":
			if len(n.Args) == 0 {
				continue
			}
			pathPrefix := n.Args[0]
			// Skip named locations (@ prefix) and regex locations.
			if strings.HasPrefix(pathPrefix, "@") ||
				pathPrefix == "~" || pathPrefix == "~*" || pathPrefix == "=" {
				continue
			}
			// Strip modifier if present (=, ^~).
			if pathPrefix == "^~" || pathPrefix == "=" {
				if len(n.Args) > 1 {
					pathPrefix = n.Args[1]
				} else {
					continue
				}
			}

			route, groupName, err := parseLocationBlock(pathPrefix, serverHost, serverMaxBodyBytes, n.Block, upstreamGroups, upstreamAlgorithms)
			if err != nil {
				return err
			}
			if route != nil {
				cfg.Proxy.Routes = append(cfg.Proxy.Routes, *route)

				// Propagate the load-balancing algorithm from the upstream group.
				if cfg.Proxy.Algorithm == "" && groupName != "" {
					if algo, ok := upstreamAlgorithms[groupName]; ok {
						cfg.Proxy.Algorithm = algo
					}
				}
			}
		}
	}

	return nil
}

// parseLocationBlock maps a nginx location { ... } block to a RouteConfig.
// Also returns the upstream group name (if proxy_pass referenced a named group)
// so the caller can propagate the load-balancing algorithm.
func parseLocationBlock(
	pathPrefix string,
	serverHost string,
	serverMaxBodyBytes int64,
	nodes []*nginxNode,
	upstreamGroups map[string][]UpstreamConfig,
	upstreamAlgorithms map[string]string,
) (*RouteConfig, string, error) {
	route := &RouteConfig{
		Name:         strings.TrimPrefix(strings.ReplaceAll(pathPrefix, "/", "-"), "-"),
		PathPrefix:   pathPrefix,
		Host:         serverHost,
		MaxBodyBytes: serverMaxBodyBytes,
	}

	if route.Name == "" {
		route.Name = "root"
	}

	var resolvedGroupName string

	for _, n := range nodes {
		switch n.Directive {

		case "proxy_pass":
			if len(n.Args) == 0 {
				continue
			}
			target := n.Args[0]
			// Strip trailing slash from proxy_pass URL.
			target = strings.TrimRight(target, "/")

			gn := extractGroupName(target)
			if group, ok := upstreamGroups[gn]; ok {
				route.Upstreams = group
				resolvedGroupName = gn
			} else {
				// Direct URL — single upstream.
				if !strings.Contains(target, "://") {
					target = "http://" + target
				}
				route.Upstreams = []UpstreamConfig{{URL: target, Weight: 1}}
			}

		case "proxy_read_timeout":
			if len(n.Args) > 0 {
				route.TimeoutSeconds = parseDurationSeconds(n.Args[0])
			}

		case "proxy_connect_timeout":
			// Maps to transport dial timeout, which is global — ignore per-route.

		case "client_max_body_size":
			if len(n.Args) > 0 {
				route.MaxBodyBytes = parseSize(n.Args[0])
			}

		case "limit_req":
			// e.g.: limit_req zone=name burst=50
			burst := parseLimitReqBurst(n.Args)
			if burst > 0 {
				route.Plugins = append(route.Plugins, PluginConfig{
					Name:    "rate-limit",
					Enabled: true,
					Config: map[string]interface{}{
						"requests_per_second": float64(burst) / 2.0,
						"burst":               burst,
					},
				})
			}

		case "proxy_set_header":
			// Collect add_headers for request transformer plugin.
			if len(n.Args) >= 2 {
				headerVal := strings.Join(n.Args[1:], " ")
				// Skip nginx variables like $host — not translatable.
				if !strings.Contains(headerVal, "$") {
					route.Plugins = appendRequestTransformer(route.Plugins, n.Args[0], headerVal)
				}
			}
		}
	}

	if len(route.Upstreams) == 0 {
		return nil, "", nil // location without proxy_pass — skip
	}

	return route, resolvedGroupName, nil
}

// ---- helpers ---------------------------------------------------------------

// extractGroupName returns the hostname from a URL, used to look up upstream groups.
// e.g. "http://my_backend" → "my_backend"
func extractGroupName(rawURL string) string {
	s := rawURL
	if idx := strings.Index(s, "://"); idx >= 0 {
		s = s[idx+3:]
	}
	// Strip path.
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	// Strip port.
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		// Only strip if it looks like a port (all digits after colon).
		port := s[idx+1:]
		if _, err := strconv.Atoi(port); err != nil {
			return s // has a colon but it's not a port (e.g. IPv6)
		}
		// It has a real port — keep full host:port as group name isn't likely.
		return s
	}

	return s
}

// parsePort extracts an integer port from strings like "8080" or "8443".
func parsePort(s string) (int, error) {
	// Strip trailing "ssl" or other tokens already handled by the caller.
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty port string")
	}
	return strconv.Atoi(parts[0])
}

// parseSize converts nginx size strings (10m, 512k, 1g) to bytes.
func parseSize(s string) int64 {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "0" {
		return 0
	}

	multiplier := int64(1)
	numStr := s

	switch s[len(s)-1] {
	case 'k':
		multiplier = 1024
		numStr = s[:len(s)-1]
	case 'm':
		multiplier = 1024 * 1024
		numStr = s[:len(s)-1]
	case 'g':
		multiplier = 1024 * 1024 * 1024
		numStr = s[:len(s)-1]
	}

	v, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0
	}

	return v * multiplier
}

// parseDurationSeconds converts nginx time strings (10s, 30, 2m) to integer seconds.
func parseDurationSeconds(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0
	}

	multiplier := 1
	numStr := s

	switch s[len(s)-1] {
	case 's':
		numStr = s[:len(s)-1]
	case 'm':
		multiplier = 60
		numStr = s[:len(s)-1]
	case 'h':
		multiplier = 3600
		numStr = s[:len(s)-1]
	case 'd':
		multiplier = 86400
		numStr = s[:len(s)-1]
	}

	v, err := strconv.Atoi(numStr)
	if err != nil {
		return 0
	}

	return v * multiplier
}

// parseRate extracts requests-per-second from limit_req_zone args.
// e.g. ["$binary_remote_addr", "zone=api:10m", "rate=20r/s"] → 20.0
func parseRate(args []string) float64 {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "rate=") {
			continue
		}

		val := strings.TrimPrefix(arg, "rate=")

		// Strip "/s", "/m", "/h".
		var divisor float64 = 1

		switch {
		case strings.HasSuffix(val, "r/m"):
			val = strings.TrimSuffix(val, "r/m")
			divisor = 60
		case strings.HasSuffix(val, "r/s"):
			val = strings.TrimSuffix(val, "r/s")
		case strings.HasSuffix(val, "/s"):
			val = strings.TrimSuffix(val, "/s")
		case strings.HasSuffix(val, "/m"):
			val = strings.TrimSuffix(val, "/m")
			divisor = 60
		}

		if n, err := strconv.ParseFloat(val, 64); err == nil {
			return n / divisor
		}
	}

	return 0
}

// parseLimitReqBurst extracts the burst value from limit_req args.
// e.g. ["zone=api", "burst=50"] → 50
func parseLimitReqBurst(args []string) int {
	for _, arg := range args {
		if strings.HasPrefix(arg, "burst=") {
			if v, err := strconv.Atoi(strings.TrimPrefix(arg, "burst=")); err == nil {
				return v
			}
		}
	}

	return 0
}

// appendRequestTransformer adds a header to an existing request-header-transformer
// plugin config on the route, or creates one if absent.
func appendRequestTransformer(plugins []PluginConfig, header, value string) []PluginConfig {
	for i := range plugins {
		if plugins[i].Name == "request-header-transformer" {
			m, _ := plugins[i].Config["add_headers"].(map[string]interface{})
			if m == nil {
				m = map[string]interface{}{}
			}
			m[header] = value
			plugins[i].Config["add_headers"] = m
			return plugins
		}
	}

	return append(plugins, PluginConfig{
		Name:    "request-header-transformer",
		Enabled: true,
		Config: map[string]interface{}{
			"add_headers": map[string]interface{}{
				header: value,
			},
		},
	})
}
