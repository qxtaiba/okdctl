package logview

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// logBase is the fixed clock every seeded log fixture stamps from.
var logBase = time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)

// seededRing fills a real ring with n deterministic lines, so pane tests
// read the same fixture the production type would hand them.
func seededRing(n int) *Ring {
	r := NewRing(DefaultCap)
	for i := range n {
		r.Append(Line{
			At:    logBase.Add(time.Duration(i*7) * time.Second),
			Level: "INFO",
			Text:  fmt.Sprintf("deploy step started step=step-%02d phase=setup", i),
		})
	}
	return r
}

func TestLogPaneFollowsTheTailWithinItsHeight(t *testing.T) {
	// Wide enough that a row's step id survives truncation once the level
	// gutter and the minimap lane have taken their columns.
	const width, height = 47, 8
	out := tailFrame(seededRing(40), view{}, width, height)
	lines := strings.Split(tuitest.StripANSI(out), "\n")

	if len(lines) > height {
		t.Fatalf("pane rendered %d rows, want <= %d", len(lines), height)
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("row %d is %d cols, want <= %d: %q", i, w, width, l)
		}
	}
	if !strings.HasPrefix(lines[0], "LOG") {
		t.Errorf("pane must lead with its section header, got %q", lines[0])
	}
	if !strings.Contains(out, "step-39") {
		t.Errorf("a following pane must show the newest line:\n%s", tuitest.StripANSI(out))
	}
	if strings.Contains(out, "step-00") {
		t.Errorf("a following pane must have scrolled past the oldest line:\n%s", tuitest.StripANSI(out))
	}
	newest := logBase.Add(39 * 7 * time.Second).Format(stampFormat)
	if !strings.Contains(tuitest.StripANSI(out), newest) {
		t.Errorf("rows must carry their own timestamp; %q missing from:\n%s", newest, tuitest.StripANSI(out))
	}
}

func TestLogPaneEmptyRingSaysSoInsteadOfRenderingBlank(t *testing.T) {
	out := tuitest.StripANSI(renderFull(NewRing(8), view{}, 40, 6))
	if !strings.Contains(out, "waiting for the first log line") {
		t.Errorf("an empty ring must say so:\n%s", out)
	}
}

func TestLogWindowLockHoldsItsPointWhileTheTailMovesOn(t *testing.T) {
	r := seededRing(20)
	v := view{locked: true, lockAt: lockedAt(r)}

	for i := 20; i < 40; i++ {
		r.Append(Line{At: logBase, Text: fmt.Sprintf("later step-%02d", i)})
	}

	lines, first := r.Snapshot()
	st := v.filter.selectFrom(lines, first)
	w, _, _ := windowIn(&st, v, 4)
	if len(w) != 4 {
		t.Fatalf("locked window holds %d rows, want 4", len(w))
	}
	if !strings.Contains(w[3].Text, "step-19") {
		t.Errorf("locked window ends at %q, want the line the lock pinned", w[3].Text)
	}

	followSt := filter{}.selectFrom(lines, first)
	following, _, _ := windowIn(&followSt, view{}, 4)
	if !strings.Contains(following[3].Text, "step-39") {
		t.Errorf("a released window ends at %q, want the newest line", following[3].Text)
	}
}

// TestLogWindowLockOlderThanTheRingFallsBackToWhatIsLeft keeps a long-held lock
// from blanking the pane once the ring has evicted its point.
func TestLogWindowLockOlderThanTheRingFallsBackToWhatIsLeft(t *testing.T) {
	r := NewRing(4)
	for i := range 20 {
		r.Append(Line{At: logBase, Text: fmt.Sprintf("step-%02d", i)})
	}
	lines, first := r.Snapshot()

	st := filter{}.selectFrom(lines, first)
	w, _, _ := windowIn(&st, view{locked: true, lockAt: 1}, 3)
	if len(w) == 0 {
		t.Fatal("a lock the ring has outrun must still show what it holds")
	}
}

