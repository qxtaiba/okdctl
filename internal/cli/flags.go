package cli

// Flag names referenced both at registration (BoolVar/StringVarP) and at
// read-back (GetBool/GetString). A typo on either side silently returns
// the zero value at runtime — Go has no compile-time check across the
// flag-set string key — so both sites reference these constants.
const (
	flagDryRun      = "dry-run"
	flagOutput      = "output"
	flagOutputShort = "o"
	flagOutputFile  = "output-file"
	flagOnly        = "only"
	flagTarget      = "target"
)

// Output-format values for the --output/-o flag. Mirrors kubectl/oc:
// "text" is the human-readable default; "json" is the machine-readable
// schema documented in docs/cli/json-schema.md.
const (
	outputText = "text"
	outputJSON = "json"
)

// annotationKeyRequiresRoot tags cobra commands whose body must run as
// root (writes to /etc, /usr/local/bin, /var/www/html, systemd, firewalls).
// The PersistentPreRunE in elevation.go re-execs under sudo when the
// caller's euid is non-zero and this annotation (or rootRequiredCmds
// ancestry) is set.
const annotationKeyRequiresRoot = "requiresRoot"
