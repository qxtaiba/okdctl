package logutil

import (
	"context"
	"log/slog"
	"testing"
)

type capturedRecord struct {
	level slog.Level
	msg   string
}

type captureHandler struct {
	records *[]capturedRecord
}

func (h captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h captureHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires value receiver
	*h.records = append(*h.records, capturedRecord{level: r.Level, msg: r.Message})
	return nil
}
func (h captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h captureHandler) WithGroup(string) slog.Handler      { return h }

func TestDedupWarner(t *testing.T) {
	var records []capturedRecord
	d := NewDedupWarner(slog.New(captureHandler{records: &records}))

	d.Warn("err-a", "check failed", "err", "err-a")
	d.Warn("err-a", "check failed", "err", "err-a")
	d.Warn("err-b", "check failed", "err", "err-b")
	d.Warn("err-b", "check failed", "err", "err-b")
	d.Reset()
	d.Warn("err-b", "check failed", "err", "err-b")

	want := []capturedRecord{
		{slog.LevelWarn, "check failed"},
		{slog.LevelDebug, "check failed (repeated)"},
		{slog.LevelWarn, "check failed"},
		{slog.LevelDebug, "check failed (repeated)"},
		{slog.LevelWarn, "check failed"},
	}
	if len(records) != len(want) {
		t.Fatalf("got %d records, want %d: %+v", len(records), len(want), records)
	}
	for i := range want {
		if records[i] != want[i] {
			t.Errorf("record %d = %+v, want %+v", i, records[i], want[i])
		}
	}
}

func TestDedupWarnerNilLoggerDoesNotPanic(*testing.T) {
	d := NewDedupWarner(nil)
	d.Warn("k", "msg")
	d.Reset()
}
