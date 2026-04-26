package cli

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
)

// stubAddFile writes addFile callbacks as plain tar entries into tw.
func stubAddFile(tw *tar.Writer) func(string, []byte) error {
	return func(name string, data []byte) error {
		hdr := &tar.Header{
			Name:    name,
			Mode:    0o600,
			Size:    int64(len(data)),
			ModTime: time.Now(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}
}

// readTarEntries closes tw, then reads every entry into the returned map.
// Callers must not use tw after calling readTarEntries.
func readTarEntries(t *testing.T, tw *tar.Writer, buf *bytes.Buffer) map[string][]byte {
	t.Helper()
	if err := tw.Close(); err != nil {
		t.Fatalf("tar.Writer.Close: %v", err)
	}
	tr := tar.NewReader(bytes.NewReader(buf.Bytes()))
	out := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("io.ReadAll %s: %v", hdr.Name, err)
		}
		out[hdr.Name] = data
	}
	return out
}

func TestBundleConfigRedactsCredentials(t *testing.T) {
	const secretToken = "root@pam!very-secret-token-id"
	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Type: config.ProviderProxmox,
			Proxmox: &config.ProxmoxConfig{
				Host:    "pve.example",
				TokenID: secretToken,
			},
		},
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	entry := bundleConfig(stubAddFile(tw), cfg, nil)
	if entry.Status != "ok" {
		t.Fatalf("bundleConfig status = %q, want ok; message: %s", entry.Status, entry.Message)
	}
	entries := readTarEntries(t, tw, &buf)
	data, ok := entries["config.yaml"]
	if !ok {
		t.Fatal("config.yaml not found in tar")
	}
	s := string(data)
	if strings.Contains(s, secretToken) {
		t.Errorf("raw TokenID leaked into config.yaml:\n%s", s)
	}
	if !strings.Contains(s, "***") {
		t.Errorf("expected *** placeholder in config.yaml; got:\n%s", s)
	}
}

func TestBundleLogFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "okdctl.log")
	if err := os.WriteFile(logPath, []byte("line1\nline2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	orig := logFile
	logFile = logPath
	t.Cleanup(func() { logFile = orig })

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	entry := bundleLogFile(stubAddFile(tw))
	if entry.Status != "ok" {
		t.Fatalf("bundleLogFile status = %q; message: %s", entry.Status, entry.Message)
	}
	entries := readTarEntries(t, tw, &buf)
	data, ok := entries["okdctl.log"]
	if !ok {
		t.Fatal("okdctl.log not found in tar")
	}
	if string(data) != "line1\nline2\n" {
		t.Errorf("unexpected log content: %q", string(data))
	}
}

func TestTarDirIntoRejectsSymlinkEscape(t *testing.T) {
	srcDir := t.TempDir()
	outside := t.TempDir()

	if err := os.WriteFile(filepath.Join(srcDir, "real.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(srcDir, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := tarDirInto(stubAddFile(tw), srcDir, "test/")
	_ = tw.Close()

	if err != nil {
		// os.Root blocked the escape — acceptable outcome.
		return
	}
	// filepath.WalkDir marks directory symlinks non-regular, so tarDirInto
	// skips them before os.Root fires. Verify the escaped file is absent.
	tr := tar.NewReader(bytes.NewReader(buf.Bytes()))
	for {
		hdr, nextErr := tr.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatalf("tar.Next: %v", nextErr)
		}
		if strings.Contains(hdr.Name, "secret.txt") {
			t.Errorf("escaped file %q present in archive", hdr.Name)
		}
	}
}

func TestCollectSectionsSkipMustGather(t *testing.T) {
	add := func(string, []byte) error { return nil }
	// cfgErr/prErr non-nil so bundleConfig and bundleTerraformState skip
	// without dereferencing nil cfg or stat-ing an empty path.
	secs := collectSections(
		context.Background(),
		add,
		nil,
		errors.New("no config"),
		"",
		errors.New("no project root"),
		time.Now(),
		"test-bundle-id",
		true,
	)
	var mg *manifestEntry
	for i := range secs {
		if secs[i].Name == "must-gather" {
			mg = &secs[i]
			break
		}
	}
	if mg == nil {
		t.Fatal("must-gather entry not found in sections")
	}
	if mg.Status != "skipped" {
		t.Errorf("Status = %q, want skipped", mg.Status)
	}
	if mg.Message != "--skip-must-gather flag set" {
		t.Errorf("Message = %q, want --skip-must-gather flag set", mg.Message)
	}
}
