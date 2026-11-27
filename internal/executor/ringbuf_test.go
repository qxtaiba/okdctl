package executor

import (
	"strings"
	"testing"
)

func TestRing_Tail(t *testing.T) {
	cases := []struct {
		name string
		push []string
		want string
	}{
		{"empty", nil, ""},
		{"exactly cap", []string{"x", "y", "z"}, "x\ny\nz"},
		{"over cap keeps the newest", []string{"1", "2", "3", "4", "5"}, "3\n4\n5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRing(3)
			for _, line := range tc.push {
				r.push(line)
			}
			if got := r.tail(); got != tc.want {
				t.Errorf("tail = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLineWriter_SplitsAcrossWrites(t *testing.T) {
	var got []string
	w := &lineWriter{emit: func(s string) { got = append(got, s) }}
	for _, chunk := range []string{"line1\nli", "ne2\npar", "tial"} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"line1", "line2"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("emitted before flush = %v, want %v", got, want)
	}
	w.flush()
	if want := "line1,line2,partial"; strings.Join(got, ",") != want {
		t.Errorf("emitted after flush = %v, want %s", got, want)
	}
}

func TestLineWriter_PartialBoundedAt64K(t *testing.T) {
	var emitted int
	w := &lineWriter{emit: func(s string) { emitted += len(s) }}
	blob := strings.Repeat("x", 1024*1024) // 1 MiB, no newline
	if _, err := w.Write([]byte(blob)); err != nil {
		t.Fatal(err)
	}
	if len(w.partial) > maxPartial {
		t.Errorf("partial len %d exceeds maxPartial %d", len(w.partial), maxPartial)
	}
	if emitted+len(w.partial) != len(blob) {
		t.Errorf("retained %d bytes of %d", emitted+len(w.partial), len(blob))
	}
}
