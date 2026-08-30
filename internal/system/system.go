// Package system provides small, dependency-light host primitives:
// filesystem helpers, privileged writes, invoking-user ownership, a
// passwordless-sudo probe, systemd control, retry/backoff, and a generic
// polling loop. It does not execute arbitrary subprocesses — see internal/executor.
package system
