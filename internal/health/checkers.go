package health

import (
	"log"
	"net/http"
	"time"

	"github.com/krishnaditya65/web-server/internal/types"
)

func StartHealthChecks(
	upstreams []*types.Upstream,
	path string,
	interval time.Duration,
	timeout time.Duration,
) {
	client := &http.Client{
		Timeout: timeout,
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			for _, upstream := range upstreams {
				check(client, upstream, path)
			}

			<-ticker.C
		}
	}()
}

func check(client *http.Client, upstream *types.Upstream, path string) {
	target := upstream.URL.String() + path

	resp, err := client.Get(target)
	if err != nil {
		upstream.Healthy.Store(false)
		log.Printf("health check failed: %s", target)
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		upstream.Healthy.Store(true)
		return
	}

	upstream.Healthy.Store(false)
	log.Printf("health check unhealthy: %s status=%d", target, resp.StatusCode)
}
