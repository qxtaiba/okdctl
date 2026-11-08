package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/releases"
)

func TestPrintVersionListTable(t *testing.T) {
	versions := []releases.OKDVersion{
		{
			Version:     "4.21.3",
			Tag:         "4.21.3",
			Stable:      true,
			Type:        releases.ReleaseTypeStable,
			ReleaseDate: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	if err := printVersionList(&buf, versions); err != nil {
		t.Fatalf("printVersionList: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"VERSION", "RELEASED", "STABLE", "TYPE", "4.21.3", "2026-01-02", "yes", "stable"} {
		if !strings.Contains(out, want) {
			t.Errorf("printVersionList output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintVersionListEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := printVersionList(&buf, nil); err != nil {
		t.Fatalf("printVersionList: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "no releases") || !strings.Contains(got, "--channel all") {
		t.Errorf("printVersionList(nil) = %q, want to contain %q and %q", got, "no releases", "--channel all")
	}
}
