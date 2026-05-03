package flux

import (
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

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
		{name: "empty", url: "", wantErr: true},
		{name: "whitespace only", url: "   ", wantErr: true},
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
