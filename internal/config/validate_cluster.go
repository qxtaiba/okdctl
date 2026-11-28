package config

import (
	"regexp"
	"strconv"
)

var okdVersion = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)-okd-scos\.(\d+)$`)

func (c *Config) validateCluster(r *ValidationResult) {
	r.check(FieldClusterName, CheckClusterName(c.Cluster.Name))
	r.check(FieldClusterDomain, CheckDomain(c.Cluster.Domain))
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
