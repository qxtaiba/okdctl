package credentials

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestLoad(t *testing.T) {
	const fileToken, envToken = "root@pam!file=aaaa", "root@pam!env=bbbb"
	cases := []struct {
		name         string
		file         string
		envToken     string
		envInsecure  string
		wantToken    string
		wantInsecure bool
		wantWarn     bool
	}{
		{"file wins when the environment is unset", "PROXMOX_VE_API_TOKEN=" + fileToken + "\nPROXMOX_VE_INSECURE=true\n", "", "", fileToken, true, false},
		{"environment wins when set", "PROXMOX_VE_API_TOKEN=" + fileToken + "\nPROXMOX_VE_INSECURE=true\n", envToken, "false", envToken, false, true},
		{"identical tokens do not warn", "PROXMOX_VE_API_TOKEN=" + fileToken + "\n", fileToken, "", fileToken, false, false},
		{"empty export does not shadow the file", "PROXMOX_VE_API_TOKEN=" + fileToken + "\n", "", "", fileToken, false, false},
		{"missing file reads the environment only", "", envToken, "true", envToken, true, false},
		{"missing file and no export is invalid", "", "", "", "", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envProxmoxAPIToken, tc.envToken)
			t.Setenv(envProxmoxInsecure, tc.envInsecure)
			var logged bytes.Buffer
			t.Cleanup(logutil.Redirect(&logged))
			path := filepath.Join(t.TempDir(), "okdctl.env")
			if tc.file != "" {
				mustWriteEnv(t, path, tc.file, 0o600)
			}

			creds, err := Load(path, "https://pve:8006")
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := string(creds.APIToken); got != tc.wantToken {
				t.Errorf("APIToken = %q, want %q", got, tc.wantToken)
			}
			if creds.IsValid() != (tc.wantToken != "") || creds.Insecure != tc.wantInsecure || creds.Endpoint != "https://pve:8006" {
				t.Errorf("creds = %+v, want insecure=%v endpoint kept", creds.Redacted(), tc.wantInsecure)
			}
			if warned := strings.Contains(logged.String(), envProxmoxAPIToken); warned != tc.wantWarn {
				t.Errorf("shadow warning logged = %v, want %v:\n%s", warned, tc.wantWarn, logged.String())
			}
			for _, secret := range []string{fileToken, envToken} {
				if strings.Contains(logged.String(), secret) {
					t.Errorf("log leaks a token:\n%s", logged.String())
				}
			}
		})
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv(envProxmoxAPIToken, "root@pam!okdctl=secret")
	t.Setenv(envProxmoxInsecure, "true")
	creds := fromEnv("https://pve:8006")
	if string(creds.APIToken) != "root@pam!okdctl=secret" || !creds.Insecure || creds.Endpoint != "https://pve:8006" {
		t.Fatalf("creds = %+v, want the environment's token and flag", creds.Redacted())
	}

	t.Setenv(envProxmoxAPIToken, "")
	t.Setenv(envProxmoxInsecure, "false")
	if creds := fromEnv(""); creds.IsValid() || creds.Insecure {
		t.Fatalf("creds = %+v, want no token and tls verification on", creds.Redacted())
	}
}

func TestProxmoxCredentialsMaskAndZeroize(t *testing.T) {
	creds := ProxmoxCredentials{Endpoint: "https://pve:8006", APIToken: []byte("tok-secret"), Insecure: true}

	for name, rendered := range map[string]string{
		"value %+v":   fmt.Sprintf("%+v", creds),
		"value %#v":   fmt.Sprintf("%#v", creds),
		"String":      creds.String(),
		"pointer %+v": fmt.Sprintf("%+v", &creds),
		"pointer %#v": fmt.Sprintf("%#v", &creds),
		"Redacted":    fmt.Sprint(creds.Redacted()),
	} {
		if strings.Contains(rendered, "tok-secret") {
			t.Errorf("%s leaks the token: %s", name, rendered)
		}
		if !strings.Contains(rendered, "https://pve:8006") {
			t.Errorf("%s dropped the endpoint: %s", name, rendered)
		}
	}

	alias := creds.APIToken
	creds.Zeroize()
	if creds.APIToken != nil || creds.IsValid() {
		t.Errorf("APIToken after Zeroize = %v, want nil", creds.APIToken)
	}
	for i, b := range alias {
		if b != 0 {
			t.Fatalf("token byte %d not zeroed", i)
		}
	}
}
