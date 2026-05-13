package proxy

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/krishnaditya65/web-server/internal/types"
)

func (e *Engine) handleWebSocket(
	w http.ResponseWriter,
	r *http.Request,
	target *types.Upstream,
) error {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(
			w,
			"hijacking not supported",
			http.StatusInternalServerError,
		)
		return http.ErrNotSupported
	}

	clientConn, clientBuf, err := hijacker.Hijack()
	if err != nil {
		return err
	}

	upstreamConn, err := dialUpstream(target)
	if err != nil {
		clientConn.Close()
		return err
	}

	if err := writeUpstreamHandshake(
		upstreamConn,
		r,
		target,
	); err != nil {
		clientConn.Close()
		upstreamConn.Close()
		return err
	}

	resp, err := http.ReadResponse(
		bufio.NewReader(upstreamConn),
		r,
	)
	if err != nil {
		clientConn.Close()
		upstreamConn.Close()
		return err
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Write(clientConn)
		clientConn.Close()
		upstreamConn.Close()

		return errors.New("upstream rejected websocket upgrade")
	}

	if err := resp.Write(clientConn); err != nil {
		clientConn.Close()
		upstreamConn.Close()
		return err
	}

	return relayBidirectional(
		r.Context(),
		clientConn,
		clientBuf,
		upstreamConn,
	)
}

func dialUpstream(target *types.Upstream) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	switch target.URL.Scheme {
	case "https", "wss":
		return tlsDial(target, dialer)

	default:
		return dialer.Dial("tcp", target.URL.Host)
	}
}

func writeUpstreamHandshake(
	conn net.Conn,
	r *http.Request,
	target *types.Upstream,
) error {
	out := cloneRequest(r.Context(), r)

	rewriteRequest(out, target)

	removeHopByHopHeaders(out.Header)

	out.Header.Set("Connection", "Upgrade")
	out.Header.Set("Upgrade", "websocket")

	appendForwardHeaders(out)

	return out.Write(conn)
}

func relayBidirectional(
	ctx context.Context,
	clientConn net.Conn,
	clientBuf *bufio.ReadWriter,
	upstreamConn net.Conn,
) error {
	var once sync.Once

	closeAll := func() {
		once.Do(func() {
			clientConn.Close()
			upstreamConn.Close()
		})
	}

	errCh := make(chan error, 3)

	if clientBuf != nil && clientBuf.Reader.Buffered() > 0 {
		go func() {
			_, err := io.Copy(
				upstreamConn,
				clientBuf,
			)
			errCh <- err
		}()
	} else {
		go func() {
			_, err := io.Copy(
				upstreamConn,
				clientConn,
			)
			errCh <- err
		}()
	}

	go func() {
		_, err := io.Copy(
			clientConn,
			upstreamConn,
		)
		errCh <- err
	}()

	go func() {
		<-ctx.Done()
		errCh <- ctx.Err()
	}()

	err := <-errCh

	closeAll()

	return err
}
