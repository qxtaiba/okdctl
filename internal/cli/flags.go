package cli

// Flag names referenced both at registration (BoolVar/StringVarP) and at
// read-back (GetBool/GetString). A typo on either side silently returns
// the zero value at runtime — Go has no compile-time check across the
// flag-set string key — so both sites reference these constants.
const (
	flagDryRun = "dry-run"
	flagOutput = "output"
)
