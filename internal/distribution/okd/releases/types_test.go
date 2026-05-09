package releases

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReleaseTypeMarshalJSON(t *testing.T) {
	cases := []struct {
		rt   ReleaseType
		want string
	}{
		{ReleaseTypeStable, `"stable"`},
		{ReleaseTypeLatestStable, `"latest-stable"`},
		{ReleaseTypePreview, `"preview"`},
		{ReleaseTypeLatestPreview, `"latest-preview"`},
		{ReleaseTypeLTS, `"lts"`},
	}
	for _, tc := range cases {
		got, err := json.Marshal(tc.rt)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", tc.rt, err)
		}
		if string(got) != tc.want {
			t.Errorf("Marshal(%v) = %s; want %s", tc.rt, got, tc.want)
		}
	}
}

func TestReleaseTypeUnmarshalJSON(t *testing.T) {
	cases := []struct {
		input string
		want  ReleaseType
	}{
		{`"stable"`, ReleaseTypeStable},
		{`"latest-stable"`, ReleaseTypeLatestStable},
		{`"preview"`, ReleaseTypePreview},
		{`"latest-preview"`, ReleaseTypeLatestPreview},
		{`"lts"`, ReleaseTypeLTS},
	}
	for _, tc := range cases {
		var got ReleaseType
		if err := json.Unmarshal([]byte(tc.input), &got); err != nil {
			t.Fatalf("Unmarshal(%s): %v", tc.input, err)
		}
		if got != tc.want {
			t.Errorf("Unmarshal(%s) = %v; want %v", tc.input, got, tc.want)
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

func TestReleaseTypeRoundTrip(t *testing.T) {
	variants := []ReleaseType{
		ReleaseTypeStable,
		ReleaseTypeLatestStable,
		ReleaseTypePreview,
		ReleaseTypeLatestPreview,
		ReleaseTypeLTS,
	}
	for _, rt := range variants {
		b, err := json.Marshal(rt)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", rt, err)
		}
		var got ReleaseType
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("Unmarshal(%s): %v", b, err)
		}
		if got != rt {
			t.Errorf("round-trip(%v): got %v", rt, got)
		}
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
