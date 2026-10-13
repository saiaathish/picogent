package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckReleaseComparison(t *testing.T) {
	for _, tc := range []struct {
		current string
		want    bool
	}{
		{"1.1.0-dev+e935692", false},
		{"1.0.0", false},
		{"0.9.9", true},
		{"1.0.0-dev", true},
		{"unknown", false},
	} {
		t.Run(tc.current, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/releases/latest" {
					t.Errorf("path = %q", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"tag_name":"v1.0.0","html_url":"https://github.com/saiaathish/picogent/releases/tag/v1.0.0"}`))
			}))
			defer ts.Close()
			oldURL, oldClient := APIURL, HTTPClient
			APIURL, HTTPClient = ts.URL+"/releases/latest", ts.Client()
			defer func() { APIURL, HTTPClient = oldURL, oldClient }()
			got, err := Check(context.Background(), tc.current)
			if err != nil {
				t.Fatal(err)
			}
			if got.Available != tc.want || got.Tag != "v1.0.0" {
				t.Fatalf("got = %+v, want available %v", got, tc.want)
			}
		})
	}
}

func TestCheckRejectsUnpublishedAndInvalidTags(t *testing.T) {
	for _, payload := range []string{
		`{"tag_name":"v1.2.0","draft":true}`,
		`{"tag_name":"v1.2.0","prerelease":true}`,
		`{"tag_name":"v1.2.0;evil"}`,
		`{"tag_name":"v1.2.0-rc1"}`,
	} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(payload)) }))
		oldURL, oldClient := APIURL, HTTPClient
		APIURL, HTTPClient = ts.URL, ts.Client()
		_, err := Check(context.Background(), "1.0.0")
		APIURL, HTTPClient = oldURL, oldClient
		ts.Close()
		if err == nil {
			t.Fatalf("accepted %s", payload)
		}
	}
}
