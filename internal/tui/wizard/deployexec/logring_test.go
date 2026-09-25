package deployexec

import (
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

func TestLogRingKeepsTheNewestLinesWithinItsCap(t *testing.T) {
	r := NewLogRing(3)
	base := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	for i := range 5 {
		r.append(LogLine{At: base, Text: string(rune('a' + i))})
	}

	lines, first := r.Snapshot()
	if len(lines) != 3 {
		t.Fatalf("ring holds %d lines, want its cap of 3", len(lines))
	}
	if got := []string{lines[0].Text, lines[1].Text, lines[2].Text}; got[0] != "c" || got[2] != "e" {
		t.Errorf("ring holds %v, want the newest three (c,d,e)", got)
	}
	if first != 2 {
		t.Errorf("first index = %d, want 2 after two evictions", first)
	}
}

// TestLogRingSnapshotIsACopy proves the render loop can never see a line the
// writers mutate underneath it.
func TestLogRingSnapshotIsACopy(t *testing.T) {
	r := NewLogRing(4)
	r.append(LogLine{Text: "first"})

	lines, _ := r.Snapshot()
	lines[0].Text = "clobbered"

	again, _ := r.Snapshot()
	if again[0].Text != "first" {
		t.Errorf("mutating a snapshot reached the ring: %q", again[0].Text)
	}
}

func TestLogRingHandlerCapturesMessageAndFieldsAndForwards(t *testing.T) {
	var sink strings.Builder
	r := NewLogRing(LogRingCap)
	log := slog.New(r.Handler(slog.NewTextHandler(&sink, nil)))

	log.Info("deploy step started", "step", "deploy-infrastructure", "phase", "install")

	lines, _ := r.Snapshot()
	if len(lines) != 1 {
		t.Fatalf("ring captured %d lines, want 1", len(lines))
	}
	want := "deploy step started step=deploy-infrastructure phase=install"
	if lines[0].Text != want {
		t.Errorf("captured %q, want %q", lines[0].Text, want)
	}
	if lines[0].At.IsZero() {
		t.Error("captured line has no timestamp to render")
	}
	if !strings.Contains(sink.String(), "deploy step started") {
		t.Errorf("the handler behind the ring never saw the record: %q", sink.String())
	}
}

// TestLogRingHandlerCarriesWithAttrs keeps a derived logger's fields from
// disappearing out of the pane.
func TestLogRingHandlerCarriesWithAttrs(t *testing.T) {
	r := NewLogRing(LogRingCap)
	slog.New(r.Handler(nil)).With("run_id", "run-42").Info("bootstrap complete")

	lines, _ := r.Snapshot()
	if len(lines) != 1 || !strings.Contains(lines[0].Text, "run_id=run-42") {
		t.Errorf("captured %+v, want the carried run_id", lines)
	}
}

// TestLogRingRedactsThroughTheFacade proves the pane can only ever show scrubbed
// records: the ring sits inside logutil's redaction wrapper.
func TestLogRingRedactsThroughTheFacade(t *testing.T) {
	r := NewLogRing(LogRingCap)
	restore := logutil.RedirectHandler(r.Handler(nil))
	defer restore()

	logutil.Info("connecting to proxmox", logutil.LF("password", "s3cr3t"))

	lines, _ := r.Snapshot()
	if len(lines) != 1 {
		t.Fatalf("captured %d lines, want 1", len(lines))
	}
	if strings.Contains(lines[0].Text, "s3cr3t") {
		t.Errorf("the pane captured an unredacted secret: %q", lines[0].Text)
	}
}

// TestLogRingConcurrentWritesAndReads is the -race gate on the tee: the engine's
// goroutines write while the render loop snapshots.
func TestLogRingConcurrentWritesAndReads(t *testing.T) {
	r := NewLogRing(32)
	log := slog.New(r.Handler(nil))

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range 200 {
				select {
				case <-stop:
					return
				default:
				}
				log.Info("step", "worker", id, "n", i)
			}
		}(w)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 500 {
			lines, first := r.Snapshot()
			_ = logRows(logWindow(lines, first, logView{}, 8), 40, 8, false)
		}
		close(stop)
	}()

	wg.Wait()
	if lines, _ := r.Snapshot(); len(lines) == 0 {
		t.Error("no lines reached the ring")
	}
}