// tailFrame renders the tail window as one frame of at most height rows.
func tailFrame(src Source, v view, width, height int) string {
	return strings.Join(renderTail(src, v, width, height-1), "\n")
}

// TestLogRowsCarryTextualLevelTags pins bug 24's minimal fix: warn and
// error lines carry their level as text, so the failure screen's evidence
// tail reads under NO_COLOR instead of riding on tint alone.
func TestLogRowsCarryTextualLevelTags(t *testing.T) {
	at := logBase
	lines := []Line{
		{At: at, Level: "INFO", Text: "deploy step started"},
		{At: at, Level: "warn", Text: "etcd member slow"},
		{At: at, Level: "ERROR", Text: "bootstrap wait failed"},
	}

	rows := renderRows(lines, 70, 3, false)

	if plain := tuitest.StripANSI(rows[1]); !strings.Contains(plain, "WARN") {
		t.Errorf("warn row carries no textual tag: %q", plain)
	}
	if plain := tuitest.StripANSI(rows[2]); !strings.Contains(plain, "ERROR") {
		t.Errorf("error row carries no textual tag: %q", plain)
	}
	if plain := tuitest.StripANSI(rows[0]); strings.Contains(plain, "INFO") {
		t.Errorf("info row must stay untagged to keep the stream quiet: %q", plain)
	}
}

func TestLogRowsStyleWarnAndErrorApart(t *testing.T) {
	at := logBase
	lines := []Line{
		{At: at, Level: "INFO", Text: "deploy step started"},
		{At: at, Level: "WARN", Text: "deploy step started"},
		{At: at, Level: "ERROR", Text: "deploy step started"},
	}

	rows := renderRows(lines, 60, 3, false)
	if len(rows) != 3 {
		t.Fatalf("renderRows returned %d rows, want 3", len(rows))
	}
	if rows[0] == rows[1] || rows[1] == rows[2] || rows[0] == rows[2] {
		t.Error("warn and error lines must be styled apart from info so a failure screen reads")
	}
	for i, row := range rows {
		if !strings.Contains(tuitest.StripANSI(row), "deploy step started") {
			t.Errorf("row %d lost its text: %q", i, tuitest.StripANSI(row))
		}
	}
}

func TestGolden_LogSurfaceTheme(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Cleanup(func() {
		tui.SetColorProfileFor(&bytes.Buffer{})
		tui.SetDarkBackground(true)
	})
	for _, tc := range []struct {
		name string
		dark bool
		ansi string
	}{
		{name: "dark", dark: true, ansi: "38;2;6;182;212"},
		{name: "light", ansi: "38;2;14;116;144"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tui.UseTheme(tui.ResolveTheme(colorprofile.TrueColor, tc.dark, tui.ThemeDefault))
			frame := tailFrame(seededRing(8), view{}, 60, 5)
			if !strings.Contains(frame, tc.ansi) {
				t.Errorf("log header = %q, want Accent %s", frame, tc.ansi)
			}
			tuitest.Golden(t, "log_"+tc.name, frame)
		})
	}
}

func TestLogSurfaceHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Cleanup(func() {
		tui.SetColorProfileFor(&bytes.Buffer{})
		tui.SetDarkBackground(true)
	})
	t.Setenv("NO_COLOR", "1")
	tui.SetColorProfileFor(io.Discard)
	frame := tailFrame(seededRing(8), view{}, 60, 5)
	frame = tui.Downsample(frame)
	if strings.Contains(frame, "\x1b[") {
		t.Fatalf("log surface emitted ANSI with NO_COLOR: %q", frame)
	}
	tuitest.Golden(t, "log_no_color", frame)
}

