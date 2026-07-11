package hostnet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/testutil"
)

func installFakeIPNmcli(t *testing.T, ipOutput string, ipExit int, nmcliLog string) {
	t.Helper()
	ipScript := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' '%s'\nexit %d\n", ipOutput, ipExit)

	nmcliScript := fmt.Sprintf(`#!/bin/sh
printf '%%s %%s\n' "$1" "$2" >> '%s'
case "$1" in
  -t)
    printf 'myconn:eth0\n'
    exit 0
    ;;
  connection)
    case "$2" in
      show)
        printf 'myconn:eth0\n'
        exit 0
        ;;
    esac
    exit 0
    ;;
  device)
    exit 0
    ;;
esac
exit 0
`, nmcliLog)

	testutil.InstallFakeBin(t, "ip", ipScript)
	testutil.InstallFakeBin(t, "nmcli", nmcliScript)
}

func nmcliCallCount(t *testing.T, logFile string) int {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("reading nmcli log: %v", err)
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func TestValidateConnectionName(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"plain name", "eth0", false},
		{"name with colon", "br0:1", false},
		{"empty rejected", "", true},
		{"leading dash rejected", "-ipv4.method", true},
		{"semicolon rejected", "conn;rm -rf /", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConnectionName(tc.in)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateConnectionName(%q) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
			}
		})
	}
}

func TestRemoveSecondaryIP(t *testing.T) {
	t.Run("ip absent fast-paths nil with no nmcli call", func(t *testing.T) {
		logFile := filepath.Join(t.TempDir(), "nmcli.log")
		installFakeIPNmcli(t, "inet 10.0.0.1/24", 0, logFile)

		err := RemoveSecondaryIP(context.Background(), "192.168.1.10", "eth0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n := nmcliCallCount(t, logFile); n != 0 {
			t.Errorf("nmcli called %d times; want 0 (short-circuit on absent IP)", n)
		}
	})

	t.Run("ip present invokes connection modify and device reapply", func(t *testing.T) {
		logFile := filepath.Join(t.TempDir(), "nmcli.log")
		installFakeIPNmcli(t, "inet 192.168.1.10/24 brd 192.168.1.255", 0, logFile)

		err := RemoveSecondaryIP(context.Background(), "192.168.1.10", "eth0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n := nmcliCallCount(t, logFile); n != 3 {
			t.Errorf("nmcli called %d times; want 3 (show + modify + reapply)", n)
		}
	})

	t.Run("ip exits 1 returns wrapped presence-check error", func(t *testing.T) {
		logFile := filepath.Join(t.TempDir(), "nmcli.log")
		installFakeIPNmcli(t, "", 1, logFile)

		err := RemoveSecondaryIP(context.Background(), "192.168.1.10", "eth0")
		if err == nil {
			t.Fatal("expected error from ip exit 1; got nil")
		}
		if !strings.Contains(err.Error(), "check IP presence") {
			t.Errorf("err = %q; want 'check IP presence' substring", err.Error())
		}
	})

	t.Run("empty ip returns validation error", func(t *testing.T) {
		err := RemoveSecondaryIP(context.Background(), "", "eth0")
		if err == nil {
			t.Fatal("expected error for empty ip")
		}
	})

	t.Run("empty iface returns validation error", func(t *testing.T) {
		err := RemoveSecondaryIP(context.Background(), "192.168.1.10", "")
		if err == nil {
			t.Fatal("expected error for empty iface")
		}
	})
}
