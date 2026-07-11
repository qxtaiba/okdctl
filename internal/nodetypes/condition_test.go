package nodetypes

import "testing"

// TestConditionStatusLiterals pins ConditionStatusTrue/False to the exact
// case-sensitive strings Kubernetes uses for status.conditions[*].status.
// cli/status.go and postinstall/verify.go compare oc-reported condition
// values against these literals, so a rename here would silently break
// Ready/Degraded detection.
func TestConditionStatusLiterals(t *testing.T) {
	if got := string(ConditionStatusTrue); got != "True" {
		t.Fatalf("ConditionStatusTrue = %q, want %q", got, "True")
	}
	if got := string(ConditionStatusFalse); got != "False" {
		t.Fatalf("ConditionStatusFalse = %q, want %q", got, "False")
	}
}
