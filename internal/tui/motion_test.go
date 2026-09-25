package tui

import "testing"

func TestResolveMotion(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}

	for _, tc := range []struct {
		name    string
		flagOff bool
		vars    map[string]string
		want    MotionMode
	}{
		{"default_full", false, nil, MotionFull},
		{"flag_off", true, nil, MotionOff},
		{"env_off_1", false, map[string]string{"OKDCTL_NO_MOTION": "1"}, MotionOff},
		{"env_off_true", false, map[string]string{"OKDCTL_NO_MOTION": "true"}, MotionOff},
		{"env_off_other_value_ignored", false, map[string]string{"OKDCTL_NO_MOTION": "yes"}, MotionFull},
		{"no_color_implies_reduced", false, map[string]string{"NO_COLOR": "1"}, MotionReduced},
		{"flag_beats_no_color", true, map[string]string{"NO_COLOR": "1"}, MotionOff},
		{"env_beats_no_color", false, map[string]string{"OKDCTL_NO_MOTION": "1", "NO_COLOR": "1"}, MotionOff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveMotion(tc.flagOff, env(tc.vars)); got != tc.want {
				t.Errorf("ResolveMotion(%v, %v) = %v, want %v", tc.flagOff, tc.vars, got, tc.want)
			}
		})
	}
}

func TestSpinnerGlyphIsPureInFrame(t *testing.T) {
	if got := SpinnerGlyph(MotionFull, 0); got != "⣾" {
		t.Errorf("full frame 0 = %q, want the first glyph", got)
	}
	if got := SpinnerGlyph(MotionFull, 5); got != "⣟" {
		t.Errorf("full frame 5 = %q, want the sixth glyph", got)
	}
	if got := SpinnerGlyph(MotionFull, 8); got != "⣾" {
		t.Errorf("full frame 8 = %q, want wrap-around to the first glyph", got)
	}
	if got := SpinnerGlyph(MotionReduced, 7); got != "⣽" {
		t.Errorf("reduced frame 7 = %q, want the second glyph (quarter cadence)", got)
	}
	for frame := uint64(0); frame < 20; frame++ {
		if got := SpinnerGlyph(MotionOff, frame); got != "⣾" {
			t.Fatalf("off frame %d = %q, want the frozen first glyph", frame, got)
		}
	}
}

func TestSetMotionRoundTrips(t *testing.T) {
	t.Cleanup(func() { SetMotion(MotionFull) })
	SetMotion(MotionReduced)
	if Motion() != MotionReduced {
		t.Errorf("Motion() = %v after SetMotion(MotionReduced)", Motion())
	}
}
