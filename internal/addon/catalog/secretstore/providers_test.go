package secretstore

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestParseOnepasswordVaults(t *testing.T) {
	accept := []struct {
		name string
		in   string
		want map[string]int
	}{
		{"empty defaults", "", map[string]int{"homelab": 1}},
		{"single", "homelab=1", map[string]int{"homelab": 1}},
		{"multiple", "a=1,b=2", map[string]int{"a": 1, "b": 2}},
		{"padded", "  a = 1 , b = 2 ", map[string]int{"a": 1, "b": 2}},
		{"dotted name", "team.prod=3", map[string]int{"team.prod": 3}},
	}
	for _, tc := range accept {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			got, err := parseOnepasswordVaults(tc.in)
			if err != nil {
				t.Fatalf("parseOnepasswordVaults(%q) error: %v", tc.in, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("parseOnepasswordVaults(%q) = %v; want %v", tc.in, got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("parseOnepasswordVaults(%q)[%q] = %d; want %d", tc.in, k, got[k], v)
				}
			}
		})
	}

	reject := []struct {
		name string
		in   string
	}{
		{"no equals", "homelab"},
		{"empty name", "=1"},
		{"non-numeric priority", "a=x"},
		{"newline in name", "evil\nkey=1"},
		{"colon injection in name", "a: b=1"},
	}
	for _, tc := range reject {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			if got, err := parseOnepasswordVaults(tc.in); err == nil {
				t.Errorf("parseOnepasswordVaults(%q) = %v, nil; want error", tc.in, got)
			}
		})
	}
}

func TestProviderValidate(t *testing.T) {
	t.Run("vault", func(t *testing.T) {
		p := &vaultProvider{}
		cases := []struct {
			name    string
			server  string
			wantErr bool
		}{
			{"https accepted", "https://vault.example.com", false},
			{"http accepted", "http://vault.example.com:8200", false},
			{"empty rejected", "", true},
			{"schemeless rejected", "vault.example.com", true},
			{"file scheme rejected", "file:///etc/passwd", true},
			{"newline injection rejected", "https://vault\n      injected: true", true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				errs := p.validate(Settings{Vault: &vaultSettings{Server: tc.server, Path: "secret", Version: "v2"}})
				if (len(errs) > 0) != tc.wantErr {
					t.Errorf("validate(server=%q) errs=%v; wantErr=%v", tc.server, errs, tc.wantErr)
				}
			})
		}
	})

	t.Run("bitwarden", func(t *testing.T) {
		p := &bitwardenProvider{}
		valid := func() *bitwardenSettings {
			return &bitwardenSettings{
				OrganizationID: "org-1",
				ProjectID:      "proj-1",
				APIURL:         defaultBitwardenAPIURL,
				IdentityURL:    defaultBitwardenIdentityURL,
				SDKServerURL:   defaultBitwardenSDKServerURL,
			}
		}
		if errs := p.validate(Settings{Bitwarden: valid()}); len(errs) > 0 {
			t.Errorf("valid bitwarden settings rejected: %v", errs)
		}

		noOrg := valid()
		noOrg.OrganizationID = ""
		if errs := p.validate(Settings{Bitwarden: noOrg}); len(errs) == 0 {
			t.Error("empty organization_id accepted; want error")
		}
		noProj := valid()
		noProj.ProjectID = ""
		if errs := p.validate(Settings{Bitwarden: noProj}); len(errs) == 0 {
			t.Error("empty project_id accepted; want error")
		}
		badURL := valid()
		badURL.APIURL = "not-a-url"
		if errs := p.validate(Settings{Bitwarden: badURL}); len(errs) == 0 {
			t.Error("non-URL api_url accepted; want error")
		}
	})

	t.Run("onepassword", func(t *testing.T) {
		p := &onepasswordProvider{}
		ok := Settings{OnePassword: &onePasswordSettings{
			ConnectHost: defaultOPConnectHost,
			Vaults:      map[string]int{"homelab": 1},
		}}
		if errs := p.validate(ok); len(errs) > 0 {
			t.Errorf("valid onepassword settings rejected: %v", errs)
		}
		badHost := Settings{OnePassword: &onePasswordSettings{
			ConnectHost: "onepassword-connect:8080",
			Vaults:      map[string]int{"homelab": 1},
		}}
		if errs := p.validate(badHost); len(errs) == 0 {
			t.Error("schemeless connect_host accepted; want error")
		}
		badVault := Settings{OnePassword: &onePasswordSettings{
			ConnectHost: defaultOPConnectHost,
			Vaults:      map[string]int{"evil name": 1},
		}}
		if errs := p.validate(badVault); len(errs) == 0 {
			t.Error("invalid vault name accepted; want error")
		}
	})
}

