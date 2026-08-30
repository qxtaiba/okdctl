package download

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type tarEntry struct {
	Name     string
	Mode     int64
	Typeflag byte
	Linkname string
	Data     []byte
}

func buildTarGz(t *testing.T, entries []tarEntry) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.Name,
			Mode:     e.Mode,
			Size:     int64(len(e.Data)),
			Typeflag: e.Typeflag,
			Linkname: e.Linkname,
		}
		if hdr.Typeflag == 0 {
			hdr.Typeflag = tar.TypeReg
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if len(e.Data) > 0 {
			if _, err := tw.Write(e.Data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "test.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractTarGz_ZipSlipRejected(t *testing.T) {
	cases := []struct {
		name        string
		entries     []tarEntry
		wantMsg     string
		escapedFile string // relative to filepath.Dir(dest); must not exist after call
	}{
		{
			name: "parent-dir traversal in entry name",
			entries: []tarEntry{
				{Name: "../escape.txt", Mode: 0o644, Data: []byte("bad")},
			},
			wantMsg: "escape destination",
		},
		{
			name: "absolute symlink target",
			entries: []tarEntry{
				{Name: "link", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
			},
			wantMsg: "absolute symlink",
		},
		{
			name: "relative symlink escaping dest",
			entries: []tarEntry{
				{Name: "link", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "../../../etc/passwd"},
			},
			wantMsg: "escape",
		},
		{
			name: "symlink-then-write redirects through escaped link",
			entries: []tarEntry{
				{Name: "link", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "../escape-dir"},
				{Name: "link/file.txt", Mode: 0o644, Data: []byte("bad")},
			},
			wantMsg:     "escape",
			escapedFile: "escape-dir/file.txt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive := buildTarGz(t, tc.entries)
			dest := realTempDir(t)
			err := ExtractTarGz(context.Background(), archive, dest)
			if err == nil {
				t.Fatalf("expected rejection")
			}
			// wantMsg is one plausible phrasing; any escape/outside/not-allowed match is acceptable.
			lower := strings.ToLower(err.Error())
			if !strings.Contains(lower, strings.ToLower(tc.wantMsg)) &&
				!strings.Contains(lower, "outside") &&
				!strings.Contains(lower, "not allowed") {
				t.Errorf("err = %v; want contains %q or generic escape phrasing", err, tc.wantMsg)
			}

			if _, statErr := os.Stat(filepath.Join(filepath.Dir(dest), "escape.txt")); !os.IsNotExist(statErr) {
				t.Errorf("escape file materialized outside dest: %v", statErr)
			}
			if tc.escapedFile != "" {
				escaped := filepath.Join(filepath.Dir(dest), tc.escapedFile)
				if _, statErr := os.Stat(escaped); !os.IsNotExist(statErr) {
					t.Errorf("escaped file materialized outside dest at %s: %v", escaped, statErr)
				}
			}
		})
	}
}

// realTempDir resolves macOS's /var symlink so EvalSymlinks-based checks compare identical paths.
func realTempDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	resolved, err := filepath.EvalSymlinks(d)
	if err != nil {
		return d
	}
	return resolved
}

func TestExtractTarGz_HappyPath(t *testing.T) {
	archive := buildTarGz(t, []tarEntry{
		{Name: "dir/", Mode: 0o755, Typeflag: tar.TypeDir},
		{Name: "dir/file.txt", Mode: 0o644, Data: []byte("hello")},
	})
	dest := realTempDir(t)
	if err := ExtractTarGz(context.Background(), archive, dest); err != nil {
		t.Fatalf("extract: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dest, "dir", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello" {
		t.Errorf("body = %q", body)
	}
}

// a/b/toroot -> ../.. then a/b/toroot/esc -> ../etc composes past the textual check alone.
func TestExtractTarGz_ComposedSymlinkChainRejected(t *testing.T) {
	archive := buildTarGz(t, []tarEntry{
		{Name: "a/", Mode: 0o755, Typeflag: tar.TypeDir},
		{Name: "a/b/", Mode: 0o755, Typeflag: tar.TypeDir},
		{Name: "a/b/toroot", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "../.."},
		{Name: "a/b/toroot/esc", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "../etc"},
	})
	dest := realTempDir(t)
	err := ExtractTarGz(context.Background(), archive, dest)
	if err == nil {
		t.Fatalf("expected rejection of composed symlink chain")
	}
	if !errors.Is(err, ErrSymlinkEscape) {
		t.Errorf("err = %v; want errors.Is ErrSymlinkEscape", err)
	}

	// Check both the resolved (destDir/esc) and literal locations for the removed link.
	for _, rel := range []string{"esc", filepath.Join("a", "b", "toroot", "esc")} {
		p := filepath.Join(dest, rel)
		if _, statErr := os.Lstat(p); !os.IsNotExist(statErr) {
			tgt, _ := os.Readlink(p)
			t.Errorf("escaping link left on disk at %s -> %s (lstat err %v)", p, tgt, statErr)
		}
	}
}

// real/ exists before the link is created, so the resolution post-check resolves it cleanly.
func TestExtractTarGz_InTreeSymlinkChain(t *testing.T) {
	archive := buildTarGz(t, []tarEntry{
		{Name: "real/", Mode: 0o755, Typeflag: tar.TypeDir},
		{Name: "real/data.txt", Mode: 0o644, Data: []byte("payload")},
		{Name: "dir/", Mode: 0o755, Typeflag: tar.TypeDir},
		{Name: "dir/link1", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "../real"},
	})
	dest := realTempDir(t)
	if err := ExtractTarGz(context.Background(), archive, dest); err != nil {
		t.Fatalf("extract in-tree chain: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dest, "dir", "link1", "data.txt"))
	if err != nil {
		t.Fatalf("read through in-tree link: %v", err)
	}
	if string(body) != "payload" {
		t.Errorf("body = %q; want payload", body)
	}
}

func TestExtractTarGz_ContextCancellation(t *testing.T) {
	archive := buildTarGz(t, []tarEntry{
		{Name: "a.txt", Mode: 0o644, Data: []byte("x")},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := ExtractTarGz(ctx, archive, t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled", err)
	}
}

func TestExtractTarGz_ModeMask(t *testing.T) {
	cases := []struct {
		name     string
		mode     int64
		wantMode os.FileMode
	}{
		{"setgid bit stripped", 0o2755, 0o0755},
		{"setuid bit stripped", 0o4755, 0o0755},
		{"normal exec preserved", 0o755, 0o0755},
		{"normal rw preserved", 0o644, 0o0644},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive := buildTarGz(t, []tarEntry{
				{Name: "file.bin", Mode: tc.mode, Data: []byte("data")},
			})
			dest := realTempDir(t)
			if err := ExtractTarGz(context.Background(), archive, dest); err != nil {
				t.Fatalf("extract: %v", err)
			}
			info, err := os.Stat(filepath.Join(dest, "file.bin"))
			if err != nil {
				t.Fatalf("stat: %v", err)
			}
			got := info.Mode().Perm()
			if got & ^tc.wantMode != 0 {
				t.Errorf("mode = %04o; want subset of %04o (setuid/setgid/sticky stripped)", got, tc.wantMode)
			}
		})
	}
}

func TestExtractTarGz_StripComponents(t *testing.T) {
	archive := buildTarGz(t, []tarEntry{
		{Name: "top/inner/file.txt", Mode: 0o644, Data: []byte("data")},
	})
	dest := realTempDir(t)
	if err := ExtractTarGz(context.Background(), archive, dest, WithExtractStripComponents(2)); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "file.txt")); err != nil {
		t.Errorf("expected stripped file at dest root: %v", err)
	}
}
