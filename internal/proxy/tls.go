package proxy

import (
	"crypto/tls"
	"net"

	"github.com/krishnaditya65/web-server/internal/types"
)

func tlsDial(
	target *types.Upstream,
	dialer *net.Dialer,
) (net.Conn, error) {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: target.URL.Hostname(),
	}

	return tls.DialWithDialer(
		dialer,
		"tcp",
		target.URL.Host,
		cfg,
	)
}
