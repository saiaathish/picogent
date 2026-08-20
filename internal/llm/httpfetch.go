package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

func newGET(url string, timeout time.Duration) (*http.Request, error) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		_ = cancel
	}
	return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
}

func doGET(req *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d", res.StatusCode)
	}
	return b, nil
}
