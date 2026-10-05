package platform

import (
	"errors"
	"strings"
	"testing"
)

func TestParseOSRelease_AcceptsRHELFamily(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    OS
	}{
		{"fedora by id", "NAME=\"Fedora Linux\"\nID=fedora\nVERSION_ID=41\n", OS{ID: "fedora", Version: "41"}},
		{"rocky by id", "ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\nVERSION_ID=\"9.4\"\n", OS{ID: "rocky", Version: "9.4"}},
		{"derivative by id_like", "# comment\nID=ol\nID_LIKE=\"fedora\"\nVERSION_ID=\"9.3\"\n", OS{ID: "ol", Version: "9.3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseOSRelease(tc.content)
			if err != nil {
				t.Fatalf("parseOSRelease: %v", err)
			}
			if got != tc.want {
				t.Errorf("parseOSRelease = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseOSRelease_RefusesOtherFamilies(t *testing.T) {
	cases := []struct {
		name    string
		content string
		id      string
	}{
		{"ubuntu", "ID=ubuntu\nID_LIKE=debian\nVERSION_ID=\"24.04\"\nVERSION_CODENAME=noble\n", "ubuntu"},
		{"debian", "ID=debian\nVERSION_ID=\"12\"\n", "debian"},
		{"debian derivative", "ID=linuxmint\nID_LIKE=\"ubuntu debian\"\n", "linuxmint"},
		{"suse", "ID=\"opensuse-leap\"\nID_LIKE=\"suse opensuse\"\n", "opensuse-leap"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOSRelease(tc.content)
			if !errors.Is(err, ErrUnsupportedOS) {
				t.Fatalf("parseOSRelease err = %v, want ErrUnsupportedOS", err)
			}
			for _, want := range []string{tc.id, "rhel-family bastions only"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err, want)
				}
			}
		})
	}
}

func TestParseOSRelease_MissingIDIsNotAnUnsupportedVerdict(t *testing.T) {
	_, err := parseOSRelease("NAME=mystery\n")
	if err == nil {
		t.Fatal("parseOSRelease accepted an os-release without ID")
	}
	if errors.Is(err, ErrUnsupportedOS) {
		t.Errorf("an unidentifiable host must not read as a positive unsupported verdict: %v", err)
	}
}
