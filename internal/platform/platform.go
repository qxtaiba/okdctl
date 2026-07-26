// Package platform detects the host Linux family (RHEL vs Debian) from
// /etc/os-release and exposes family-specific knobs — Apache paths,
// service naming, CoreOS/download architecture strings — plus the
// PackageManager abstraction (single Manager type driven by per-family
// binary names: dnf/rpm on RHEL, apt-get/dpkg on Debian) so provisioning
// code can stay distribution-agnostic.
package platform

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Family identifies the host OS lineage (RHEL or Debian).
type Family string

const archARM64 = "arm64"

// Platform OS-family identifiers.
const (
	FamilyRHEL   Family = "rhel"
	FamilyDebian Family = "debian"
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
	Family   Family
	ID       string // "fedora", "ubuntu", "rocky", "almalinux", "rhel", "debian"
	Version  string // "39", "24.04"
	Codename string // VERSION_CODENAME, e.g. "noble", "bookworm"; "" on RHEL family
}

var rhelIDs = map[string]bool{
	"fedora": true, "rhel": true, "rocky": true, "almalinux": true, "alma": true, "centos": true,
}

var debianIDs = map[string]bool{
	"debian": true, "ubuntu": true,
}

// DetectOrDefault returns the detected OS, falling back to
// OS{Family: FamilyRHEL, ID: "unknown"} and warning via logger when
// detection fails. A nil logger is treated as no-op.
func DetectOrDefault(logger *slog.Logger) OS {
	detected, err := Detect()
	if err != nil {
		logutil.OrNop(logger).Warn("platform: detect failed; defaulting to rhel", "err", err)
		return OS{Family: FamilyRHEL, ID: "unknown"}
	}
	return detected
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
	for line := range strings.Lines(content) {
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
		Family:   family,
		ID:       id,
		Version:  fields["VERSION_ID"],
		Codename: fields["VERSION_CODENAME"],
	}, nil
}

func detectFamily(id, idLike string) Family {
	if rhelIDs[id] {
		return FamilyRHEL
	}
	if debianIDs[id] {
		return FamilyDebian
	}
	for like := range strings.FieldsSeq(idLike) {
		if rhelIDs[like] {
			return FamilyRHEL
		}
		if debianIDs[like] {
			return FamilyDebian
		}
	}
	return ""
}

// ApachePackageName returns the package name for Apache HTTP server.
func (o OS) ApachePackageName() string {
	if o.Family == FamilyDebian {
		return "apache2"
	}
	return "httpd"
}

// ApacheServiceName returns the systemd service name for Apache.
func (o OS) ApacheServiceName() string {
	if o.Family == FamilyDebian {
		return "apache2"
	}
	return "httpd"
}

// ApacheUser returns the user that Apache runs as.
func (o OS) ApacheUser() string {
	if o.Family == FamilyDebian {
		return "www-data"
	}
	return "apache"
}

// ApacheVhostConfDir returns the directory where drop-in vhost conf files
// land. On RHEL conf.d is auto-included by httpd.conf; on Debian the conf
// is activated via a2enconf.
func (o OS) ApacheVhostConfDir() string {
	if o.Family == FamilyDebian {
		return "/etc/apache2/conf-available"
	}
	return "/etc/httpd/conf.d"
}
