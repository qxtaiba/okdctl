package logview

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// TestLogRowsLeadWithTheLevelGutter pins the one-char severity column left
// of the stamp: severity reads down a fixed column, not out of the text.
func TestLogRowsLeadWithTheLevelGutter(t *testing.T) {
	at := logBase
	lines := []Line{
		{At: at, Level: "INFO", Text: "deploy step started"},
		{At: at, Level: "warn", Text: "etcd member slow"},
		{At: at, Level: "ERROR", Text: "bootstrap wait failed"},
	}

	rows := renderRows(lines, 70, 3, false)
	want := []string{tui.IconLevelInfo, "W", "E"}
	for i, row := range rows {
		plain := tuitest.StripANSI(row)
		if !strings.HasPrefix(plain, want[i]+" "+at.Format(stampFormat)) {
			t.Errorf("row %d = %q, want gutter %q then the stamp", i, plain, want[i])
		}
	}
}

// TestLogWrappedRowsIndentPastTheGutter keeps a continuation row's text
// under the message column rather than under the gutter.
func TestLogWrappedRowsIndentPastTheGutter(t *testing.T) {
	long := strings.Repeat("terraform refused the clone request ", 4)
	rows := renderRows([]Line{{At: logBase, Level: "ERROR", Text: long}}, 46, 8, true)
	if len(rows) < 2 {
		t.Fatalf("a long line rendered %d rows, want it wrapped", len(rows))
	}
	indent := gutterWidth + lipgloss.Width(stampFormat) + 1
	if plain := tuitest.StripANSI(rows[1]); !strings.HasPrefix(plain, strings.Repeat(" ", indent)) {
		t.Errorf("continuation row = %q, want %d columns of indent", plain, indent)
	}
}

// ringWithErrorAt seeds n info lines with one error line at index at.
func ringWithErrorAt(n, at int) *Ring {
	r := NewRing(DefaultCap)
	for i := range n {
		line := Line{At: logBase.Add(time.Duration(i) * time.Second), Level: "INFO", Text: fmt.Sprintf("step-%02d", i)}
		if i == at {
			line.Level, line.Text = "ERROR", "terraform apply failed"
		}
		r.Append(line)
	}
	return r
}

// TestLogMinimapMarksAnOffScreenErrorPosition is the lane's whole point: an
// error 60 lines back stays visible as a scroll target while the window
// still follows the tail.
func TestLogMinimapMarksAnOffScreenErrorPosition(t *testing.T) {
	const width, height = 60, 10
	out := tuitest.StripANSI(renderPane(ringWithErrorAt(60, 6), view{}, width, height, false))

	rows := strings.Split(out, "\n")
	var marked []int
	for i, row := range rows {
		if strings.HasSuffix(row, tui.IconBar) {
			marked = append(marked, i)
		}
	}
	if len(marked) != 1 {
		t.Fatalf("lane carries %d error marks, want exactly 1:\n%s", len(marked), out)
	}
	// The error sits at ring index 6 of 60 and the nine content rows follow
	// the header, so its bucket is the third rendered row.
	if marked[0] != 2 {
		t.Errorf("error mark on row %d, want row 2 for ring index 6 of 60:\n%s", marked[0], out)
	}
	if strings.Contains(out, "terraform apply failed") {
		t.Fatal("fixture is wrong: the error must be off the following window")
	}
}

// TestLogMinimapMarksTheVisibleWindowApart keeps the lane orienting: the
// cells the window covers read differently from the track around them.
func TestLogMinimapMarksTheVisibleWindowApart(t *testing.T) {
	out := tuitest.StripANSI(renderPane(seededRing(60), view{}, 60, 10, false))
	rows := strings.Split(out, "\n")

	var thumb, track int
	for _, row := range rows[1:] {
		switch {
		case strings.HasSuffix(row, tui.IconBarTick):
			thumb++
		case strings.HasSuffix(row, tui.IconBarSegment):
			track++
		}
	}
	if thumb == 0 || track == 0 {
		t.Errorf("lane must show a window thumb against a track, got %d thumb / %d track:\n%s", thumb, track, out)
	}
}

