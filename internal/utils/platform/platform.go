// Package platform detects the host Linux family (RHEL vs Debian) from
// /etc/os-release and exposes family-specific knobs — package manager,
// Apache paths, SELinux presence, CoreOS/download architecture strings —
// so provisioning code can stay distribution-agnostic.
package platform

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

const (
	archARM64    = "arm64"
	familyRHEL   = "rhel"
	familyDebian = "debian"
)

// DownloadArch returns the architecture suffix for tool download URLs.
func DownloadArch() string {
	if runtime.GOARCH == archARM64 {
		return archARM64
	}
	return "amd64"
}

// CoreOSArch returns the CoreOS stream architecture key.
func CoreOSArch() string {
	if runtime.GOARCH == archARM64 {
		return "aarch64"
	}
	return "x86_64"
}

// OS describes the detected host operating system.
type OS struct {
	Family  string // "rhel", "debian"
	ID      string // "fedora", "ubuntu", "rocky", "almalinux", "rhel", "debian"
	Version string // "39", "24.04"
}

var rhelIDs = map[string]bool{
	"fedora": true, "rhel": true, "rocky": true, "almalinux": true, "alma": true, "centos": true,
}

var debianIDs = map[string]bool{
	"debian": true, "ubuntu": true,
}

// Detect reads /etc/os-release and returns the detected OS.
func Detect() (OS, error) {
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return OS{}, fmt.Errorf("cannot read /etc/os-release: %w", err)
	}
	return parseOSRelease(string(content))
}

func parseOSRelease(content string) (OS, error) {
	fields := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields[key] = strings.Trim(value, "\"")
	}

	id := strings.ToLower(fields["ID"])
	if id == "" {
		return OS{}, fmt.Errorf("ID not found in os-release")
	}

	family := detectFamily(id, fields["ID_LIKE"])
	if family == "" {
		return OS{}, fmt.Errorf("unsupported os: %s (requires fedora, rocky, alma, rhel, ubuntu, or debian)", id)
	}

	return OS{
		Family:  family,
		ID:      id,
		Version: fields["VERSION_ID"],
	}, nil
}

func detectFamily(id, idLike string) string {
	if rhelIDs[id] {
		return familyRHEL
	}
	if debianIDs[id] {
		return familyDebian
	}
	for _, like := range strings.Fields(idLike) {
		if rhelIDs[like] {
			return familyRHEL
		}
		if debianIDs[like] {
			return familyDebian
		}
	}
	return ""
}

// ApachePackageName returns the package name for Apache HTTP server.
func (o OS) ApachePackageName() string {
	if o.Family == familyDebian {
		return "apache2"
	}
	return "httpd"
}

// ApacheConfigPath returns the path to the main Apache config file.
func (o OS) ApacheConfigPath() string {
	if o.Family == familyDebian {
		return "/etc/apache2/apache2.conf"
	}
	return "/etc/httpd/conf/httpd.conf"
}

// ApacheServiceName returns the systemd service name for Apache.
func (o OS) ApacheServiceName() string {
	if o.Family == familyDebian {
		return "apache2"
	}
	return "httpd"
}

// ApacheUser returns the user that Apache runs as.
func (o OS) ApacheUser() string {
	if o.Family == familyDebian {
		return "www-data"
	}
	return "apache"
}

// HasSELinux returns true if the OS family uses SELinux.
func (o OS) HasSELinux() bool {
	return o.Family == familyRHEL
}
