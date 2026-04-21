package phase

import "testing"

func TestProxmoxBareHost(t *testing.T) {
	cases := map[string]string{
		"pve.example":             "pve.example",
		"pve.example:8006":        "pve.example",
		"https://pve.example":     "pve.example",
		"https://pve.example:443": "pve.example",
		"http://pve.example:8006": "pve.example",
		"[2001:db8::1]:8006":      "2001:db8::1",
		"10.0.0.1":                "10.0.0.1",
		"10.0.0.1:22":             "10.0.0.1",
		"":                        "",
	}
	for in, want := range cases {
		if got := ProxmoxBareHost(in); got != want {
			t.Errorf("ProxmoxBareHost(%q) = %q, want %q", in, got, want)
		}
	}
}
