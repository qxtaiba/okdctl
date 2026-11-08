package config

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	maxClusterNameLen = 54
	maxDNSNameLen     = 253
)

var (
	dnsLabel   = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	okdVersion = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)-okd-scos\.(\d+)$`)
)

func (c *Config) validateCluster(r *ValidationResult) {
	switch name := c.Cluster.Name; {
	case name == "":
		r.AddError(FieldClusterName, "required")
	case len(name) > maxClusterNameLen:
		r.addf(FieldClusterName, "must be at most %d characters, got %d", maxClusterNameLen, len(name))
	case !dnsLabel.MatchString(name):
		r.AddError(FieldClusterName, "must be lowercase letters, digits and hyphens, no dots")
	}
	switch domain := c.Cluster.Domain; {
	case domain == "":
		r.AddError(FieldClusterDomain, "required")
	case !isDNSName(domain):
		r.addf(FieldClusterDomain, "%q is not a valid DNS name", domain)
	}
	if c.Distribution.Type != DistributionOKD {
		r.addf(FieldDistributionType, "must be %s", DistributionOKD)
	}
	validateVersion(r, c.Distribution.Version)
	if c.Files.PullSecret == "" {
		r.AddError(FieldFilesPullSecret, "required")
	}
	if c.Files.SSHPublicKey == "" {
		r.AddError(FieldFilesSSHPublicKey, "required")
	}
}

func isDNSName(name string) bool {
	if len(name) > maxDNSNameLen {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if !dnsLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func validateVersion(r *ValidationResult, version string) {
	parsed, ok := parseOKDVersion(version)
	if !ok {
		r.addf(FieldDistributionVersion, "%q must look like %s", version, DefaultOKDVersion)
		return
	}
	floor, _ := parseOKDVersion(MinOKDVersion)
	for i := range parsed {
		if parsed[i] == floor[i] {
			continue
		}
		if parsed[i] < floor[i] {
			r.addf(FieldDistributionVersion, "%s is below the floor %s", version, MinOKDVersion)
		}
		return
	}
}

func parseOKDVersion(version string) (parts [4]int, ok bool) {
	m := okdVersion.FindStringSubmatch(version)
	if m == nil {
		return parts, false
	}
	for i := range parts {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}
