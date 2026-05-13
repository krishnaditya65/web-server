package proxy

import "net/http"

var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Proxy-Connection":    {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

func removeHopByHopHeaders(h http.Header) {
	for k := range hopByHopHeaders {
		h.Del(k)
	}

	// RFC 7230:
	// headers named in Connection must also be removed
	for _, connectionValue := range h.Values("Connection") {
		for _, token := range splitHeaderTokens(connectionValue) {
			h.Del(token)
		}
	}
}

func splitHeaderTokens(v string) []string {
	var tokens []string
	start := 0

	for i := 0; i <= len(v); i++ {
		if i == len(v) || v[i] == ',' {
			token := http.CanonicalHeaderKey(trimSpaces(v[start:i]))
			if token != "" {
				tokens = append(tokens, token)
			}
			start = i + 1
		}
	}

	return tokens
}

func trimSpaces(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}

	return s[start:end]
}
