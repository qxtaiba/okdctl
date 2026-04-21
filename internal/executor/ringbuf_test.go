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

func TestRingWriter_UnderCap(t *testing.T) {
	r := newRingWriter(5)
	if _, err := fmt.Fprintln(r, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(r, "b"); err != nil {
		t.Fatal(err)
	}
	got := r.tail()
	if got != "a\nb" {
		t.Errorf("want \"a\\nb\", got %q", got)
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

func TestRingWriter_LargeOverCap(t *testing.T) {
	const maxLines = 200
	r := newRingWriter(maxLines)
	for i := 0; i < 500; i++ {
		if _, err := fmt.Fprintf(r, "line%d\n", i); err != nil {
			t.Fatal(err)
		}
	}
	tail := r.tail()
	lines := strings.Split(tail, "\n")
	if len(lines) != maxLines {
		t.Errorf("want %d lines, got %d", maxLines, len(lines))
	}
	if lines[0] != "line300" {
		t.Errorf("want first retained line \"line300\", got %q", lines[0])
	}
	if lines[maxLines-1] != "line499" {
		t.Errorf("want last retained line \"line499\", got %q", lines[maxLines-1])
	}
}
