package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

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

func TestWriteJSON_EmptySliceEncodesAsArray(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, []releases.OKDVersion{}); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Fatalf("empty slice should encode as []; got %q", got)
	}
}
