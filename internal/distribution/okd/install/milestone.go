package install

import (
	"bytes"
	"io"
	"regexp"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// MilestoneKind classifies a recognized openshift-install progress line
// promoted to the operator-facing TTY.
type MilestoneKind int

const (
	// MilestoneBootstrapComplete marks openshift-install's bootstrap-complete line.
	MilestoneBootstrapComplete MilestoneKind = iota + 1
	// MilestoneInstallComplete marks "Install complete!".
	MilestoneInstallComplete
	// MilestoneOperatorDegraded marks a cluster-operator-degraded line, with
	// Operator carrying the operator name.
	MilestoneOperatorDegraded
)

// Milestone is a parsed openshift-install milestone. Operator is populated
// only for MilestoneOperatorDegraded.
type Milestone struct {
	Kind     MilestoneKind
	Operator string
}

// Substring-matched against logrus debug output; no need to model the
// timestamp/level prefix.
var (
	reBootstrapComplete = regexp.MustCompile(`It is now safe to remove the bootstrap resources`)
	reInstallComplete   = regexp.MustCompile(`Install complete!`)
	reOperatorDegraded  = regexp.MustCompile(`[Cc]luster operator (\S+) Degraded is True`)
)

// ParseMilestone reports whether line is a recognized milestone; completion
// milestones win over a degraded match on the same line.
func ParseMilestone(line string) (Milestone, bool) {
	switch {
	case reBootstrapComplete.MatchString(line):
		return Milestone{Kind: MilestoneBootstrapComplete}, true
	case reInstallComplete.MatchString(line):
		return Milestone{Kind: MilestoneInstallComplete}, true
	case reOperatorDegraded.MatchString(line):
		m := reOperatorDegraded.FindStringSubmatch(line)
		return Milestone{Kind: MilestoneOperatorDegraded, Operator: m[1]}, true
	}
	return Milestone{}, false
}

// milestoneScanMax caps the in-flight line buffer; a newline-less stream is
// flushed in chunks of this size instead of growing unbounded.
const milestoneScanMax = 64 * 1024

// milestoneWriter scrubs secrets via logutil.ScrubSecrets before writing dst
// (okdctl.log is archived). notify may run concurrently with the exec copy
// goroutine and must tolerate that.
type milestoneWriter struct {
	dst    io.Writer
	notify func(Milestone)
	buf    []byte
}

// NewMilestoneWriter wraps dst, scrubbing streamed output and reporting
// milestones via notify.
func NewMilestoneWriter(dst io.Writer, notify func(Milestone)) io.Writer {
	return &milestoneWriter{dst: dst, notify: notify}
}

func (w *milestoneWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	var out []byte
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		if m, ok := ParseMilestone(line); ok {
			w.notify(m)
		}
		out = append(out, logutil.ScrubSecrets(line)...)
		out = append(out, '\n')
	}
	if len(w.buf) > milestoneScanMax {
		// A secret straddling this boundary could evade scrubbing; accepted
		// since output is line-oriented.
		out = append(out, logutil.ScrubSecrets(string(w.buf))...)
		w.buf = w.buf[:0]
	}
	if len(out) > 0 {
		if _, err := w.dst.Write(out); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}
