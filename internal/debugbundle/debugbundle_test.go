package debugbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

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

func stubAddStream(tw *tar.Writer) func(*tar.Header, io.Reader) error {
	return func(hdr *tar.Header, r io.Reader) error {
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err := io.Copy(tw, r)
		return err
	}
}

// readTarEntries closes tw and returns its entries; callers must not use tw afterward.
func readTarEntries(t *testing.T, tw *tar.Writer, buf *bytes.Buffer) map[string][]byte {
	t.Helper()
	if err := tw.Close(); err != nil {
		t.Fatalf("tar.Writer.Close: %v", err)
	}
	tr := tar.NewReader(bytes.NewReader(buf.Bytes()))
	out := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
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
	cases := []struct {
		name       string
		content    string // okdctl.log content written into the temp dir; empty writes no file
		explicit   bool   // pass the log path directly instead of discovering it via projectRoot
		prErr      error
		wantStatus bundleStatus
		wantMsg    string // substring of entry.Message when non-empty
	}{
		{name: "explicit path", content: "line1\nline2\n", explicit: true, prErr: errors.New("unused"), wantStatus: bundleStatusOK},
		// Locks default-log discovery: no --log-file must still find <projectRoot>/okdctl.log.
		{name: "default discovery", content: "deploy failed here\n", wantStatus: bundleStatusOK},
		{name: "skips when no log exists", wantStatus: bundleStatusSkipped, wantMsg: "okdctl.log"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			logPath, projectRoot := "", dir
			if tc.explicit {
				logPath, projectRoot = filepath.Join(dir, "okdctl.log"), ""
			}
			if tc.content != "" {
				if err := os.WriteFile(filepath.Join(dir, "okdctl.log"), []byte(tc.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			entry := bundleLogFile(stubAddFile(tw), logPath, projectRoot, tc.prErr)
			if entry.Status != tc.wantStatus {
				t.Fatalf("bundleLogFile status = %q, want %q; message: %s", entry.Status, tc.wantStatus, entry.Message)
			}
			if tc.wantMsg != "" && !strings.Contains(entry.Message, tc.wantMsg) {
				t.Errorf("message = %q; want it to contain %q", entry.Message, tc.wantMsg)
			}
			if tc.wantStatus != bundleStatusOK {
				return
			}
			data, ok := readTarEntries(t, tw, &buf)["okdctl.log"]
			if !ok {
				t.Fatal("okdctl.log not found in tar")
			}
			if string(data) != tc.content {
				t.Errorf("bundled log content = %q, want %q", string(data), tc.content)
			}
		})
	}
}

func TestBundledLogScrubsInstallerCredentials(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		secret string
		keep   string
	}{
		{
			name:   "kubeadmin console password at install-complete",
			line:   `level=info msg="Login to the console with user: \"kubeadmin\", and password: \"AbCdE-FgHiJ-KlMnO-PqRsT\""`,
			secret: "AbCdE-FgHiJ-KlMnO-PqRsT",
			keep:   "kubeadmin",
		},
		{
			name:   "token key-value pair",
			line:   `level=debug msg="registry auth" token=sha256~sUpErSeCrEtToKeNvAlUe`,
			secret: "sha256~sUpErSeCrEtToKeNvAlUe",
			keep:   "registry auth",
		},
		{
			name:   "bearer jwt",
			line:   `level=debug msg="Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJrdWJlYWRtaW4ifQ.c2lnbmF0dXJl"`,
			secret: "eyJhbGciOiJIUzI1NiJ9",
			keep:   "Authorization",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "okdctl.log")
			logF, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			w := install.NewMilestoneWriter(logF, func(install.Milestone) {})
			stream := "level=info msg=\"Waiting for the cluster to initialize...\"\n" + tt.line + "\n"
			if _, err := io.WriteString(w, stream); err != nil {
				t.Fatalf("write stream: %v", err)
			}
			if err := logF.Close(); err != nil {
				t.Fatal(err)
			}

			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			entry := bundleLogFile(stubAddFile(tw), logPath, "", nil)
			if entry.Status != "ok" {
				t.Fatalf("bundleLogFile status = %q; message: %s", entry.Status, entry.Message)
			}
			bundled := string(readTarEntries(t, tw, &buf)["okdctl.log"])
			if strings.Contains(bundled, tt.secret) {
				t.Errorf("credential %q leaked into bundled log:\n%s", tt.secret, bundled)
			}
			if !strings.Contains(bundled, logutil.Redacted) {
				t.Errorf("expected %q sentinel in bundled log:\n%s", logutil.Redacted, bundled)
			}
			if !strings.Contains(bundled, tt.keep) {
				t.Errorf("non-secret context %q missing from bundled log:\n%s", tt.keep, bundled)
			}
			if !strings.Contains(bundled, "Waiting for the cluster to initialize") {
				t.Errorf("ordinary progress line missing from bundled log:\n%s", bundled)
			}
		})
	}
}

