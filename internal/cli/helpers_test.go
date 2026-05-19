package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasProjectMarker_ConfigFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "okdctl.yaml"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasProjectMarker(dir) {
		t.Error("okdctl.yaml present: want true, got false")
	}
}

func TestHasProjectMarker_EnvFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "okdctl.env"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasProjectMarker(dir) {
		t.Error("okdctl.env present: want true, got false")
	}
}

func TestHasProjectMarker_TfState(t *testing.T) {
	dir := t.TempDir()
	envDir := filepath.Join(dir, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "terraform.tfstate"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasProjectMarker(dir) {
		t.Error("terraform.tfstate present: want true, got false")
	}
}

func TestHasProjectMarker_None(t *testing.T) {
	dir := t.TempDir()
	if hasProjectMarker(dir) {
		t.Error("empty dir: want false, got true")
	}
}

func TestWarnIfTfStateOnly_PrimaryPresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "okdctl.yaml"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	warnIfTfStateOnly(dir)
}

func TestWarnIfTfStateOnly_TfStateOnly(t *testing.T) {
	dir := t.TempDir()
	envDir := filepath.Join(dir, "infrastructure", "terraform", "environments", "production")
	if err := os.MkdirAll(envDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "terraform.tfstate"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	warnIfTfStateOnly(dir)
}

func TestWarnIfTfStateOnly_NoMarkers(t *testing.T) {
	dir := t.TempDir()
	warnIfTfStateOnly(dir)
}
