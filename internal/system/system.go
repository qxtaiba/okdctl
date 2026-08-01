// Package system provides small, dependency-light host primitives used
// across okdctl: filesystem helpers and privileged file writes,
// invoking-user ownership and a passwordless-sudo probe, systemd unit
// control, retry/backoff helpers, a generic context-aware polling loop, and
// byte-buffer primitives with no narrower home. It does not execute
// arbitrary subprocesses — see internal/executor for the command-execution
// stack.
package system