// TestScrollLogFollowsThePagerContract pins the pager contract on the view
// state itself: paging up from follow engages the lock, further pages hold
// honest window ends, the floor stops at the oldest full window, and paging
// back past the tail releases the lock and resumes following.
func TestScrollLogFollowsThePagerContract(t *testing.T) {
	r := seededRing(40)
	v := view{}

	scroll(&v, r, -10, 10)
	if !v.locked {
		t.Fatal("paging up from follow must engage the lock")
	}
	if v.lockAt != 30 {
		t.Errorf("lockAt = %d after one page up from 40, want 30", v.lockAt)
	}

	scroll(&v, r, -100, 10)
	if v.lockAt != 10 {
		t.Errorf("lockAt = %d at the floor, want the oldest full window end 10", v.lockAt)
	}

	scroll(&v, r, 10, 10)
	if v.lockAt != 20 {
		t.Errorf("lockAt = %d after one page down from the floor, want 20", v.lockAt)
	}

	scroll(&v, r, 100, 10)
	if v.locked {
		t.Error("paging back to the tail must release the lock and resume following")
	}
}

// TestScrollLogWholeRingIsReachable walks a ring top to bottom: every line
// the ring holds must appear in some window along the way.
func TestScrollLogWholeRingIsReachable(t *testing.T) {
	const n, budget = 60, 8
	r := seededRing(n)
	v := view{}

	seen := map[string]bool{}
	record := func() {
		lines, first := r.Snapshot()
		st := v.filter.selectFrom(lines, first)
		w, _, _ := windowIn(&st, v, budget)
		for i := range w {
			seen[w[i].Text] = true
		}
	}
	record()
	for range n {
		scroll(&v, r, -budget, budget)
		record()
	}
	for i := range n {
		if !seen[fmt.Sprintf("deploy step started step=step-%02d phase=setup", i)] {
			t.Fatalf("line %d never appeared in any window while paging to the top", i)
		}
	}
}

// TestLogPaneHeaderNamesTheLockedWindowPosition pins the lock indicator's
// honest coordinates: the shown line span out of the stream's total.
func TestLogPaneHeaderNamesTheLockedWindowPosition(t *testing.T) {
	r := seededRing(40)
	v := view{locked: true, lockAt: 30}

	out := tuitest.StripANSI(tailFrame(r, v, 60, 6))
	if !strings.Contains(out, "LOG · 26–30 of 40") {
		t.Errorf("locked pane header must name its window position, got:\n%s", out)
	}

	if got := tuitest.StripANSI(paneHeader(view{}, coords{shown: 5, end: 40, total: 40, pos: 40, matches: 40}, 60)); got != "LOG" {
		t.Errorf("a following pane keeps the bare label, got %q", got)
	}
}

// TestRenderLogPaneWrapKeepsWholeLinesAndHonestCoordinates pins the
// full-screen wrap contract: long lines wrap (tui.WrapLines, space-only)
// instead of …-clipping, and the locked header names only lines actually on
// screen after wrapping trimmed the window.
func TestRenderLogPaneWrapKeepsWholeLinesAndHonestCoordinates(t *testing.T) {
	r := NewRing(16)
	for i := range 8 {
		text := fmt.Sprintf("short line %02d", i)
		if i%2 == 1 {
			text = fmt.Sprintf("terraform apply failed on attempt %02d: proxmox task UPID:pve:0000ABCD refused the clone request because the target volume is out of space", i)
		}
		r.Append(Line{At: logBase, Level: "INFO", Text: text})
	}

	out := tuitest.StripANSI(renderFull(r, view{locked: true, lockAt: 8}, 60, 8))
	if strings.Contains(out, "…") {
		t.Errorf("full-screen mode must wrap, never …-clip:\n%s", out)
	}
	if !strings.Contains(out, "out of space") {
		t.Errorf("the long line's tail must be readable:\n%s", out)
	}
	rows := strings.Split(out, "\n")
	if len(rows) > 8 {
		t.Fatalf("rendered %d rows, want <= 8", len(rows))
	}
	if !strings.Contains(rows[0], "of 8") {
		t.Errorf("header must carry the stream total:\n%s", rows[0])
	}
}
