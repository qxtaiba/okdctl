package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/releases"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestValidateChannel_InvalidIsUsageError(t *testing.T) {
	err := validateChannel("nightly")
	if err == nil {
		t.Fatal("expected error for invalid channel")
	}
	var ue *errtypes.UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("expected *errtypes.UsageError, got %T: %v", err, err)
	}
}

func TestValidateChannel_ValidReturnsNil(t *testing.T) {
	for _, ch := range []string{channelStable, channelAll} {
		if err := validateChannel(ch); err != nil {
			t.Errorf("validateChannel(%q) = %v, want nil", ch, err)
		}
	}
}

func TestValidateFormat_InvalidIsUsageError(t *testing.T) {
	err := validateFormat("yaml")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	var ue *errtypes.UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("expected *errtypes.UsageError, got %T: %v", err, err)
	}
}

func TestValidateFormat_ValidReturnsNil(t *testing.T) {
	for _, f := range []string{outputText, outputJSON} {
		if err := validateFormat(f); err != nil {
			t.Errorf("validateFormat(%q) = %v, want nil", f, err)
		}
	}
}

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

func TestWriteJSON_EmptySliceEncodesAsArray(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, []releases.OKDVersion{}); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Fatalf("empty slice should encode as []; got %q", got)
	}
}
