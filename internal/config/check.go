package config

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Limits the field validators and the wizard share.
const (
	MaxVLAN     = 4094
	MinVMIDBase = 100

	maxClusterNameLen = 54
	maxDNSLabelLen    = 63
	maxDNSNameLen     = 253
	minCPUModelLevel  = 3
	cpuTypeHost       = "host"
	cpuModelPrefix    = "x86-64-v"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

var errRequired = errors.New("required")

// Floor is the least sizing the agent ISO accepts for a role.
type Floor struct {
	CPU       int
	MemoryMiB int
	RootGiB   int
}

// FloorFor returns the sizing floor of role.
func FloorFor(role Role) Floor {
	if role == RoleMaster {
		return Floor{CPU: 4, MemoryMiB: 16384, RootGiB: 100}
	}
	return Floor{CPU: 2, MemoryMiB: 8192, RootGiB: 100}
}

// CheckClusterName reports why name cannot be a cluster name: a DNS label
// of at most 54 characters, so every produced node name fits a label.
func CheckClusterName(name string) error {
	switch {
	case name == "":
		return errRequired
	case len(name) > maxClusterNameLen:
		return fmt.Errorf("must be at most %d characters, got %d", maxClusterNameLen, len(name))
	case !dnsLabel.MatchString(name):
		return errors.New("must be lowercase letters, digits and hyphens, no dots")
	}
	return nil
}

// CheckDomain reports why domain is not a DNS name.
func CheckDomain(domain string) error {
	if domain == "" {
		return errRequired
	}
	if len(domain) > maxDNSNameLen {
		return fmt.Errorf("%q is not a valid DNS name", domain)
	}
	for _, label := range strings.Split(domain, ".") {
		if !dnsLabel.MatchString(label) {
			return fmt.Errorf("%q is not a valid DNS name", domain)
		}
	}
	return nil
}

// CheckEndpoint reports why endpoint is not an http(s) URL with a host.
func CheckEndpoint(endpoint string) error {
	if endpoint == "" {
		return errRequired
	}
	u, err := url.Parse(endpoint)
	switch {
	case err != nil || u.Scheme == "" || u.Host == "":
		return fmt.Errorf("%q must be a URL with scheme and host, such as https://pve:8006", endpoint)
	case u.Scheme != "http" && u.Scheme != "https":
		return fmt.Errorf("%q must use http or https", endpoint)
	}
	return nil
}

// CheckCPUType accepts host or an x86-64-v<N> model at or above v3, the
// level the agent ISO's RHCOS requires.
func CheckCPUType(cpuType string) error {
	if cpuType == cpuTypeHost {
		return nil
	}
	level, ok := strings.CutPrefix(cpuType, cpuModelPrefix)
	n, err := strconv.Atoi(level)
	if !ok || err != nil || n < minCPUModelLevel {
		return fmt.Errorf("%q must be %s or %s%d or newer", cpuType, cpuTypeHost, cpuModelPrefix, minCPUModelLevel)
	}
	return nil
}
