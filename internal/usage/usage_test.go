package usage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONErrorsAreCredentialFreeAndRedirectsAreBlocked(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 302} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "https://invalid.test/private-token")
			w.WriteHeader(status)
			_, _ = w.Write([]byte("private-token response"))
		}))
		var out any
		err := JSON(context.Background(), NewHTTPClient(), http.MethodGet, srv.URL, "private-token", nil, nil, &out)
		srv.Close()
		if err == nil || strings.Contains(err.Error(), "private-token") {
			t.Fatalf("unsafe error for %d: %v", status, err)
		}
		if status == 401 && !errors.Is(err, ErrLogin) {
			t.Fatalf("bad login classification: %v", err)
		}
	}
}

func TestCacheFollowsCredentialIdentityAndPreservesUnavailable(t *testing.T) {
	var cache Cache
	calls := 0
	fetch := func(context.Context) (Snapshot, error) { calls++; return Snapshot{}, ErrUnavailable }
	for i := 0; i < 2; i++ {
		_, err := cache.Read(context.Background(), "first-account", fetch)
		if !errors.Is(err, ErrUnavailable) {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("same login requested %d times", calls)
	}
	_, _ = cache.Read(context.Background(), "second-account", fetch)
	if calls != 2 {
		t.Fatal("new login reused old usage")
	}
}
