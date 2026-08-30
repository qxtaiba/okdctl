package deploy

import (
	"bytes"
	"strings"
	"testing"
)

func TestStreamWriters_NoSinkLeavesDefaults(t *testing.T) {
	so, se := streamWriters(nil, false)
	if so != nil || se != nil {
		t.Fatalf("no sink must leave executor defaults; got %v/%v", so, se)
	}
}

// Proves the firehose reaches the sink verbatim (including milestone lines)
// without also going to a TTY writer.
func TestStreamWriters_DefaultRoutesToSinkOnly(t *testing.T) {
	var sink bytes.Buffer
	_, se := streamWriters(&sink, false)
	if se == nil {
		t.Fatal("expected a stderr writer for an active sink")
	}

	const line = `level=info msg="It is now safe to remove the bootstrap resources"` + "\n"
	if _, err := se.Write([]byte(line)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := sink.String(); got != line {
		t.Fatalf("sink missing verbatim stream:\ngot  %q\nwant %q", got, line)
	}
}

// Proves --verbose keeps the raw stream while still persisting everything to the sink.
func TestStreamWriters_VerboseTeesToStderrAndSink(t *testing.T) {
	var sink bytes.Buffer
	_, se := streamWriters(&sink, true)

	const payload = "level=debug msg=\"reconciling\"\n"
	if _, err := se.Write([]byte(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(sink.String(), "reconciling") {
		t.Fatalf("verbose sink did not receive the stream: %q", sink.String())
	}
}
