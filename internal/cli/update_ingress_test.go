package cli

import (
	"context"
	"strings"
	"testing"
)

func TestBuildConvertConfirm_YesTrue(t *testing.T) {
	if !buildConvertConfirm(context.Background(), true)(nil) {
		t.Error("yes=true must confirm without consulting stdin")
	}
}

func TestBuildConvertConfirm_YesFalse(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"y answer", "y\n", true},
		{"n answer", "n\n", false},
		{"EOF", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := testStdinReader
			testStdinReader = strings.NewReader(tc.input)
			t.Cleanup(func() { testStdinReader = old })

			fn := buildConvertConfirm(context.Background(), false)
			if got := fn([]string{"default"}); got != tc.want {
				t.Errorf("input %q: want %v, got %v", tc.input, tc.want, got)
			}
		})
	}
}
