package httputil

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestClientFactories(t *testing.T) {
	cases := []struct {
		name         string
		client       *http.Client
		wantTimeout  time.Duration
		nilTransport bool
		wantInsecure bool
	}{
		{
			name:         "New leaves Transport nil for verified TLS",
			client:       New(5 * time.Second),
			wantTimeout:  5 * time.Second,
			nilTransport: true,
		},
		{
			name:         "NewOptionalInsecure true skips verification",
			client:       NewOptionalInsecure(true, 3*time.Second),
			wantTimeout:  3 * time.Second,
			wantInsecure: true,
		},
		{
			name:        "NewOptionalInsecure false keeps verification on",
			client:      NewOptionalInsecure(false, time.Second),
			wantTimeout: time.Second,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.client.Timeout != tc.wantTimeout {
				t.Errorf("Timeout = %v, want %v", tc.client.Timeout, tc.wantTimeout)
			}
			if tc.client.CheckRedirect == nil {
				t.Error("CheckRedirect not installed; redirect cap policy is missing")
			}
			if tc.nilTransport {
				if tc.client.Transport != nil {
					t.Errorf("Transport = %T, want nil for default (verified TLS)", tc.client.Transport)
				}
				return
			}
			tr, ok := tc.client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("Transport not *http.Transport: %T", tc.client.Transport)
			}
			if tr.TLSClientConfig == nil {
				t.Fatal("TLSClientConfig missing — verification mode would be ignored")
			}
			if tr.TLSClientConfig.InsecureSkipVerify != tc.wantInsecure {
				t.Errorf("InsecureSkipVerify = %v, want %v", tr.TLSClientConfig.InsecureSkipVerify, tc.wantInsecure)
			}
		})
	}
}

func TestCapRedirects(t *testing.T) {
	mkReq := func(host, auth string) *http.Request {
		return &http.Request{
			URL:    &url.URL{Scheme: "https", Host: host, Path: "/x"},
			Header: http.Header{"Authorization": []string{auth}},
		}
	}
	mkVia := func(n int, host string) []*http.Request {
		via := make([]*http.Request, n)
		for i := range via {
			via[i] = &http.Request{URL: &url.URL{Scheme: "https", Host: host, Path: "/x"}}
		}
		return via
	}

	t.Run("same host below cap is allowed", func(t *testing.T) {
		req := mkReq("example.com", "Bearer x")
		if err := capRedirects(req, mkVia(1, "example.com")); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("cross host without auth is allowed", func(t *testing.T) {
		req := mkReq("other.com", "")
		if err := capRedirects(req, mkVia(1, "example.com")); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("cross host with auth is refused", func(t *testing.T) {
		req := mkReq("other.com", "Bearer x")
		err := capRedirects(req, mkVia(1, "example.com"))
		if err == nil {
			t.Fatal("expected refusal, got nil")
		}
	})

	t.Run("five redirects hits cap", func(t *testing.T) {
		req := mkReq("example.com", "")
		err := capRedirects(req, mkVia(5, "example.com"))
		if err == nil {
			t.Fatal("expected cap error, got nil")
		}
	})

	t.Run("four redirects below cap is allowed", func(t *testing.T) {
		req := mkReq("example.com", "")
		if err := capRedirects(req, mkVia(4, "example.com")); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})
}

func TestRedirectRefusesHTTPSDowngrade(t *testing.T) {
	for _, auth := range []string{"", "Bearer synthetic"} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/next", http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", auth)
		previous, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com/", http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		if err := capRedirects(req, []*http.Request{previous}); !errors.Is(err, errInsecureRedirect) {
			t.Fatalf("error = %v", err)
		}
	}
}