// unavailableOptions makes every section skip so Write exercises only tarball-level behavior.
func unavailableOptions(outPath string) Options {
	return Options{
		OutPath:        outPath,
		LoadConfig:     func() (*config.Config, error) { return nil, errors.New("no config in test") },
		ProjectRoot:    func() (string, error) { return "", errors.New("no project root in test") },
		SkipMustGather: true,
	}
}

func TestWriteRefusesSymlinkOutPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.tgz")
	if err := os.WriteFile(target, []byte("pre-existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "bundle.tgz")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	err := Write(context.Background(), unavailableOptions(link))
	if err == nil {
		t.Fatal("Write accepted a symlink output path")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error does not name the symlink refusal: %v", err)
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "pre-existing" {
		t.Errorf("symlink target was modified: %q", string(data))
	}
}

func TestWriteBundleFileMode0600(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "bundle.tgz")
	if err := Write(context.Background(), unavailableOptions(outPath)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("bundle file mode = %o, want 0600", got)
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("bundle is not valid gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	found := false
	for {
		hdr, nextErr := tr.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			t.Fatalf("tar.Next: %v", nextErr)
		}
		if hdr.Name == "manifest.yaml" {
			found = true
		}
	}
	if !found {
		t.Error("manifest.yaml missing from bundle")
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
	_, err := tarDirInto(context.Background(), stubAddStream(tw), srcDir, "test/")
	_ = tw.Close()

	if err != nil {
		// os.Root blocked the escape — acceptable outcome.
		return
	}
	// filepath.WalkDir marks directory symlinks non-regular, so tarDirInto
	// skips them before os.Root fires.
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

func TestTarDirIntoTruncatesOversizedFile(t *testing.T) {
	// Override the production cap so the test doesn't allocate tens of MB of zeros.
	orig := maxBundleFileBytes
	maxBundleFileBytes = 1024
	t.Cleanup(func() { maxBundleFileBytes = orig })

	srcDir := t.TempDir()
	bigPath := filepath.Join(srcDir, "big.log")

	bigSize := maxBundleFileBytes + 64
	f, err := os.Create(bigPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(bigSize); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	truncated, tarErr := tarDirInto(context.Background(), stubAddStream(tw), srcDir, "mg/")
	if cerr := tw.Close(); cerr != nil {
		t.Fatalf("tw.Close: %v", cerr)
	}
	if tarErr != nil {
		t.Fatalf("tarDirInto: %v", tarErr)
	}
	if len(truncated) != 1 || truncated[0] != "big.log" {
		t.Errorf("truncated = %v, want [big.log]", truncated)
	}

	tr := tar.NewReader(bytes.NewReader(buf.Bytes()))
	hdr, nextErr := tr.Next()
	if nextErr != nil {
		t.Fatalf("tar.Next: %v", nextErr)
	}
	if hdr.Size != maxBundleFileBytes {
		t.Errorf("tar entry Size = %d, want %d", hdr.Size, maxBundleFileBytes)
	}
	content, readErr := io.ReadAll(tr)
	if readErr != nil {
		t.Fatalf("io.ReadAll: %v", readErr)
	}
	if int64(len(content)) != maxBundleFileBytes {
		t.Errorf("tar entry content length = %d, want %d", len(content), maxBundleFileBytes)
	}
}

func TestTarDirIntoCancelledContext(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.log"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_, err := tarDirInto(ctx, stubAddStream(tw), srcDir, "mg/")
	_ = tw.Close()

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

type redactableErr struct{ raw string }

func (e redactableErr) Error() string { return e.raw }
func (e redactableErr) Redacted() any { return "[redacted-in-test]" }

func TestSafeMessageRedacted(t *testing.T) {
	plain := errors.New("plain error")
	if got := safeMessage(plain); got != "plain error" {
		t.Errorf("plain error: got %q, want %q", got, "plain error")
	}

	re := redactableErr{raw: "secret-token-xyz"}
	if got := safeMessage(re); got != "[redacted-in-test]" {
		t.Errorf("redactable error: got %q, want %q", got, "[redacted-in-test]")
	}

	if got := safeMessage(nil); got != "" {
		t.Errorf("nil error: got %q, want empty string", got)
	}
}

func TestCollectSectionsSkipMustGather(t *testing.T) {
	add := func(string, []byte) error { return nil }
	addStream := func(*tar.Header, io.Reader) error { return nil }
	// cfgErr/prErr non-nil so bundleConfig/bundleTerraformState skip without dereferencing nil cfg.
	secs := collectSections(
		context.Background(),
		add,
		addStream,
		nil,
		errors.New("no config"),
		"",
		errors.New("no project root"),
		time.Now(),
		"test-bundle-id",
		"",
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
	if !strings.Contains(mg.Message, "--skip-must-gather") {
		t.Errorf("Message = %q, want it to reference --skip-must-gather", mg.Message)
	}
}
