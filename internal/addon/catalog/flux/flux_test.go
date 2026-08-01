package flux

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestBuildInstanceValues(t *testing.T) {
	fs := Settings{
		Repository: "ssh://git@github.com/org/repo.git",
		Branch:     "feature",
		Path:       "k8s/prod",
	}
	b, err := buildInstanceValues(&fs)
	if err != nil {
		t.Fatalf("buildInstanceValues: %v", err)
	}
	var v map[string]any
	if err := yaml.Unmarshal(b, &v); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, b)
	}
	inst, _ := v["instance"].(map[string]any)
	if inst == nil {
		t.Fatal("instance key missing")
	}
	sync, _ := inst["sync"].(map[string]any)
	if sync == nil {
		t.Fatal("instance.sync key missing")
	}
	if sync["url"] != fs.Repository {
		t.Errorf("url = %v, want %v", sync["url"], fs.Repository)
	}
	if sync["ref"] != "refs/heads/feature" {
		t.Errorf("ref = %v, want refs/heads/feature", sync["ref"])
	}
	if sync["path"] != "k8s/prod" {
		t.Errorf("path = %v, want k8s/prod", sync["path"])
	}
	cluster, _ := inst["cluster"].(map[string]any)
	if cluster == nil {
		t.Fatal("instance.cluster key missing")
	}
	if cluster["type"] != "openshift" {
		t.Errorf("cluster.type = %v, want openshift", cluster["type"])
	}
}

func TestBuildFluxDeployKeySecret(t *testing.T) {
	t.Run("all fields present", func(t *testing.T) {
		manifest, err := buildFluxDeployKeySecret(
			"flux-system", "flux-system",
			[]byte("PRIVATE_KEY_DATA\n"), []byte("PUBLIC_KEY_DATA\n"), []byte("github.com ssh-ed25519 AAAA\n"),
		)
		if err != nil {
			t.Fatal(err)
		}

		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(manifest), &parsed); err != nil {
			t.Fatalf("emitted YAML does not re-parse: %v\n%s", err, manifest)
		}

		if parsed["kind"] != "Secret" {
			t.Errorf("kind = %v, want Secret", parsed["kind"])
		}
		if parsed["type"] != "Opaque" {
			t.Errorf("type = %v, want Opaque", parsed["type"])
		}

		dataRaw, _ := parsed["data"].(map[string]any)
		if dataRaw == nil {
			t.Fatal("data map missing")
		}
		for _, key := range []string{"identity", "identity.pub", "known_hosts"} {
			if dataRaw[key] == nil {
				t.Errorf("data[%q] missing", key)
			}
		}

		// Plaintext must NOT appear in the YAML (base64-encoded values only).
		for _, plain := range []string{"PRIVATE_KEY_DATA", "PUBLIC_KEY_DATA", "github.com ssh-ed25519 AAAA"} {
			if strings.Contains(manifest, plain) {
				t.Errorf("manifest leaks plaintext %q:\n%s", plain, manifest)
			}
		}
	})

	t.Run("empty publicKey omits identity.pub", func(t *testing.T) {
		manifest, err := buildFluxDeployKeySecret(
			"flux-system", "flux-system",
			[]byte("PRIVATE_KEY_DATA\n"), nil, []byte("github.com ssh-ed25519 AAAA\n"),
		)
		if err != nil {
			t.Fatal(err)
		}

		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(manifest), &parsed); err != nil {
			t.Fatalf("emitted YAML does not re-parse: %v\n%s", err, manifest)
		}
		dataRaw, _ := parsed["data"].(map[string]any)
		if dataRaw == nil {
			t.Fatal("data map missing")
		}
		if dataRaw["identity.pub"] != nil {
			t.Error("identity.pub should be absent when publicKey is empty")
		}
		if dataRaw["identity"] == nil {
			t.Error("identity must be present")
		}
		if dataRaw["known_hosts"] == nil {
			t.Error("known_hosts must be present")
		}
	})

	t.Run("namespace and name set correctly", func(t *testing.T) {
		manifest, err := buildFluxDeployKeySecret("my-ns", "my-secret", []byte("key\n"), nil, []byte("host key\n"))
		if err != nil {
			t.Fatal(err)
		}
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(manifest), &parsed); err != nil {
			t.Fatalf("emitted YAML does not re-parse: %v\n%s", err, manifest)
		}
		meta, _ := parsed["metadata"].(map[string]any)
		if meta == nil {
			t.Fatal("metadata missing")
		}
		if meta["name"] != "my-secret" {
			t.Errorf("name = %v, want my-secret", meta["name"])
		}
		if meta["namespace"] != "my-ns" {
			t.Errorf("namespace = %v, want my-ns", meta["namespace"])
		}
	})
}

