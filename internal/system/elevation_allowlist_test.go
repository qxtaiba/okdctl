package system

import (
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestIsAllowedChownRoot(t *testing.T) {
	home := "/home/alice"
	tmp := "/tmp"

	cases := []struct {
		name    string
		absPath string
		want    bool
	}{
		{"projectRoot/okd-install allowed", "/srv/cluster/okd-install", true},
		{"projectRoot/infrastructure allowed", "/srv/cluster/infrastructure", true},
		{"/etc refused", "/etc", false},
		{"/usr/local refused", "/usr/local", false},
		{"user home subdir allowed", "/home/alice/.kube", true},
		{"user home exact match allowed", "/home/alice", true},
		{"cross-user home refused", "/home/otheruser/.kube", false},
		{"tmp subdir allowed", "/tmp/okdctl-work", true},
		{"tmp exact match allowed", "/tmp", true},
		{"path sharing home prefix but not child refused", "/home/alicefoo", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isAllowedChownRoot(tc.absPath, home, tmp)
			if got != tc.want {
				t.Errorf("isAllowedChownRoot(%q) = %v; want %v", tc.absPath, got, tc.want)
			}
		})
	}
}

func TestChownTreeToInvokingUser_RefusesDisallowedPath(t *testing.T) {
	setSudoEnv(t, "1000", "1000")

	err := ChownTreeToInvokingUser("/etc")
	if err == nil {
		t.Fatal("expected error for /etc; got nil")
	}
	var ae *errtypes.AuthError
	if !errors.As(err, &ae) {
		t.Fatalf("err is %T; want *errtypes.AuthError", err)
	}
}
