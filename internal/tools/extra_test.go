package tools

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestWebFetchRejectsPrivateAndLocalTargetsAfterApproval(t *testing.T) {
	for _, raw := range []string{
		"http://localhost/metadata",
		"http://127.0.0.1:8080/",
		"http://169.254.169.254/latest/meta-data",
		"http://[::1]/",
		"http://100.100.100.100/",
	} {
		_, err := (webFetch{}).Run(context.Background(), `{"url":"`+raw+`"}`, Context{})
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "private or local") {
			t.Fatalf("web fetch %q error = %v, want private/local rejection", raw, err)
		}
	}
}

func TestWebFetchURLParserRejectsCredentials(t *testing.T) {
	if _, err := parseWebFetchURL("https://user:pass@example.com/"); err == nil {
		t.Fatal("URL credentials were accepted")
	}
}

type staticWebFetchResolver struct {
	ips []net.IP
	err error
}

func (r staticWebFetchResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return r.ips, r.err
}

func TestWebFetchRejectsMixedPublicAndPrivateDNSAnswers(t *testing.T) {
	resolver := staticWebFetchResolver{ips: []net.IP{
		net.ParseIP("8.8.8.8"),
		net.ParseIP("127.0.0.1"),
	}}
	if _, err := webFetchIPs(context.Background(), "rebind.test", resolver); err == nil {
		t.Fatal("mixed public/private DNS answers were accepted")
	}
}

func TestWebFetchDialPinsToValidatedAddress(t *testing.T) {
	resolver := staticWebFetchResolver{ips: []net.IP{net.ParseIP("8.8.8.8")}}
	var gotAddress string
	_, err := dialWebFetchAddress(context.Background(), "tcp", "rebind.test:443", resolver,
		func(_ context.Context, _, address string) (net.Conn, error) {
			gotAddress = address
			return nil, errors.New("stop after observing dial target")
		})
	if err == nil {
		t.Fatal("expected injected dial failure")
	}
	if gotAddress != "8.8.8.8:443" {
		t.Fatalf("dial address = %q, want pinned IP (err=%v)", gotAddress, err)
	}
}
