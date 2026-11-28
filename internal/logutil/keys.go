package logutil

// Log attr key glossary — one concept, one key, so log queries can join
// records on them. Pick from here before minting a new key.
//
//	err            error value, passed structured ("err", err) — never err.Error()
//	path           filesystem path (file or directory), absolute where known
//	file           base filename of an artifact (ISO, tarball, binary download)
//	endpoint       Proxmox API URL; redacted of userinfo like url
//	step           ID of the running engine step
//	cmd, argc      executor: command name and argument count (never argv)
//	exit           subprocess exit code
//	exit_code      okdctl's own exit code on a failed command
//	duration       elapsed time of a completed span (time.Duration)
//	attempts       retries consumed before giving up
//	run_id         run correlation ID