// TestLogMinimapYieldsTheColumnWhenNothingIsOffScreen keeps the lane out of
// a window already showing the whole stream: there is no target to scroll to.
func TestLogMinimapYieldsTheColumnWhenNothingIsOffScreen(t *testing.T) {
	out := tuitest.StripANSI(renderPane(ringWithErrorAt(6, 1), view{}, 60, 10, false))
	for _, row := range strings.Split(out, "\n") {
		if strings.HasSuffix(row, tui.IconBar) || strings.HasSuffix(row, tui.IconBarSegment) {
			t.Errorf("a fully-visible stream still drew a lane cell: %q", row)
		}
	}
}

// TestLogMinimapYieldsTheColumnOnANarrowWindow keeps the lane from eating
// message text where there is none to spare.
func TestLogMinimapYieldsTheColumnOnANarrowWindow(t *testing.T) {
	out := tuitest.StripANSI(renderPane(ringWithErrorAt(60, 6), view{}, minimapMinWidth-1, 8, false))
	for _, row := range strings.Split(out, "\n") {
		if strings.HasSuffix(row, tui.IconBar) || strings.HasSuffix(row, tui.IconBarSegment) {
			t.Errorf("narrow window still drew a lane cell: %q", row)
		}
	}
}

// TestLogMinimapHonorsSparseFilterSpan proves the thumb spans the window's
// real raw-index reach, not its matched-line count: three matches scattered
// across 1000 raw lines (at 10, 500, and 990) must still read as a window
// reaching nearly the whole stream back, not one hugging the tail — the
// bug computed the span from len(w) (3), which only holds when matches are
// contiguous.
func TestLogMinimapHonorsSparseFilterSpan(t *testing.T) {
	r := NewRing(2000)
	for i := range 1000 {
		text := fmt.Sprintf("step-%03d", i)
		if i == 10 || i == 500 || i == 990 {
			text = "needle " + text
		}
		r.Append(Line{At: logBase.Add(time.Duration(i) * time.Second), Level: "INFO", Text: text})
	}

	out := tuitest.StripANSI(renderPane(r, view{filter: filter{text: "needle"}}, 60, 10, false))
	rows := strings.Split(out, "\n")[1:] // drop the header

	if len(rows) != 3 {
		t.Fatalf("setup: rendered %d rows, want the 3 matches:\n%s", len(rows), out)
	}
	// The oldest match sits at raw index 10 of 1000; a window that honestly
	// reports its reach marks the lane's oldest bucket thumb too, not just
	// the one nearest the tail.
	if !strings.HasSuffix(rows[0], tui.IconBarTick) {
		t.Errorf("oldest lane bucket = %q, want the window thumb (the window reaches back to raw index 10 of 1000)", rows[0])
	}
}

// TestLogPaneRowsFitTheWidthWithTheLane pins the width idiom with the lane
// attached: every row stays within the pane's columns, never one over.
func TestLogPaneRowsFitTheWidthWithTheLane(t *testing.T) {
	const width, height = 52, 9
	out := renderPane(ringWithErrorAt(80, 3), view{}, width, height, false)
	for i, row := range strings.Split(tuitest.StripANSI(out), "\n") {
		if w := lipgloss.Width(row); w > width {
			t.Errorf("row %d is %d cols, want <= %d: %q", i, w, width, row)
		}
	}
}

// TestLogTailCarriesTheGutterAndLane pins the narrow tier: the rows riding
// under a step's own body get the same instrument the pane does.
func TestLogTailCarriesTheGutterAndLane(t *testing.T) {
	rows := renderTail(ringWithErrorAt(60, 6), view{}, 60, NarrowTailRows)
	if len(rows) < 2 {
		t.Fatalf("tail rendered %d rows, want a header plus content", len(rows))
	}
	joined := tuitest.StripANSI(strings.Join(rows[1:], "\n"))
	if !strings.Contains(joined, tui.IconLevelInfo+" ") {
		t.Errorf("tail rows carry no level gutter:\n%s", joined)
	}
	if !strings.Contains(joined, tui.IconBar) {
		t.Errorf("tail lane must mark the off-screen error:\n%s", joined)
	}
}
