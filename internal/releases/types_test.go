package releases

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReleaseTypeJSON(t *testing.T) {
	cases := []struct {
		rt    ReleaseType
		label string
	}{
		{ReleaseTypeStable, `"stable"`},
		{ReleaseTypeLatestStable, `"latest-stable"`},
		{ReleaseTypePreview, `"preview"`},
		{ReleaseTypeLatestPreview, `"latest-preview"`},
		{ReleaseTypeLTS, `"lts"`},
	}
	for _, tc := range cases {
		b, err := json.Marshal(tc.rt)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", tc.rt, err)
		}
		if string(b) != tc.label {
			t.Errorf("Marshal(%v) = %s; want %s", tc.rt, b, tc.label)
		}
		var fromLabel ReleaseType
		if err := json.Unmarshal([]byte(tc.label), &fromLabel); err != nil {
			t.Fatalf("Unmarshal(%s): %v", tc.label, err)
		}
		if fromLabel != tc.rt {
			t.Errorf("Unmarshal(%s) = %v; want %v", tc.label, fromLabel, tc.rt)
		}
	}
}

func TestReleaseTypeUnmarshalJSONUnknown(t *testing.T) {
	var rt ReleaseType
	err := json.Unmarshal([]byte(`"lts-preview"`), &rt)
	if err == nil {
		t.Errorf("expected error for unknown release type, got nil; rt=%v", rt)
	}
}

func TestOKDVersionReleaseTypeInJSON(t *testing.T) {
	v := OKDVersion{
		Version: "4.21.3",
		Tag:     "4.21.3-okd-scos.0",
		Stable:  true,
		Latest:  true,
		Type:    ReleaseTypeLatestStable,
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal OKDVersion: %v", err)
	}
	if !strings.Contains(string(b), `"release_type":"latest-stable"`) {
		t.Errorf("JSON missing latest-stable label; got %s", b)
	}
}
