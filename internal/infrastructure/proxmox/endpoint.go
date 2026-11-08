// Package proxmox holds the Proxmox API endpoint helper the wizard's
// discovery builds its client URL from.
package proxmox

import "strings"

// APIBaseURL normalizes endpoint (https default, trailing slash trimmed) and
// appends /api2/json.
func APIBaseURL(endpoint string) string {
	return normalizeEndpoint(endpoint) + "/api2/json"
}

func normalizeEndpoint(endpoint string) string {
	e := strings.TrimRight(endpoint, "/")
	if strings.Contains(e, "://") {
		return e
	}
	return "https://" + e
}
