package proxy

import (
	"net/http"
	"strings"
)

func isWebSocketUpgrade(r *http.Request) bool {
	if !headerContainsToken(r.Header, "Connection", "upgrade") {
		return false
	}

	return strings.EqualFold(
		r.Header.Get("Upgrade"),
		"websocket",
	)
}

func headerContainsToken(
	h http.Header,
	key string,
	want string,
) bool {
	values := h.Values(key)

	for _, v := range values {
		for _, token := range splitHeaderTokens(v) {
			if strings.EqualFold(token, want) {
				return true
			}
		}
	}

	return false
}
