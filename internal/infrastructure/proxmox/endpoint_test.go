package proxmox

import "testing"

func TestAPIBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://pve:8006/":    "https://pve:8006/api2/json",
		"pve.example.com":      "https://pve.example.com/api2/json",
		"pve.example.com/":     "https://pve.example.com/api2/json",
		"http://10.0.0.1:8006": "http://10.0.0.1:8006/api2/json",
	}
	for in, want := range cases {
		if got := APIBaseURL(in); got != want {
			t.Errorf("APIBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}
