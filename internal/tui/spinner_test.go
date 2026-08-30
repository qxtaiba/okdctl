package tui

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

const clearSeq = "\r\x1b[2K"

// fakeOwner records clear calls without painting, keeping buffer contents
// deterministic.
type fakeOwner struct {
	w      io.Writer
	clears int
}

func (f *fakeOwner) clearLine() {
	f.clears++
	_, _ = io.WriteString(f.w, clearSeq)
}

func configureBuf(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	if err := ConfigureLoggers("debug", "text", buf, buf, false); err != nil {
		t.Fatal(err)
	}
	logutil.InstallHandler(newStderrHandler())
}

func TestHandler_ClearsOwnerLineOncePerRecord(t *testing.T) {
	var buf bytes.Buffer
	configureBuf(t, &buf)

	fake := &fakeOwner{w: &buf}
	lineReg.register(fake)
	t.Cleanup(func() { lineReg.release(fake) })

	const n = 5
	for range n {
		logutil.Info("phase tick")
	}

	if fake.clears != n {
		t.Fatalf("clears = %d, want %d (one per record)", fake.clears, n)
	}
	out := buf.String()
	if got := strings.Count(out, clearSeq); got != n {
		t.Fatalf("clear sequences = %d, want %d", got, n)
	}
	if !strings.HasPrefix(out, clearSeq) {
		t.Fatalf("output does not open with a clear; record landed mid-line:\n%q", out)
	}
	for _, seg := range strings.SplitAfter(out, clearSeq)[1:] {
		if strings.Contains(seg, clearSeq) {
			continue
		}
		if !strings.Contains(seg, "phase tick") {
			t.Fatalf("segment after clear has no record:\n%q", seg)
		}
	}
}

// Runs under synctest so sleeps ride the fake clock instead of real-time delay.
func TestSpinner_LogDuringSpinnerStartsAtColumnZero(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var buf bytes.Buffer
		configureBuf(t, &buf)

		stop := startSpinner(context.Background(), "installing", &buf)
		for range 3 {
			logutil.Info("csr pending")
			time.Sleep(150 * time.Millisecond)
		}
		stop()

		out := buf.String()
		if !strings.Contains(out, clearSeq) {
			t.Fatalf("no clear sequence emitted during spinner:\n%q", out)
		}
		if !strings.Contains(out, "csr pending") {
			t.Fatalf("log record lost:\n%q", out)
		}
		if !strings.HasSuffix(out, clearSeq) {
			t.Fatalf("teardown did not leave a cleared line; suffix = %q", tail(out))
		}
	})
}

// Guarded by a timeout so a deadlock fails fast instead of stalling.
func TestSpinner_NoDeadlockUnderConcurrentLogs(t *testing.T) {
	var buf bytes.Buffer
	configureBuf(t, &buf)

	stop := startSpinner(context.Background(), "converging", &buf)

	const goroutines, perG = 16, 40
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for range perG {
				logutil.Info("worker log", logutil.LF("g", id))
			}
		}(g)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout: concurrent logging deadlocked with spinner")
	}
	stop()

	if got := strings.Count(buf.String(), "worker log"); got != goroutines*perG {
		t.Fatalf("record count = %d, want %d", got, goroutines*perG)
	}
}

// Covers both stop paths: explicit stop() and ctx cancellation.
func TestSpinner_TeardownClearsAndDeregisters(t *testing.T) {
	var buf bytes.Buffer
	stop := startSpinner(context.Background(), "waiting", &buf)
	if !lineReg.hasOwner() {
		t.Fatal("spinner did not register as line owner")
	}
	stop()
	if lineReg.hasOwner() {
		t.Fatal("owner still active after stop")
	}
	if !strings.HasSuffix(buf.String(), clearSeq) {
		t.Fatalf("stop did not clear the line; suffix = %q", tail(buf.String()))
	}

	ctx, cancel := context.WithCancel(context.Background())
	var buf2 bytes.Buffer
	stop2 := startSpinner(ctx, "cancelme", &buf2)
	cancel()
	stop2() // OnceFunc + <-done must return even though ctx already fired
	if lineReg.hasOwner() {
		t.Fatal("owner still active after ctx-cancel teardown")
	}
}

func TestStartSpinner_NonTTYNoOp(t *testing.T) {
	prev := logutil.ProgressBarsEnabled()
	logutil.SetProgressBarsEnabled(false)
	t.Cleanup(func() { logutil.SetProgressBarsEnabled(prev) })

	stop := StartSpinner(context.Background(), "quiet")
	if lineReg.hasOwner() {
		t.Fatal("non-TTY StartSpinner registered a line owner")
	}
	stop() // must not panic or block
}

// Runs under synctest so the ticker's repaint frames pass on the fake clock.
func TestStatusLine_SetUpdatesDesc(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var buf bytes.Buffer
		set, stop := startStatusLine(context.Background(), "waiting for cluster operators", &buf)
		if !lineReg.hasOwner() {
			t.Fatal("status line did not register as line owner")
		}
		set("cluster operators 12/33 available · 4 CSRs approved")
		// Give the ticker a couple of frames to repaint with the new desc.
		time.Sleep(300 * time.Millisecond)
		stop()

		out := buf.String()
		if !strings.Contains(out, "12/33 available") {
			t.Fatalf("painted line missing updated detail:\n%q", out)
		}
		if lineReg.hasOwner() {
			t.Fatal("owner still active after stop")
		}
	})
}

func TestStartStatusLine_NonTTYNoOp(t *testing.T) {
	prev := logutil.ProgressBarsEnabled()
	logutil.SetProgressBarsEnabled(false)
	t.Cleanup(func() { logutil.SetProgressBarsEnabled(prev) })

	set, stop := StartStatusLine(context.Background(), "quiet")
	if lineReg.hasOwner() {
		t.Fatal("non-TTY StartStatusLine registered a line owner")
	}
	set("detail") // must not panic
	stop()        // must not panic or block
}

func TestLineRegistry_RegisterClearsOutgoingOwner(t *testing.T) {
	var reg lineRegistry
	var buf bytes.Buffer
	a := &fakeOwner{w: &buf}
	b := &fakeOwner{w: &buf}

	reg.register(a)
	if a.clears != 0 {
		t.Fatalf("register cleared before any takeover: clears=%d", a.clears)
	}
	reg.register(b)
	if a.clears != 1 {
		t.Fatalf("takeover did not clear the outgoing owner: a.clears=%d", a.clears)
	}
	if b.clears != 0 {
		t.Fatalf("takeover cleared the incoming owner: b.clears=%d", b.clears)
	}
	// Re-registering the same owner must not self-clear.
	reg.register(b)
	if b.clears != 0 {
		t.Fatalf("re-register of the same owner cleared it: b.clears=%d", b.clears)
	}
}

func TestSpinner_PaintErasesToEndOfLine(t *testing.T) {
	var buf bytes.Buffer
	sp := &spinner{w: &buf, desc: "a considerably longer description", start: time.Now()}
	lineReg.register(sp)
	t.Cleanup(func() { lineReg.release(sp) })

	sp.paint()
	if !strings.HasPrefix(buf.String(), clearSeq) {
		t.Fatalf("paint did not erase to end of line; got %q", buf.String())
	}

	buf.Reset()
	sp.setDesc("short")
	sp.paint()
	if !strings.HasPrefix(buf.String(), clearSeq) {
		t.Fatalf("shorter repaint did not erase the longer prior line; got %q", buf.String())
	}
}

func tail(s string) string {
	if len(s) > 24 {
		return s[len(s)-24:]
	}
	return s
}
