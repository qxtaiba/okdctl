package secretstore

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

func makeEnv(projectRoot, secretsDir string) *addon.Environment {
	settings := map[string]string{}
	if secretsDir != "" {
		settings[SettingSecretsDir] = secretsDir
	}
	return &addon.Environment{
		AddonConfig: config.AddonConfig{Settings: settings},
		Logger:      logutil.NopLogger,
		ProjectRoot: projectRoot,
	}
}

func TestResolveSecretsDir_Absolute(t *testing.T) {
	absDir := "/srv/secrets"
	env := makeEnv("/project/root", absDir)
	got := resolveSecretsDir(env)
	if got != absDir {
		t.Errorf("resolveSecretsDir absolute = %q; want %q", got, absDir)
	}
}

func TestResolveSecretsDir_Relative(t *testing.T) {
	root := "/project/root"
	env := makeEnv(root, "secrets/provider")
	got := resolveSecretsDir(env)
	want := filepath.Join(root, "secrets", "provider")
	if got != want {
		t.Errorf("resolveSecretsDir relative = %q; want %q", got, want)
	}
}

func TestResolveSecretsDir_Empty(t *testing.T) {
	root := "/project/root"
	env := makeEnv(root, "")
	got := resolveSecretsDir(env)
	want := filepath.Join(root, defaultSecretsDir)
	if got != want {
		t.Errorf("resolveSecretsDir empty = %q; want %q", got, want)
	}
}

// Anchors current permissive behaviour: SecretsDir="../etc" is joined under
// ProjectRoot via filepath.Join's Clean. Future allowlist hardening should
// update this assertion.
func TestResolveSecretsDir_TraversalAnchor(t *testing.T) {
	root := "/project/root"
	env := makeEnv(root, "../etc")
	got := resolveSecretsDir(env)
	want := filepath.Join(root, "..", "etc")
	if got != want {
		t.Errorf("resolveSecretsDir traversal = %q; want %q", got, want)
	}
}

func TestSecretManifestFromFile(t *testing.T) {
	tmp := t.TempDir()
	tokenFile := filepath.Join(tmp, "vault-token.txt")
	tokenValue := "s.mysecrettoken"
	if err := os.WriteFile(tokenFile, []byte(tokenValue+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	env := makeEnv(tmp, tmp)
	manifest, err := secretManifestFromFile(context.Background(), env, tokenFile, "vault-token", "token")
	if err != nil {
		t.Fatalf("secretManifestFromFile error: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(manifest), &parsed); err != nil {
		t.Fatalf("manifest YAML invalid: %v\n%s", err, manifest)
	}
	if parsed["kind"] != "Secret" {
		t.Errorf("kind = %v; want Secret", parsed["kind"])
	}
	if parsed["apiVersion"] != "v1" {
		t.Errorf("apiVersion = %v; want v1", parsed["apiVersion"])
	}
	if parsed["type"] != "Opaque" {
		t.Errorf("type = %v; want Opaque", parsed["type"])
	}

	dataMap, ok := parsed["data"].(map[string]any)
	if !ok {
		t.Fatalf("data field missing or wrong type: %T", parsed["data"])
	}
	encoded, ok := dataMap["token"].(string)
	if !ok {
		t.Fatalf("data[token] missing or wrong type: %T", dataMap["token"])
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode data[token]: %v", err)
	}
	if string(decoded) != tokenValue {
		t.Errorf("data[token] decoded = %q; want %q (trailing newline must be trimmed)", string(decoded), tokenValue)
	}

	if strings.Contains(manifest, tokenValue) {
		t.Errorf("manifest leaks plaintext token: %s", manifest)
	}
}

func TestReadSecret_RejectsSymlink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "real.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	env := makeEnv(tmp, tmp)
	if _, err := readSecret(context.Background(), env, link); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink-refusal error, got %v", err)
	}
}

func TestReadSecret_PermRefusal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the 0o077 permission gate")
	}
	const contents = "s.plaintext-provider-token"
	tests := []struct {
		name       string
		perm       os.FileMode
		wantReject bool
	}{
		{"0600 accepted", 0o600, false},
		{"0400 accepted", 0o400, false},
		{"0640 group-readable rejected", 0o640, true},
		{"0604 other-readable rejected", 0o604, true},
		{"0644 rejected", 0o644, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			path := filepath.Join(tmp, "vault-token.txt")
			if err := os.WriteFile(path, []byte(contents), tc.perm); err != nil {
				t.Fatal(err)
			}
			// Explicit chmod in case umask narrowed the create mode.
			if err := os.Chmod(path, tc.perm); err != nil {
				t.Fatal(err)
			}

			env := makeEnv(tmp, tmp)
			got, err := readSecret(context.Background(), env, path)
			if tc.wantReject {
				if err == nil {
					t.Fatalf("perm %#o: expected refusal, got nil error", tc.perm)
				}
				if !strings.Contains(err.Error(), "insecure permissions") {
					t.Errorf("perm %#o: error %q does not name the permission failure", tc.perm, err)
				}
				if strings.Contains(err.Error(), contents) {
					t.Errorf("perm %#o: error leaks file contents: %q", tc.perm, err)
				}
				if !strings.Contains(err.Error(), "vault-token.txt") {
					t.Errorf("perm %#o: error should name the basename; got %q", tc.perm, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("perm %#o: unexpected error: %v", tc.perm, err)
			}
			if got != contents {
				t.Errorf("perm %#o: readSecret = %q; want %q", tc.perm, got, contents)
			}
		})
	}
}

func TestSecretManifestFromFile_NamespaceAndName(t *testing.T) {
	tmp := t.TempDir()
	credFile := filepath.Join(tmp, "creds.json")
	if err := os.WriteFile(credFile, []byte(`{"key":"val"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	env := makeEnv(tmp, tmp)
	manifest, err := secretManifestFromFile(context.Background(), env, credFile, "onepassword-connect-credentials", "credentials_base64")
	if err != nil {
		t.Fatalf("secretManifestFromFile error: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(manifest), &parsed); err != nil {
		t.Fatalf("manifest YAML invalid: %v", err)
	}
	meta, _ := parsed["metadata"].(map[string]any)
	if meta == nil {
		t.Fatal("metadata missing")
	}
	if meta["name"] != "onepassword-connect-credentials" {
		t.Errorf("metadata.name = %v; want onepassword-connect-credentials", meta["name"])
	}
	if meta["namespace"] != defaultNamespace {
		t.Errorf("metadata.namespace = %v; want %s", meta["namespace"], defaultNamespace)
	}
	if _, ok := parsed["data"].(map[string]any)["credentials_base64"]; !ok {
		t.Errorf("data[credentials_base64] key missing")
	}
}
