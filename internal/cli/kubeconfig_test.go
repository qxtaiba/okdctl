package cli

import (
	"reflect"
	"testing"
)

func TestMergeNamedList(t *testing.T) {
	t.Run("nil src returns dest unchanged", func(t *testing.T) {
		dest := []any{map[string]any{"name": "a"}}
		got := mergeNamedList(dest, nil)
		if !reflect.DeepEqual(got, dest) {
			t.Errorf("got %v, want %v", got, dest)
		}
	})

	t.Run("empty src returns dest unchanged", func(t *testing.T) {
		dest := []any{map[string]any{"name": "a"}}
		got := mergeNamedList(dest, []any{})
		if !reflect.DeepEqual(got, dest) {
			t.Errorf("got %v, want %v", got, dest)
		}
	})

	t.Run("src entry with no name collision is appended", func(t *testing.T) {
		dest := []any{map[string]any{"name": "existing"}}
		src := []any{map[string]any{"name": "new"}}
		got, _ := mergeNamedList(dest, src).([]any)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2: %+v", len(got), got)
		}
		if got[0].(map[string]any)["name"] != "existing" {
			t.Errorf("first entry lost")
		}
		if got[1].(map[string]any)["name"] != "new" {
			t.Errorf("new entry not appended")
		}
	})

	t.Run("src entry with same name is NOT appended (no-clobber)", func(t *testing.T) {
		dest := []any{map[string]any{"name": "prod", "cluster": map[string]any{"server": "https://prod.example"}}}
		src := []any{map[string]any{"name": "prod", "cluster": map[string]any{"server": "https://EVIL.example"}}}
		got, _ := mergeNamedList(dest, src).([]any)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1 (src must be dropped)", len(got))
		}
		srv, _ := got[0].(map[string]any)["cluster"].(map[string]any)
		if srv["server"] != "https://prod.example" {
			t.Errorf("existing entry was clobbered: got %v", got[0])
		}
	})

	t.Run("mix: one collides, one does not", func(t *testing.T) {
		dest := []any{map[string]any{"name": "prod"}}
		src := []any{
			map[string]any{"name": "prod"},
			map[string]any{"name": "staging"},
		}
		got, _ := mergeNamedList(dest, src).([]any)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		names := []string{
			got[0].(map[string]any)["name"].(string),
			got[1].(map[string]any)["name"].(string),
		}
		want := []string{"prod", "staging"}
		if !reflect.DeepEqual(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("entries without a name key are skipped silently", func(t *testing.T) {
		dest := []any{}
		src := []any{
			map[string]any{"name": "good"},
			map[string]any{"cluster": "no-name-key"}, // skipped
			"not-a-map",                              // skipped
		}
		got, _ := mergeNamedList(dest, src).([]any)
		if len(got) != 1 {
			t.Errorf("len = %d, want 1 (only named map survives)", len(got))
		}
	})
}
