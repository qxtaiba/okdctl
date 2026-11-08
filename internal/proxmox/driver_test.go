package proxmox

import (
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		token    string
		wantErr  string
	}{
		{
			name: "empty endpoint", endpoint: "", token: "user@pve!tok=secret",
			wantErr: "endpoint is required",
		},
		{
			name: "malformed token", endpoint: "pve.example", token: "user@pve!tok",
			wantErr: "id=secret",
		},
		{name: "missing token", endpoint: "pve.example", token: "", wantErr: "no credentials"},
		{name: "well-formed token", endpoint: "https://pve.example:8006/", token: "user@pve!tok=secret"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := New(tc.endpoint, []byte(tc.token), false)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("New() = %v, want error containing %q", d, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("New() error %q, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("New(): %v", err)
			}
			if d == nil {
				t.Fatal("New() returned nil Driver with nil error")
			}
		})
	}
}
