package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/releases"
)

func TestWriteJSON_EmptyVersionSliceIsArray(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, []releases.OKDVersion{}); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "[]" {
		t.Fatalf("empty slice should encode as []; got %q", got)
	}
}

func TestWriteJSON_MakeZeroLengthSliceIsArray(t *testing.T) {
	var buf bytes.Buffer
	versions := make([]releases.OKDVersion, 0)
	if err := writeJSON(&buf, versions); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "[]" {
		t.Fatalf("make([]T, 0) should encode as []; got %q", got)
	}
}
