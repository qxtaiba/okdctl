package download

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// progressWriter wraps dst, counting bytes written and rendering a simple
// carriage-return progress bar to stderr on each Write. Caller must call
// Close to emit the final newline. When tui.ProgressBarsEnabled() is false
// or total is unknown (<=0), renders nothing and acts as a transparent
// pass-through.
type progressWriter struct {
	dst     io.Writer
	total   int64
	written int64
	desc    string
	width   int
}

func newProgressWriter(dst io.Writer, total int64, desc string) io.WriteCloser {
	if !tui.ProgressBarsEnabled() || total <= 0 {
		return nopCloser{dst}
	}
	w, _, err := term.GetSize(int(os.Stderr.Fd())) //nolint:gosec // G115: Fd() fits int on all supported platforms
	if err != nil || w < 20 {
		w = 72
	}
	return &progressWriter{dst: dst, total: total, desc: desc, width: w}
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.dst.Write(p)
	pw.written += int64(n)
	pw.render()
	return n, err
}

func (pw *progressWriter) Close() error {
	_, _ = fmt.Fprintln(os.Stderr)
	return nil
}

func (pw *progressWriter) render() {
	pct := min(float64(pw.written)/float64(pw.total), 1.0)
	barWidth := max(pw.width-len(pw.desc)-12, 4)
	filled := int(pct * float64(barWidth))
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", barWidth-filled)
	_, _ = fmt.Fprintf(os.Stderr, "\r[%s] %3.0f%%  %s", bar, pct*100, pw.desc)
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
