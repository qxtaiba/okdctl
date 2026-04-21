package addon

import (
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestBuildOpaqueSecret(t *testing.T) {
	data := map[string][]byte{
		"username": []byte("root@pam"),
		"password": []byte("hunter2"),
	}
	out, err := BuildOpaqueSecret("flux-system", "creds", data)
	if err != nil {
		t.Fatal(err)
	}

	// Round-trip: parse the YAML and check structure.
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("emitted YAML does not re-parse: %v\n%s", err, out)
	}

	if parsed["apiVersion"] != "v1" {
		t.Errorf("apiVersion = %v, want v1", parsed["apiVersion"])
	}
	if parsed["kind"] != "Secret" {
		t.Errorf("kind = %v, want Secret", parsed["kind"])
	}
	if parsed["type"] != "Opaque" {
		t.Errorf("type = %v, want Opaque", parsed["type"])
	}
	meta, _ := parsed["metadata"].(map[string]any)
	if meta == nil {
		t.Fatal("metadata missing")
	}
	if meta["name"] != "creds" || meta["namespace"] != "flux-system" {
		t.Errorf("metadata name/namespace wrong: %+v", meta)
	}

	// Values should be base64-encoded, not plaintext.
	if strings.Contains(out, "hunter2") {
		t.Errorf("secret body leaks plaintext: %s", out)
	}
	if strings.Contains(out, "root@pam") {
		t.Errorf("secret body leaks plaintext username: %s", out)
	}
}

func TestBuildOpaqueSecret_Empty(t *testing.T) {
	out, err := BuildOpaqueSecret("ns", "name", map[string][]byte{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "kind: Secret") {
		t.Errorf("empty-data secret missing kind: %s", out)
	}
}
