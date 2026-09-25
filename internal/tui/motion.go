package tui

import "sync/atomic"

// MotionMode is the three-position animation dial: full motion, reduced
// (calmer cadence, honored when the environment asks for less movement),
// and off (no frame clock at all — animations freeze at their first frame).
type MotionMode int32

// Motion dial positions, most animated first.
const (
	MotionFull MotionMode = iota
	MotionReduced
	MotionOff
)

// ResolveMotion resolves the motion dial from its inputs, most explicit
// first: the --no-motion flag, then OKDCTL_NO_MOTION ("1"/"true"), then
// NO_COLOR (any value, no-color.org) which implies reduced motion only —
// off has its own switch and is deliberately NO_COLOR-independent.
func ResolveMotion(flagOff bool, getenv func(string) string) MotionMode {
	if flagOff {
		return MotionOff
	}
	if v := getenv("OKDCTL_NO_MOTION"); v == "1" || v == "true" {
		return MotionOff
	}
	if getenv("NO_COLOR") != "" {
		return MotionReduced
	}
	return MotionFull
}

var motionMode atomic.Int32

// Motion returns the active motion dial position.
func Motion() MotionMode {
	return MotionMode(motionMode.Load())
}

// SetMotion installs the motion dial position; call once at startup with
// ResolveMotion's result.
func SetMotion(m MotionMode) {
	motionMode.Store(int32(m))
}

// reducedMotionDivisor slows the reduced dial's glyph cadence to a quarter
// of the frame clock — still alive, far calmer.
const reducedMotionDivisor = 4

// SpinnerGlyph returns the spinner glyph for one frame of the shared clock
// — a pure function of (mode, frame): full advances one glyph per frame,
// reduced every fourth, off always renders frame zero.
func SpinnerGlyph(mode MotionMode, frame uint64) string {
	switch mode {
	case MotionOff:
		return spinnerFrames[0]
	case MotionReduced:
		frame /= reducedMotionDivisor
	}
	return spinnerFrames[frame%uint64(len(spinnerFrames))]
}
