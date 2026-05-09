package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCompletion_WritesToCmdOut(t *testing.T) {
	cases := []struct {
		shell  string
		prefix string
	}{
		{"bash", "# bash completion"},
		{"zsh", "#compdef"},
		{"fish", "# fish completion"},
	}
	for _, tc := range cases {
		t.Run(tc.shell, func(t *testing.T) {
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			t.Cleanup(func() { rootCmd.SetOut(nil) })

			err := runCompletion(completionCmd, []string{tc.shell})
			if err != nil {
				t.Fatalf("runCompletion(%q) returned error: %v", tc.shell, err)
			}
			got := strings.ToLower(buf.String())
			if !strings.HasPrefix(got, strings.ToLower(tc.prefix)) {
				t.Fatalf("shell %q: output starts with %q, want prefix %q",
					tc.shell, buf.String()[:min(len(buf.String()), 40)], tc.prefix)
			}
		})
	}
}

func TestRunCompletion_UnknownShellErrors(t *testing.T) {
	err := runCompletion(completionCmd, []string{"powershell"})
	if err == nil {
		t.Fatal("expected error for unknown shell, got nil")
	}
}
