package fetchplan_test

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/fetchplan"
)

func TestEnvOverrideResolver_blobEnvSet(t *testing.T) {
	t.Setenv(fetchplan.EnvSCOSStreamURL, "https://override.example.com/scos.json")
	r := fetchplan.EnvOverrideResolver{
		Inner:     fetchplan.DefaultResolver{},
		Overrides: fetchplan.DefaultEnvOverrides(),
	}
	got, err := r.ResolveBlob(fetchplan.Blob{
		URL:     "https://raw.githubusercontent.com/openshift/installer/release-4.19/data/data/coreos/scos.json",
		Purpose: fetchplan.PurposeCoreOSStream,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://override.example.com/scos.json" {
		t.Errorf("env override not applied; got %q", got)
	}
}

func TestEnvOverrideResolver_blobEnvUnset(t *testing.T) {
	t.Setenv(fetchplan.EnvSCOSStreamURL, "")
	r := fetchplan.EnvOverrideResolver{
		Inner:     fetchplan.DefaultResolver{},
		Overrides: fetchplan.DefaultEnvOverrides(),
	}
	const upstream = "https://raw.githubusercontent.com/openshift/installer/release-4.19/data/data/coreos/scos.json"
	got, err := r.ResolveBlob(fetchplan.Blob{URL: upstream, Purpose: fetchplan.PurposeCoreOSStream})
	if err != nil || got != upstream {
		t.Errorf("expected passthrough when env unset; got (%q, %v)", got, err)
	}
}

func TestEnvOverrideResolver_unknownPurposePassthrough(t *testing.T) {
	r := fetchplan.EnvOverrideResolver{
		Inner:     fetchplan.DefaultResolver{},
		Overrides: fetchplan.DefaultEnvOverrides(),
	}
	const u = "https://example.com/file.bin"
	got, err := r.ResolveBlob(fetchplan.Blob{URL: u, Purpose: "unknown-purpose"})
	if err != nil || got != u {
		t.Errorf("unknown purpose should pass through; got (%q, %v)", got, err)
	}
}

func TestEnvOverrideResolver_ociEnvSet(t *testing.T) {
	t.Setenv(fetchplan.EnvBootstrapOCURL, "https://override.example.com/oc.tar.gz")
	r := fetchplan.EnvOverrideResolver{
		Inner:     fetchplan.DefaultResolver{},
		Overrides: fetchplan.DefaultEnvOverrides(),
	}
	got, err := r.ResolveOCI(fetchplan.OCIArtifact{
		Ref:     "quay.io/okd/scos-release:4.21",
		Purpose: fetchplan.PurposeBootstrapOC,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://override.example.com/oc.tar.gz" {
		t.Errorf("OCI env override not applied; got %q", got)
	}
}

func TestBuildUpdateCheckPlan(t *testing.T) {
	p := fetchplan.BuildUpdateCheckPlan()
	if len(p.HTTPS) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(p.HTTPS))
	}
	b := p.HTTPS[0]
	if b.Purpose != fetchplan.PurposeUpdateCheck {
		t.Errorf("purpose = %q, want %q", b.Purpose, fetchplan.PurposeUpdateCheck)
	}
	if !strings.Contains(b.URL, "api.github.com") {
		t.Errorf("URL %q missing api.github.com", b.URL)
	}
}

func TestBuildCoreOSISOPlan(t *testing.T) {
	const isoURL = "https://rhcos.mirror.openshift.com/art/storage/scos.iso"
	const sha = "deadbeef"
	p := fetchplan.BuildCoreOSISOPlan(isoURL, sha)
	if len(p.HTTPS) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(p.HTTPS))
	}
	b := p.HTTPS[0]
	if b.Purpose != fetchplan.PurposeCoreOSISO {
		t.Errorf("purpose = %q, want %q", b.Purpose, fetchplan.PurposeCoreOSISO)
	}
	if b.URL != isoURL {
		t.Errorf("URL = %q, want %q", b.URL, isoURL)
	}
	if b.SHA256 != sha {
		t.Errorf("SHA256 = %q, want %q", b.SHA256, sha)
	}
}

func TestBuildAddonChartPlan(t *testing.T) {
	const ref = "ghcr.io/controlplaneio-fluxcd/charts/flux-operator"
	p := fetchplan.BuildAddonChartPlan(ref)
	if len(p.OCI) != 1 {
		t.Fatalf("expected 1 OCI artifact, got %d", len(p.OCI))
	}
	a := p.OCI[0]
	if a.Purpose != fetchplan.PurposeAddonChart {
		t.Errorf("purpose = %q, want %q", a.Purpose, fetchplan.PurposeAddonChart)
	}
	if a.Ref != ref {
		t.Errorf("ref = %q, want %q", a.Ref, ref)
	}
}
