package executor

import (
	"fmt"
	"strings"
	"testing"
)

func TestRingWriter_Empty(t *testing.T) {
	r := newRingWriter(5)
	if got := r.tail(); got != "" {
		t.Errorf("empty tail want \"\", got %q", got)
	}
}

func TestRingWriter_ExactlyCap(t *testing.T) {
	r := newRingWriter(3)
	for _, line := range []string{"x", "y", "z"} {
		if _, err := fmt.Fprintln(r, line); err != nil {
			t.Fatal(err)
		}
	}
	got := r.tail()
	if got != "x\ny\nz" {
		t.Errorf("want \"x\\ny\\nz\", got %q", got)
	}
}

func TestRingWriter_OverCap(t *testing.T) {
	r := newRingWriter(3)
	for _, line := range []string{"1", "2", "3", "4", "5"} {
		if _, err := fmt.Fprintln(r, line); err != nil {
			t.Fatal(err)
		}
	}
	got := r.tail()
	if got != "3\n4\n5" {
		t.Errorf("want \"3\\n4\\n5\", got %q", got)
	}
}

func TestRingWriter_MultiLineWrite(t *testing.T) {
	r := newRingWriter(10)
	if _, err := r.Write([]byte("line1\nline2\nline3\n")); err != nil {
		t.Fatal(err)
	}
	got := r.tail()
	if got != "line1\nline2\nline3" {
		t.Errorf("want \"line1\\nline2\\nline3\", got %q", got)
	}
}

func TestRingWriter_PartialLineWrite(t *testing.T) {
	r := newRingWriter(10)
	if _, err := r.Write([]byte("no newline")); err != nil {
		t.Fatal(err)
	}
	got := r.tail()
	if got != "no newline" {
		t.Errorf("want \"no newline\", got %q", got)
	}
}

func TestRingWriter_PartialThenComplete(t *testing.T) {
	r := newRingWriter(10)
	if _, err := r.Write([]byte("par")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Write([]byte("tial\nfull\n")); err != nil {
		t.Fatal(err)
	}
	got := r.tail()
	if got != "partial\nfull" {
		t.Errorf("want \"partial\\nfull\", got %q", got)
	}
}

func TestRingWriter_PartialBoundedAt64K(t *testing.T) {
	r := newRingWriter(maxCapturedLines)
	blob := strings.Repeat("x", 1024*1024) // 1 MiB, no newline
	if _, err := r.Write([]byte(blob)); err != nil {
		t.Fatal(err)
	}
	if len(r.partial) > maxPartial {
		t.Errorf("partial len %d exceeds maxPartial %d", len(r.partial), maxPartial)
	}
	var total int
	for _, line := range r.lines {
		total += len(line)
	}
	total += len(r.partial)
	limit := maxPartial * (maxCapturedLines + 1)
	if total > limit {
		t.Errorf("total retained bytes %d exceeds limit %d", total, limit)
	}
}