func TestValidateSettingsUserinfo(t *testing.T) {
	f := &fluxAddon{}
	cases := []struct {
		name    string
		repo    string
		wantErr bool
	}{
		{name: "ssh scp style", repo: "git@github.com:org/repo.git", wantErr: false},
		{name: "ssh scheme no creds", repo: "ssh://git@github.com/org/repo.git", wantErr: false},
		{name: "https no creds", repo: "https://github.com/org/repo.git", wantErr: false},
		{name: "https with token", repo: "https://user:ghp_token@github.com/org/repo.git", wantErr: true},
		{name: "https user only", repo: "https://user@github.com/org/repo.git", wantErr: true},
		{name: "empty repo", repo: "", wantErr: true},
		{name: "bad scheme", repo: "ftp://example.com/repo", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := map[string]string{
				SettingRepository: tc.repo,
				SettingBranch:     "main",
				SettingPath:       "kubernetes/clusters/production",
			}
			errs := f.ValidateSettings(settings)
			if tc.wantErr && len(errs) == 0 {
				t.Errorf("ValidateSettings(%q) = nil errors, want at least one", tc.repo)
			}
			if !tc.wantErr && len(errs) != 0 {
				t.Errorf("ValidateSettings(%q) = %v, want no errors", tc.repo, errs)
			}
		})
	}
}

const fixtureGitKeyscanOutput = "# github.com:22 SSH-2.0-babeld-1234\n" +
	"github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl\n" +
	"github.com ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBEmKSENjQEezOmxkZMy7opKgwFB9nkt5YRrYMjNuG5N87uRgg6CLrbo5wAdT/y6v0mKV0U2w0WZ2YB/++Tpockg=\n"

func gitFingerprintFromFixture(t *testing.T, keyLine string) string {
	t.Helper()
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(keyLine))
	if err != nil {
		t.Fatalf("fixture key unparseable: %v", err)
	}
	return ssh.FingerprintSHA256(key)
}

func TestVerifyKeyscanFingerprint_Match(t *testing.T) {
	keyLine := "github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl"
	fp := gitFingerprintFromFixture(t, keyLine)
	if err := verifyKeyscanFingerprint(fixtureGitKeyscanOutput, "github.com", fp, false, logutil.NopLogger); err != nil {
		t.Fatalf("unexpected err on match: %v", err)
	}
}

func TestVerifyKeyscanFingerprint_Mismatch(t *testing.T) {
	wrong := "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	err := verifyKeyscanFingerprint(fixtureGitKeyscanOutput, "github.com", wrong, false, logutil.NopLogger)
	if err == nil {
		t.Fatal("expected error for mismatch; got nil")
	}
	if !strings.Contains(err.Error(), wrong) {
		t.Errorf("error %q missing expected fingerprint", err.Error())
	}
}

func TestVerifyKeyscanFingerprint_EmptyExpected_FailClosed(t *testing.T) {
	err := verifyKeyscanFingerprint(fixtureGitKeyscanOutput, "github.com", "", false, logutil.NopLogger)
	if err == nil {
		t.Fatal("expected fail-closed error when expected empty and acceptHostKey=false; got nil")
	}
	if !strings.Contains(err.Error(), "accept_host_key=true") {
		t.Errorf("fail-closed error %q should mention the accept_host_key opt-out", err.Error())
	}
}

