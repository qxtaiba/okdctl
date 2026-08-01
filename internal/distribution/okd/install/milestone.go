package install

import (
	"bytes"
	"io"
	"regexp"

	"github.com/qxtaiba/okdctl/internal/logutil"
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

// milestoneScanMax caps the in-flight line buffer: a stream that never emits
// a newline (carriage-return progress bars) is flushed to dst in scrubbed
// chunks of this size instead of growing the buffer without bound.
const milestoneScanMax = 64 * 1024

// milestoneWriter line-buffers the stream, scans each complete line for
// milestones, and persists it to dst only after logutil.ScrubSecrets has
// masked credential-shaped values. dst is the workspace okdctl.log that
// debug-bundle archives into the operator-shared tarball, and
// openshift-install prints the kubeadmin console password on the
// install-complete line — this writer is the last point where that line can
// be scrubbed before it becomes shareable. A trailing partial line stays
// buffered until its newline arrives (openshift-install's logrus output is
// newline-terminated, so at most one incomplete line is lost at process
// exit). Each recognized milestone is handed to notify, which the caller
// wires to the TTY log. notify may be called from the os/exec copy
// goroutine, so it must be safe for concurrent use when the same notify
// backs both a stdout and a stderr writer.
type milestoneWriter struct {
	dst    io.Writer
	notify func(Milestone)
	buf    []byte
}

// NewMilestoneWriter wraps dst so streamed subprocess output is persisted to
// dst with credential-shaped values scrubbed, while openshift-install
// milestones are surfaced via notify.
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
		// A secret straddling this chunk boundary could evade the scrub;
		// accepted — it requires a newline-less stream past 64 KiB, which
		// openshift-install's line-oriented logging never produces.
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
