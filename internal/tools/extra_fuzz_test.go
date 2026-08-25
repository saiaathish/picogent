package tools

import (
	"context"
	"net"
	"testing"
)

func FuzzWebFetchIPBoundary(f *testing.F) {
	for _, seed := range []string{"example.test", "localhost", "127.0.0.1", "", "[::1]"} {
		f.Add(seed, false)
	}
	f.Fuzz(func(t *testing.T, host string, private bool) {
		// Keep the fuzzed value in the hostname path so the injected resolver is
		// what supplies the answer under test, even when the input resembles an
		// IP literal.
		host += ".fuzz.test"
		ips := []net.IP{net.ParseIP("8.8.8.8")}
		if private {
			ips = []net.IP{net.ParseIP("127.0.0.1")}
		}
		resolved, err := webFetchIPs(context.Background(), host, staticWebFetchResolver{ips: ips})
		if private && err == nil {
			t.Fatalf("private answer accepted for host %q: %v", host, resolved)
		}
		if err == nil {
			for _, ip := range resolved {
				if blockedWebFetchIP(ip) {
					t.Fatalf("blocked address returned without error: %v", ip)
				}
			}
		}
	})
}
