package terraform

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPreventDestroyOverrideRealTerraform is the acceptance test for the
// prevent_destroy/override mechanism okdctl's destroy path relies on: every
// other test in this package stubs terraform with a recording script, so
// none of them can catch a typo in override HCL, a wrong resource address,
// or a dropped *_override.tf suffix silently defeating the guard. This test
// runs a real terraform binary against a local-only fixture (the built-in
// terraform_data resource — no cloud provider, no credentials, no network)
// and proves: a resource with lifecycle.prevent_destroy=true refuses
// destroy, and a matching *_override.tf disabling it (Terraform's documented
// override-file merge, loaded after and merged into the original block)
// lifts the refusal.
func TestPreventDestroyOverrideRealTerraform(t *testing.T) {
	terraformPath, err := exec.LookPath("terraform")
	if err != nil {
		t.Skip("skipping TestPreventDestroyOverrideRealTerraform: no terraform binary on PATH; this acceptance test needs a real terraform to exercise the prevent_destroy/override mechanism end to end")
	}
	ctx := t.Context()
	if out, err := exec.CommandContext(ctx, terraformPath, "version").Output(); err == nil {
		t.Logf("using terraform binary at %s: %s", terraformPath, strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]))
	}

	dir := t.TempDir()
	const fixture = `resource "terraform_data" "guarded" {
  input = "guarded"

  lifecycle {
    prevent_destroy = true
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(fixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tf := New(dir)

	if err := tf.Init(ctx); err != nil {
		t.Fatalf("terraform init: %v", err)
	}
	if err := tf.Apply(ctx, ApplyOptions{AutoApprove: true}); err != nil {
		t.Fatalf("terraform apply (create the guarded fixture): %v", err)
	}

	destroyErr := tf.Destroy(ctx, DestroyOptions{AutoApprove: true, UsePlan: true})
	if destroyErr == nil {
		t.Fatal("destroy must be refused while lifecycle.prevent_destroy is true")
	}
	if !strings.Contains(destroyErr.Error(), "prevent_destroy") {
		t.Fatalf("refusal must be reported as a prevent_destroy failure, got: %v", destroyErr)
	}

	const override = `resource "terraform_data" "guarded" {
  lifecycle {
    prevent_destroy = false
  }
}
`
	overridePath := filepath.Join(dir, "guarded_override.tf")
	if !strings.HasSuffix(overridePath, "_override.tf") {
		t.Fatalf("override file %q must end in _override.tf for terraform to merge it", overridePath)
	}
	if err := os.WriteFile(overridePath, []byte(override), 0o644); err != nil {
		t.Fatalf("write override: %v", err)
	}

	if err := tf.Destroy(ctx, DestroyOptions{AutoApprove: true, UsePlan: true}); err != nil {
		t.Fatalf("destroy must succeed once the override lifts prevent_destroy: %v", err)
	}
}