// providerBlock unmarshals manifest into a map so tests assert structure, not YAML text.
func providerBlock(t *testing.T, manifest string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(manifest), &doc); err != nil {
		t.Fatalf("manifest is not valid YAML: %v\n%s", err, manifest)
	}
	if doc["apiVersion"] != "external-secrets.io/v1beta1" {
		t.Errorf("apiVersion = %v; want external-secrets.io/v1beta1", doc["apiVersion"])
	}
	if doc["kind"] != "SecretStore" {
		t.Errorf("kind = %v; want SecretStore", doc["kind"])
	}
	spec, _ := doc["spec"].(map[string]any)
	prov, _ := spec["provider"].(map[string]any)
	if prov == nil {
		t.Fatalf("spec.provider missing: %s", manifest)
	}
	return prov
}

func TestBuildVaultSecretStoreCRD_Injection(t *testing.T) {
	// Would inject a sibling YAML key under naive fmt.Sprintf interpolation;
	// must stay a scalar string.
	evil := "https://vault\n      injected: pwned"
	manifest, err := buildVaultSecretStoreCRD(evil, "secret", "v2")
	if err != nil {
		t.Fatalf("buildVaultSecretStoreCRD: %v", err)
	}
	prov := providerBlock(t, manifest)
	vault, _ := prov["vault"].(map[string]any)
	if vault == nil {
		t.Fatalf("provider.vault missing: %s", manifest)
	}
	if _, injected := vault["injected"]; injected {
		t.Errorf("YAML injection succeeded: 'injected' key present\n%s", manifest)
	}
	if vault["server"] != evil {
		t.Errorf("server = %q; want the raw scalar %q", vault["server"], evil)
	}
}

func TestBuildOPSecretStoreCRD(t *testing.T) {
	manifest, err := buildOPSecretStoreCRD("http://onepassword-connect:8080", map[string]int{"homelab": 1, "shared": 2})
	if err != nil {
		t.Fatalf("buildOPSecretStoreCRD: %v", err)
	}
	prov := providerBlock(t, manifest)
	op, _ := prov["onepassword"].(map[string]any)
	if op == nil {
		t.Fatalf("provider.onepassword missing: %s", manifest)
	}
	if op["connectHost"] != "http://onepassword-connect:8080" {
		t.Errorf("connectHost = %v", op["connectHost"])
	}
	vaults, _ := op["vaults"].(map[string]any)
	if len(vaults) != 2 {
		t.Fatalf("vaults = %v; want 2 entries", vaults)
	}
}

func TestBuildBitwardenSecretStoreCRD(t *testing.T) {
	manifest, err := buildBitwardenSecretStoreCRD(
		defaultBitwardenAPIURL, defaultBitwardenIdentityURL, defaultBitwardenSDKServerURL,
		"org-1", "proj-1")
	if err != nil {
		t.Fatalf("buildBitwardenSecretStoreCRD: %v", err)
	}
	prov := providerBlock(t, manifest)
	bw, _ := prov["bitwardensecretsmanager"].(map[string]any)
	if bw == nil {
		t.Fatalf("provider.bitwardensecretsmanager missing: %s", manifest)
	}
	if bw["organizationID"] != "org-1" || bw["projectID"] != "proj-1" {
		t.Errorf("org/project = %v/%v; want org-1/proj-1", bw["organizationID"], bw["projectID"])
	}
	auth, _ := bw["auth"].(map[string]any)
	secretRef, _ := auth["secretRef"].(map[string]any)
	creds, _ := secretRef["credentials"].(map[string]any)
	if creds["name"] != bitwardenTokenSecretName {
		t.Errorf("auth credentials name = %v; want %s", creds["name"], bitwardenTokenSecretName)
	}
}
