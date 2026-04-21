package httputil

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	c := New(5 * time.Second)
	if c.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", c.Timeout)
	}
	if c.Transport != nil {
		t.Errorf("New() should leave Transport nil for default (verified TLS)")
	}
}

func TestNewInsecure(t *testing.T) {
	c := NewInsecure(3 * time.Second)
	if c.Timeout != 3*time.Second {
		t.Errorf("Timeout = %v, want 3s", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport not *http.Transport: %T", c.Transport)
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig missing — insecure flag would be ignored")
	}
	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("InsecureSkipVerify = false; NewInsecure's whole purpose is to set this true")
	}

	// Defense-in-depth: New (secure variant) must not accidentally also
	// produce InsecureSkipVerify=true via shared-transport goofs.
	secure := New(3 * time.Second)
	if secure.Transport != nil {
		if st, ok := secure.Transport.(*http.Transport); ok && st.TLSClientConfig != nil {
			if st.TLSClientConfig.InsecureSkipVerify {
				t.Errorf("secure New() has InsecureSkipVerify=true — cross-contamination")
			}
		}
	}

	// Minor: ensure the tls config type is what we expect (guards against a
	// future refactor that swaps to a different package).
	var _ *tls.Config = tr.TLSClientConfig
}
