package tui

import "sync/atomic"

// MotionMode is the animation dial: full, reduced, or off (animations
// freeze at their first frame).
type MotionMode int32

// Motion dial positions, most animated first.
const (
	MotionFull MotionMode = iota
	MotionReduced
	MotionOff
)

// ResolveMotion resolves the dial, most explicit first: --no-motion, then
// OKDCTL_NO_MOTION, then no-color by flag or NO_COLOR, which implies
// reduced only.
func ResolveMotion(flagOff, flagNoColor bool, getenv func(string) string) MotionMode {
	if flagOff {
		return MotionOff
	}
	if v := getenv("OKDCTL_NO_MOTION"); v == "1" || v == "true" {
		return MotionOff
	}
	if flagNoColor || getenv("NO_COLOR") != "" {
		return MotionReduced
	}
	return MotionFull
}

var motionMode atomic.Int32

// Motion returns the active motion dial position.
func Motion() MotionMode {
	return MotionMode(motionMode.Load())
}

// SetMotion installs the dial position; call once at startup.
func SetMotion(m MotionMode) {
	motionMode.Store(int32(m))
}

const reducedMotionDivisor = 4

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// SpinnerGlyph returns the spinner glyph for one frame: full advances every
// frame, reduced every fourth, off always frame zero.
func SpinnerGlyph(mode MotionMode, frame uint64) string {
	switch mode {
	case MotionOff:
		return spinnerFrames[0]
	case MotionReduced:
		frame /= reducedMotionDivisor
	}
	return spinnerFrames[frame%uint64(len(spinnerFrames))]
}