func TestVerifyKeyscanFingerprint_EmptyExpected_AcceptHostKey(t *testing.T) {
	if err := verifyKeyscanFingerprint(fixtureGitKeyscanOutput, "github.com", "", true, logutil.NopLogger); err != nil {
		t.Fatalf("unexpected err with acceptHostKey=true: %v", err)
	}
}

func TestFilterKeyscanLines(t *testing.T) {
	got := string(filterKeyscanLines(fixtureGitKeyscanOutput))
	if strings.Contains(got, "#") {
		t.Errorf("filterKeyscanLines output contains comment line: %q", got)
	}
	if !strings.Contains(got, "ssh-ed25519") {
		t.Errorf("filterKeyscanLines output missing ed25519 key line")
	}
	if !strings.Contains(got, "ecdsa-sha2-nistp256") {
		t.Errorf("filterKeyscanLines output missing ecdsa key line")
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("filterKeyscanLines: got %d lines, want 2: %q", len(lines), got)
	}
}

func TestGitHost(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{name: "ssh scheme", url: "ssh://git@github.com/org/repo.git", want: "github.com"},
		{name: "https scheme", url: "https://github.com/org/repo.git", want: "github.com"},
		{name: "scp style", url: "git@github.com:org/repo.git", want: "github.com"},
		{name: "scp self-hosted", url: "git@gitlab.internal.example.com:team/repo.git", want: "gitlab.internal.example.com"},
		{name: "ssh with port", url: "ssh://git@bitbucket.org:7999/proj/repo.git", want: "bitbucket.org"},
		{name: "ssh with short host and port", url: "ssh://git@host:2222/o/r", want: "host"},
		{name: "ssh with ipv6 and port", url: "ssh://git@[2001:db8::1]:2222/o/r", want: "2001:db8::1"},
		{name: "empty", url: "", wantErr: true},
		{name: "whitespace only", url: "   ", wantErr: true},
		{name: "no-host scp", url: "no-host", wantErr: true},
		{name: "malformed scheme", url: "://nope", wantErr: true},
		{name: "scheme only", url: "http://", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := gitHost(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("gitHost(%q) = %q, nil; want error", tc.url, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("gitHost(%q) unexpected error: %v", tc.url, err)
			}
			if got != tc.want {
				t.Errorf("gitHost(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestValidateSettings_MalformedTimeout(t *testing.T) {
	f := &fluxAddon{}
	settings := map[string]string{
		SettingRepository:        "ssh://git@github.com/org/repo.git",
		SettingControllerTimeout: "not-a-number",
	}
	errs := f.ValidateSettings(settings)
	if len(errs) == 0 {
		t.Fatal("ValidateSettings with malformed controller_timeout = no errors, want at least one")
	}
}

// TestReadKeyFile locks the symlink guard on the deploy-key read: a
// symlinked ~/.ssh/flux-deploy-key must fail closed, or a hostile link
// could exfiltrate an arbitrary root-readable file into a cluster Secret.
// Mirrors setup's readNoFollow symlink-rejection test.
func TestReadKeyFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("regular file returns bytes", func(t *testing.T) {
		path := filepath.Join(dir, "key")
		want := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n")
		if err := os.WriteFile(path, want, 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := readKeyFile(path)
		if err != nil {
			t.Fatalf("readKeyFile: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("readKeyFile = %q, want %q", got, want)
		}
	})

	t.Run("symlink at final component refused", func(t *testing.T) {
		target := filepath.Join(dir, "target")
		if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := readKeyFile(link); err == nil || !strings.Contains(err.Error(), "refusing to follow") {
			t.Errorf("symlink read must fail closed with a refusal, got: %v", err)
		}
	})

	t.Run("missing file maps to ErrNotExist", func(t *testing.T) {
		// createDeployKeySecret treats a missing .pub as optional via
		// errors.Is(err, os.ErrNotExist); the identity error type is load-bearing.
		_, err := readKeyFile(filepath.Join(dir, "absent"))
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("want os.ErrNotExist for a missing key, got: %v", err)
		}
	})
}
