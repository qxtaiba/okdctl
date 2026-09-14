package tuitest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStripANSI_RemovesSGRAndOSC(t *testing.T) {
	in := "\x1b[1;38;2;147;51;234mhi\x1b[m \x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\"
	if got := StripANSI(in); got != "hi link" {
		t.Fatalf("StripANSI = %q", got)
	}
}

func TestAssertFits_Table(t *testing.T) {
	cases := []struct {
		frame string
		w, h  int
		ok    bool
	}{
		{"ab\ncd", 2, 2, true},
		{"abc\ncd", 2, 2, false},
		{"ab\ncd\nef", 2, 2, false},
		{"ab\ncd\nef", 2, 0, true},
		{"abc\ncd\nef", 2, 0, false},
	}
	for _, c := range cases {
		if got := fits(c.frame, c.w, c.h) == nil; got != c.ok {
			t.Errorf("fits(%q,%d,%d) ok=%v want %v", c.frame, c.w, c.h, got, c.ok)
		}
	}
}

func TestGolden_UpdateWritesFile(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("OKDCTL_UPDATE_GOLDEN", "1")
	Golden(t, "sample", "\x1b[1mframe\x1b[m")
	b, err := os.ReadFile(filepath.Join("testdata", t.Name(), "sample.golden"))
	if err != nil || string(b) != "frame" {
		t.Fatalf("golden not written: %v %q", err, b)
	}
}
