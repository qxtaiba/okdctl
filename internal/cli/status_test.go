package cli

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func TestConditionStatusLiterals(t *testing.T) {
	if got := string(nodetypes.ConditionStatusTrue); got != "True" {
		t.Fatalf("ConditionStatusTrue = %q, want %q", got, "True")
	}
	if got := string(nodetypes.ConditionStatusFalse); got != "False" {
		t.Fatalf("ConditionStatusFalse = %q, want %q", got, "False")
	}
}

func TestValidateFormat_DescribeNode(t *testing.T) {
	cases := []struct {
		format  string
		wantErr bool
	}{
		{"text", false},
		{"json", false},
		{"foo", true},
		{"", true},
		{"JSON", true},
	}
	for _, tc := range cases {
		t.Run("node/"+tc.format, func(t *testing.T) {
			err := validateFormat(tc.format)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateFormat(%q) error = %v, wantErr %v", tc.format, err, tc.wantErr)
			}
		})
	}
}

func TestValidateFormat_DescribeAddon(t *testing.T) {
	cases := []struct {
		format  string
		wantErr bool
	}{
		{"text", false},
		{"json", false},
		{"foo", true},
		{"", true},
		{"TEXT", true},
	}
	for _, tc := range cases {
		t.Run("addon/"+tc.format, func(t *testing.T) {
			err := validateFormat(tc.format)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateFormat(%q) error = %v, wantErr %v", tc.format, err, tc.wantErr)
			}
		})
	}
}
