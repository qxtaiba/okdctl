package cli

import "testing"

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
