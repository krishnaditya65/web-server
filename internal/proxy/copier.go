package proxy

import (
	"io"
	"net/http"
)

func writeResponse(w http.ResponseWriter, resp *http.Response) error {
	defer resp.Body.Close()

	removeHopByHopHeaders(resp.Header)

	copyHeaders(w.Header(), resp.Header)

	if len(resp.Trailer) > 0 {
		declareTrailers(w.Header(), resp.Trailer)
	}

	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		return err
	}

	copyTrailers(w.Header(), resp.Trailer)

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	return nil
}

func copyHeaders(dst, src http.Header) {
	for k, values := range src {
		for _, v := range values {
			dst.Add(k, v)
		}
	}
}

func declareTrailers(dst http.Header, trailers http.Header) {
	for key := range trailers {
		dst.Add("Trailer", key)
	}
}

func copyTrailers(dst, trailers http.Header) {
	for key, values := range trailers {
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}
