package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/workspace"
	"github.com/spf13/cobra"
)

func seedKubeconfigWorkspace(t *testing.T) []byte {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile(filepath.Join(root, "okdctl.yaml"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	authDir := filepath.Join(root, workspace.WorkDirName, "cluster-config", "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("apiVersion: v1\nkind: Config\nusers:\n- name: admin\n")
	if err := os.WriteFile(filepath.Join(authDir, "kubeconfig"), want, 0o600); err != nil {
		t.Fatal(err)
	}
	return want
}

func setKubeconfigOutput(t *testing.T, output string) {
	t.Helper()
	orig := kubeconfigOutput
	t.Cleanup(func() { kubeconfigOutput = orig })
	kubeconfigOutput = output
}

func TestRunKubeconfig_OutputFilePerms(t *testing.T) {
	want := seedKubeconfigWorkspace(t)
	dest := filepath.Join(t.TempDir(), "sub", "okd.kubeconfig")
	setKubeconfigOutput(t, dest)

	cmd := &cobra.Command{}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err := runKubeconfig(cmd, nil); err != nil {
		t.Fatalf("runKubeconfig: %v", err)
	}

	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("output file not written: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("output file perm = %#o, want 0600 (kubeconfig is a credential)", perm)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output file bytes = %q, want %q", got, want)
	}
	if stdout.Len() != 0 {
		t.Errorf("--output-file must not also print the credential to stdout, got %q", stdout.String())
	}
}

func TestRunKubeconfig_StdoutDefault(t *testing.T) {
	want := seedKubeconfigWorkspace(t)
	setKubeconfigOutput(t, "-")

	cmd := &cobra.Command{}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err := runKubeconfig(cmd, nil); err != nil {
		t.Fatalf("runKubeconfig: %v", err)
	}
	if !bytes.Equal(stdout.Bytes(), want) {
		t.Errorf("stdout = %q, want the kubeconfig bytes", stdout.String())
	}
	if _, err := os.Stat("-"); !os.IsNotExist(err) {
		t.Error("a literal '-' file must not be created")
	}
}

func TestRunKubeconfig_MissingSourceIsConfigError(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile(filepath.Join(root, "okdctl.yaml"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "okd.kubeconfig")
	setKubeconfigOutput(t, dest)

	err := runKubeconfig(&cobra.Command{}, nil)
	if !errors.Is(err, errtypes.ErrConfigMissing) {
		t.Fatalf("want ErrConfigMissing, got: %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Error("no output file may be created when the source kubeconfig is missing")
	}
}
