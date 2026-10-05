package tui

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestColumnTableAlignsRight(t *testing.T) {
	lines := ColumnTable(
		[]Column{{Header: "NAME"}, {Header: "CPU", Align: AlignRight}},
		[]RowGroup{{Rows: [][]string{{"pve1", "32"}, {"long-node-name", "8"}}}},
		TableOptions{PlainHeader: true},
	)
	want := []string{
		"NAME            CPU",
		"pve1             32",
		"long-node-name    8",
	}
	for i, w := range want {
		if got := tuitest.StripANSI(lines[i]); got != w {
			t.Errorf("line %d = %q, want %q", i, got, w)
		}
	}
}

func TestColumnTableMinAndMaxWidth(t *testing.T) {
	lines := ColumnTable(
		[]Column{{Header: "A", MinWidth: 6}, {Header: "B", MaxWidth: 4}},
		[]RowGroup{{Rows: [][]string{{"x", "overlong"}}}},
		TableOptions{PlainHeader: true},
	)
	if got := tuitest.StripANSI(lines[1]); got != "x       o…ng" {
		t.Errorf("row = %q, want min-floored col A and middle-truncated col B", got)
	}
}

func TestColumnTableWeightDistributesSurplus(t *testing.T) {
	lines := ColumnTable(
		[]Column{{Header: "A", Weight: 1}, {Header: "B"}},
		[]RowGroup{{Rows: [][]string{{"aa", "bb"}}}},
		TableOptions{PlainHeader: true, Width: 20},
	)
	// natural: A=2, B=2, gap 2 → 6 columns; the 14-column surplus goes to
	// the sole weighted column, so the full line reaches exactly Width.
	if got := lipgloss.Width(lines[0]); got != 20 {
		t.Errorf("header width = %d, want the weighted column stretched to Width 20", got)
	}
	if got := tuitest.StripANSI(lines[1]); got != "aa                bb" {
		t.Errorf("row = %q, want the surplus absorbed by column A", got)
	}
}

func TestColumnTableRendersGroupHeadersAndContinuousRowStyleIndex(t *testing.T) {
	var styled []int
	ColumnTable(
		[]Column{{Header: "NODE"}, {Header: "READY"}},
		[]RowGroup{
			{Title: "masters (2)", Rows: [][]string{{"m1", "yes"}, {"m2", "yes"}}},
			{Title: "workers (1)", Rows: [][]string{{"w1", "no"}}},
		},
		TableOptions{RowStyle: func(row int) (lipgloss.Style, bool) {
			styled = append(styled, row)
			return lipgloss.Style{}, false
		}},
	)
	if len(styled) != 3 || styled[2] != 2 {
		t.Fatalf("RowStyle indices = %v, want continuous 0..2 across groups", styled)
	}

	lines := ColumnTable(
		[]Column{{Header: "NODE"}, {Header: "READY"}},
		[]RowGroup{
			{Title: "masters (1)", Rows: [][]string{{"m1", "yes"}}},
			{Title: "workers (1)", Rows: [][]string{{"w1", "no"}}},
		},
		TableOptions{PlainHeader: true},
	)
	want := []string{"NODE  READY", "masters (1)", "m1    yes  ", "workers (1)", "w1    no   "}
	if len(lines) != len(want) {
		t.Fatalf("len(lines) = %d, want %d:\n%v", len(lines), len(want), lines)
	}
	for i, w := range want {
		if got := tuitest.StripANSI(lines[i]); got != w {
			t.Errorf("line %d = %q, want %q", i, got, w)
		}
	}
}

func TestTableStaysAThinWrapperOverColumnTable(t *testing.T) {
	viaTable := Table([]string{"A", "B"}, [][]string{{"x", "y"}}, TableOptions{PlainHeader: true})
	viaColumns := ColumnTable(
		[]Column{{Header: "A"}, {Header: "B"}},
		[]RowGroup{{Rows: [][]string{{"x", "y"}}}},
		TableOptions{PlainHeader: true},
	)
	if len(viaTable) != len(viaColumns) {
		t.Fatalf("line counts differ: %d vs %d", len(viaTable), len(viaColumns))
	}
	for i := range viaTable {
		if viaTable[i] != viaColumns[i] {
			t.Errorf("line %d differs: %q vs %q", i, viaTable[i], viaColumns[i])
		}
	}
}
