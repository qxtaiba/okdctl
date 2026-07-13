package install

import (
	"bytes"
	"io"
	"regexp"
)

// MilestoneKind classifies a recognized openshift-install progress line. The
// stream firehose routes to the log file by default; a small set of milestone
// lines are promoted to the operator-facing TTY.
type MilestoneKind int

const (
	// MilestoneBootstrapComplete marks "It is now safe to remove the
	// bootstrap resources" — the control plane has taken over from bootstrap.
	MilestoneBootstrapComplete MilestoneKind = iota + 1
	// MilestoneInstallComplete marks "Install complete!".
	MilestoneInstallComplete
	// MilestoneOperatorDegraded marks a "Cluster operator <name> Degraded is
	// True" line; Operator carries the operator name.
	MilestoneOperatorDegraded
)

// Milestone is a parsed openshift-install milestone. Operator is populated
// only for MilestoneOperatorDegraded.
type Milestone struct {
	Kind     MilestoneKind
	Operator string
}

// Matched against the log-level-prefixed logrus output openshift-install
// emits under `wait-for ... --log-level=debug`. Kept as substring matches so
// the timestamp/level prefix (which differs between text and debug formats)
// does not need to be modelled.
var (
	reBootstrapComplete = regexp.MustCompile(`It is now safe to remove the bootstrap resources`)
	reInstallComplete   = regexp.MustCompile(`Install complete!`)
	reOperatorDegraded  = regexp.MustCompile(`[Cc]luster operator (\S+) Degraded is True`)
)

// ParseMilestone reports whether line is a recognized milestone. The two
// completion milestones win over a degraded match on the same line (they never
// co-occur in practice, but the ordering keeps the result deterministic).
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

// milestoneScanMax caps the in-flight line buffer so a stream that never emits
// a newline (carriage-return progress bars) cannot grow it without bound.
const milestoneScanMax = 64 * 1024

// milestoneWriter tees every byte to dst (the persistent log sink) unbuffered,
// while accumulating complete lines to scan for milestones. Each recognized
// milestone is handed to notify, which the caller wires to the TTY log. notify
// may be called from the os/exec copy goroutine, so it must be safe for
// concurrent use when the same notify backs both a stdout and a stderr writer.
type milestoneWriter struct {
	dst    io.Writer
	notify func(Milestone)
	buf    []byte
}

// NewMilestoneWriter wraps dst so streamed subprocess output is persisted
// verbatim to dst while openshift-install milestones are surfaced via notify.
func NewMilestoneWriter(dst io.Writer, notify func(Milestone)) io.Writer {
	return &milestoneWriter{dst: dst, notify: notify}
}

func (w *milestoneWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	// Scan only what dst accepted so a short write does not double-count.
	w.buf = append(w.buf, p[:n]...)
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
	}
	if len(w.buf) > milestoneScanMax {
		w.buf = w.buf[len(w.buf)-milestoneScanMax:]
	}
	return n, err
}
