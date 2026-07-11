// Package system provides small, dependency-light host primitives used
// across okdctl: filesystem helpers and privileged file writes (fs.go),
// sudo re-exec and permission elevation (elevation.go), systemd unit
// control (systemd.go), a generic context-aware polling loop (wait.go),
// and byte-buffer/UUID primitives with no narrower existing home
// (runid.go, zeroize.go). It does not execute arbitrary subprocesses —
// see internal/executor for the command-execution stack.
package system
