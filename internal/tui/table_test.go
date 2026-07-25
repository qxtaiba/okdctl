package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestTableAlignsColumnsToWidestCell(t *testing.T) {
	lines := Table(
		[]string{"NAME", "ROLE"},
		[][]string{{"m0", "master"}, {"a-very-long-name", "worker"}},
		TableOptions{PlainHeader: true},
	)
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3", len(lines))
	}
	w := lipgloss.Width(lines[0])
	for i, l := range lines {
		if lipgloss.Width(l) != w {
			t.Errorf("row %d width %d != header width %d", i, lipgloss.Width(l), w)
		}
	}
	if !strings.HasPrefix(lines[0], "NAME") {
		t.Errorf("header should start with NAME: %q", lines[0])
	}
}

func TestTableMiddleTruncatesOverLongCells(t *testing.T) {
	lines := Table(
		[]string{"ADDRESS"},
		[][]string{{"module.okd_cluster.proxmox_virtual_environment_vm.master[0]"}},
		TableOptions{PlainHeader: true, MaxColWidth: 20},
	)
	row := lines[1]
	if lipgloss.Width(row) > 20 {
		t.Errorf("truncated cell width %d exceeds cap 20: %q", lipgloss.Width(row), row)
	}
	if !strings.Contains(row, "…") {
		t.Errorf("over-long cell should carry an ellipsis: %q", row)
	}
	if !strings.HasSuffix(strings.TrimRight(row, " "), "master[0]") {
		t.Errorf("middle-truncation should keep the distinguishing tail: %q", row)
	}
}

func TestTableRowStylePaintsSelectedRow(t *testing.T) {
	lines := Table(
		[]string{"NAME", "READY"},
		[][]string{{"ok", "yes"}, {"bad", "no"}},
		TableOptions{
			PlainHeader: true,
			RowStyle: func(i int) (lipgloss.Style, bool) {
				if i == 1 {
					return ErrorStyle, true
				}
				return lipgloss.Style{}, false
			},
		},
	)
	if strings.Contains(lines[1], "\x1b[") {
		t.Errorf("unstyled row should carry no ANSI: %q", lines[1])
	}
	if !strings.Contains(lines[2], "\x1b[") {
		t.Errorf("styled row should carry ANSI: %q", lines[2])
	}
}
