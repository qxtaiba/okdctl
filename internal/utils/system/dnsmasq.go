package system

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

const (
	dnsmasqConfigDir = "/etc/dnsmasq.d"
	dnsmasqService   = "dnsmasq"
)

var validConfigNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func EnableDnsmasq(ctx context.Context) error {
	return ManageService(ctx, ServiceEnable, dnsmasqService, "dnsmasq")
}

func RestartDnsmasq(ctx context.Context) error {
	return ManageService(ctx, ServiceRestart, dnsmasqService, "dnsmasq")
}

func ValidateDnsmasqConfig(ctx context.Context) error {
	return RunSudo(ctx, "dnsmasq", "--test")
}

func ReloadDnsmasq(ctx context.Context) error {
	return ManageService(ctx, ServiceReload, dnsmasqService, "dnsmasq")
}

func validateConfigName(name string) error {
	if name == "" {
		return fmt.Errorf("config name cannot be empty")
	}
	if !validConfigNameRegex.MatchString(name) {
		return fmt.Errorf("config name must contain only alphanumeric characters, hyphens, and underscores, and start with alphanumeric")
	}
	return nil
}

func WriteDnsmasqConfig(ctx context.Context, name, content string) error {
	if err := validateConfigName(name); err != nil {
		return utils.WrapError("invalid config name", err)
	}

	configPath := filepath.Join(dnsmasqConfigDir, fmt.Sprintf("%s.conf", name))

	if err := MkdirAll(ctx, dnsmasqConfigDir, "dnsmasq config directory"); err != nil {
		return utils.WrapError("failed to create dnsmasq config directory", err)
	}

	tmpFile, err := os.CreateTemp("", "dnsmasq-*.conf")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	cleanup := func() { _ = os.Remove(tmpPath) }
	defer cleanup()

	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		return utils.WrapError("failed to write temp file", err)
	}
	if err := tmpFile.Close(); err != nil {
		return utils.WrapError("failed to close temp file", err)
	}

	if err := CopyFileWithElevation(ctx, tmpPath, configPath, "dnsmasq config"); err != nil {
		return utils.WrapErrorf(err, "failed to copy config to %s", configPath)
	}

	if err := Chmod(ctx, configPath, "644", "dnsmasq config permissions"); err != nil {
		return utils.WrapError("failed to set config permissions", err)
	}

	return nil
}

func DnsmasqConfigPath(name string) string {
	if err := validateConfigName(name); err != nil {
		return ""
	}
	return filepath.Join(dnsmasqConfigDir, fmt.Sprintf("%s.conf", name))
}

func IsNetworkManagerActive() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := exec.LookPath("nmcli"); err != nil {
		return false
	}
	return IsServiceActive("NetworkManager")
}

func getActiveConnection(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME", "connection", "show", "--active").Output()
	if err != nil {
		return "", utils.WrapError("failed to list network connections", err)
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != "lo" {
			return line, nil
		}
	}
	return "", fmt.Errorf("no active network connection found")
}

func validateDNSAddresses(addresses []string) error {
	for _, addr := range addresses {
		if net.ParseIP(addr) == nil {
			return fmt.Errorf("invalid DNS address: %s", addr)
		}
	}
	return nil
}

// ConfigureSystemResolver configures the system to use localhost (dnsmasq) for DNS resolution,
// with the given fallbackDNS servers for queries dnsmasq cannot resolve.
func ConfigureSystemResolver(ctx context.Context, fallbackDNS []string, logger utils.Logger) error {
	if !IsNetworkManagerActive() {
		logger.Warn("resolver: NetworkManager not active, skipping system resolver configuration")
		return nil
	}

	if err := validateDNSAddresses(fallbackDNS); err != nil {
		return utils.WrapError("invalid fallback DNS configuration", err)
	}

	conn, err := getActiveConnection(ctx)
	if err != nil {
		return err
	}

	dnsList := []string{"127.0.0.1"}
	dnsList = append(dnsList, fallbackDNS...)
	dnsConfig := strings.Join(dnsList, ",")

	logger.Info(fmt.Sprintf("resolver: configuring %s to use local dnsmasq", conn))

	if err := runSudo("nmcli", "connection", "modify", conn, "ipv4.dns", dnsConfig, "ipv4.ignore-auto-dns", "yes"); err != nil {
		return utils.WrapError("failed to configure DNS for connection", err)
	}

	if err := runSudo("nmcli", "connection", "up", conn); err != nil {
		return utils.WrapError("failed to apply DNS configuration", err)
	}

	logger.Info("resolver: system configured to use local dnsmasq")
	return nil
}

func RestoreSystemResolver(ctx context.Context, logger utils.Logger) error {
	if !IsNetworkManagerActive() {
		return nil
	}

	conn, err := getActiveConnection(ctx)
	if err != nil {
		return nil
	}

	logger.Info(fmt.Sprintf("resolver: restoring DHCP DNS for %s", conn))

	if err := runSudo("nmcli", "connection", "modify", conn, "ipv4.dns", "", "ipv4.ignore-auto-dns", "no"); err != nil {
		logger.Warn(fmt.Sprintf("resolver: failed to clear DNS settings: %v", err))
	}

	if err := runSudo("nmcli", "connection", "up", conn); err != nil {
		logger.Warn(fmt.Sprintf("resolver: failed to apply DNS configuration: %v", err))
	}

	logger.Info("resolver: system DNS restored to DHCP")
	return nil
}

