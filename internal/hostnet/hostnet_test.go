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

// installReapplyFailNmcli wires a fake ip reporting the IP present plus an
// nmcli where `device reapply` always fails; restoreExit controls whether the
// compensating `+ipv4.addresses` modify succeeds (0) or fails too (non-zero).
func installReapplyFailNmcli(t *testing.T, nmcliLog string, restoreExit int) {
	t.Helper()
	ipScript := "#!/bin/sh\nprintf 'inet 192.168.1.10/24 brd 192.168.1.255\\n'\nexit 0\n"

	nmcliScript := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> '%s'
case "$1" in
  -t)
    printf 'myconn:eth0\n'
    exit 0
    ;;
  device)
    exit 1
    ;;
  connection)
    case "$4" in
      +ipv4.addresses)
        exit %d
        ;;
    esac
    exit 0
    ;;
esac
exit 0
`, nmcliLog, restoreExit)

	testutil.InstallFakeBin(t, "ip", ipScript)
	testutil.InstallFakeBin(t, "nmcli", nmcliScript)
}

func readLog(t *testing.T, logFile string) string {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("reading nmcli log: %v", err)
	}
	return string(data)
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
		{"name with space", "Wired connection 1", false},
		{"hyphen underscore", "my-conn_1", false},
		{"slash in name", "br0/dnsmasq", false},
		{"dot hyphen underscore", "a.b-c_d", false},
		{"max length 128", strings.Repeat("a", 128), false},
		{"empty rejected", "", true},
		{"too long 129 rejected", strings.Repeat("a", 129), true},
		{"leading dash rejected", "-ipv4.method", true},
		{"semicolon rejected", "conn;rm -rf /", true},
		{"newline rejected", "eth0\nid", true},
		{"carriage return rejected", "eth0\rid", true},
		{"null byte rejected", "eth0\x00id", true},
		{"backtick rejected", "eth`0", true},
		{"dollar sign rejected", "eth$0", true},
		{"less than rejected", "eth<0", true},
		{"greater than rejected", "eth>0", true},
		{"pipe rejected", "eth0|id", true},
		{"ampersand rejected", "eth0&id", true},
		{"shell injection rejected", "; rm -rf /", true},
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

func TestActiveConnection(t *testing.T) {
	t.Run("skips loopback and returns first name", func(t *testing.T) {
		testutil.InstallFakeBin(t, "nmcli", "#!/bin/sh\nprintf 'lo\\neth-conn\\n'\n")
		conn, err := ActiveConnection(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if conn != "eth-conn" {
			t.Errorf("conn = %q, want %q", conn, "eth-conn")
		}
	})

	t.Run("nmcli stderr propagates", func(t *testing.T) {
		testutil.InstallFakeBin(t, "nmcli", "#!/bin/sh\necho 'Error: NetworkManager is not running' >&2\nexit 10\n")
		_, err := ActiveConnection(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "NetworkManager is not running") {
			t.Errorf("error does not contain nmcli stderr: %q", err.Error())
		}
	})

	t.Run("unsafe scraped name rejected", func(t *testing.T) {
		testutil.InstallFakeBin(t, "nmcli", "#!/bin/sh\nprintf 'eth0;evil\\n'\n")
		_, err := ActiveConnection(context.Background())
		if err == nil {
			t.Fatal("expected error for unsafe connection name, got nil")
		}
		if !strings.Contains(err.Error(), "does not match allowed character set") {
			t.Errorf("error does not mention allowlist rejection: %q", err.Error())
		}
	})

	t.Run("no active connection is an error", func(t *testing.T) {
		testutil.InstallFakeBin(t, "nmcli", "#!/bin/sh\nprintf 'lo\\n'\n")
		if _, err := ActiveConnection(context.Background()); err == nil {
			t.Fatal("expected error when only loopback is active")
		}
	})
}

// TestConnectionOpsRejectBadNamesBeforeExec pins the CWE-88 guard on every
// exported op that places a connection name in nmcli argv position: a
// leading-dash name must be refused before nmcli runs at all.
func TestConnectionOpsRejectBadNamesBeforeExec(t *testing.T) {
	ops := map[string]func(context.Context, string) error{
		"OverrideConnectionDNS": func(ctx context.Context, conn string) error {
			return OverrideConnectionDNS(ctx, conn, []string{"127.0.0.1"})
		},
		"ClearConnectionDNSOverride": ClearConnectionDNSOverride,
		"ActivateConnection":         ActivateConnection,
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			logFile := filepath.Join(t.TempDir(), "nmcli.log")
			testutil.InstallFakeBin(t, "nmcli", "#!/bin/sh\necho \"$@\" >> "+logFile+"\nexit 0\n")
			if err := op(context.Background(), "-ipv4.method"); err == nil {
				t.Fatal("leading-dash connection name must be rejected")
			}
			if n := nmcliCallCount(t, logFile); n != 0 {
				t.Errorf("nmcli ran %d times on the refusal path; want 0", n)
			}
		})
	}
}

func TestOverrideAndClearConnectionDNSArgv(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "nmcli.log")
	testutil.InstallFakeBin(t, "nmcli", "#!/bin/sh\necho \"$@\" >> "+logFile+"\nexit 0\n")

	ctx := context.Background()
	if err := OverrideConnectionDNS(ctx, "eth-conn", []string{"127.0.0.1", "8.8.8.8"}); err != nil {
		t.Fatalf("OverrideConnectionDNS: %v", err)
	}
	if err := ActivateConnection(ctx, "eth-conn"); err != nil {
		t.Fatalf("ActivateConnection: %v", err)
	}
	if err := ClearConnectionDNSOverride(ctx, "eth-conn"); err != nil {
		t.Fatalf("ClearConnectionDNSOverride: %v", err)
	}

	got := strings.Split(strings.TrimRight(readLog(t, logFile), "\n"), "\n")
	want := []string{
		"connection modify eth-conn ipv4.dns 127.0.0.1,8.8.8.8 ipv4.ignore-auto-dns yes",
		"connection up eth-conn",
		"connection modify eth-conn ipv4.dns  ipv4.ignore-auto-dns no",
	}
	if len(got) != len(want) {
		t.Fatalf("nmcli calls = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("nmcli call %d = %q, want %q", i, got[i], want[i])
		}
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

	t.Run("reapply failure rolls the profile back", func(t *testing.T) {
		logFile := filepath.Join(t.TempDir(), "nmcli.log")
		installReapplyFailNmcli(t, logFile, 0)

		err := RemoveSecondaryIP(context.Background(), "192.168.1.10", "eth0")
		if err == nil {
			t.Fatal("expected error from failed device reapply; got nil")
		}
		if !strings.Contains(err.Error(), "rolled back") {
			t.Errorf("err = %q; want 'rolled back' substring", err.Error())
		}
		log := readLog(t, logFile)
		if !strings.Contains(log, "connection modify myconn +ipv4.addresses 192.168.1.10/32") {
			t.Errorf("no compensating +ipv4.addresses call recorded; log:\n%s", log)
		}
	})

	t.Run("reapply and rollback both failing names the divergence", func(t *testing.T) {
		logFile := filepath.Join(t.TempDir(), "nmcli.log")
		installReapplyFailNmcli(t, logFile, 1)

		err := RemoveSecondaryIP(context.Background(), "192.168.1.10", "eth0")
		if err == nil {
			t.Fatal("expected error; got nil")
		}
		if !strings.Contains(err.Error(), "no longer lists") {
			t.Errorf("err = %q; want profile/runtime divergence named", err.Error())
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
