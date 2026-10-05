// Package platform identifies the bastion host from /etc/os-release — only
// RHEL-family Linux is supported — and holds its fixed Apache names, CoreOS
// arch keys, and package manager.
package platform

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

const archARM64 = "arm64"

// Apache names on a RHEL-family bastion; conf.d drop-ins are auto-included.
const (
	ApachePackage      = "httpd"
	ApacheService      = "httpd"
	ApacheUser         = "apache"
	ApacheVhostConfDir = "/etc/httpd/conf.d"
)

// ErrUnsupportedOS marks an os-release that names a non-RHEL-family distribution.
var ErrUnsupportedOS = errors.New("unsupported host os")

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
	ID      string // "fedora", "rocky", "almalinux", "rhel", "centos"
	Version string // "41", "9.4"
}

var rhelIDs = map[string]bool{
	"fedora": true, "rhel": true, "rocky": true, "almalinux": true, "alma": true, "centos": true,
}

// Detect reads /etc/os-release and returns the detected OS. A host outside the
// RHEL family fails with an error wrapping ErrUnsupportedOS.
func Detect() (OS, error) {
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return OS{}, fmt.Errorf("cannot read /etc/os-release: %w", err)
	}
	return parseOSRelease(string(content))
}

// RequireSupported fails with ErrUnsupportedOS only on a host positively
// identified as non-RHEL-family; an unreadable os-release passes, so
// non-Linux development hosts are not refused here.
func RequireSupported() error {
	if _, err := Detect(); errors.Is(err, ErrUnsupportedOS) {
		return err
	}
	return nil
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

	if !isRHELFamily(id, fields["ID_LIKE"]) {
		return OS{}, fmt.Errorf("%w %q: okdctl deploys from rhel-family bastions only (fedora, rhel, centos, rocky, almalinux)", ErrUnsupportedOS, id)
	}

	return OS{ID: id, Version: fields["VERSION_ID"]}, nil
}

func isRHELFamily(id, idLike string) bool {
	if rhelIDs[id] {
		return true
	}
	for like := range strings.FieldsSeq(idLike) {
		if rhelIDs[like] {
			return true
		}
	}
	return false
}
