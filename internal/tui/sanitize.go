package tui

import (
	"strings"
	"unicode"
)

// controlMarker is the stand-in SanitizeTerminalEscapes writes for every
// control byte or escape sequence it neutralizes. A silently dropped byte
// would leave a sanitized value that looks like a faithful, same-length copy
// of untrusted text that actually carried a control sequence; the Unicode
// replacement character makes the redaction visible instead, using the same
// glyph Go's own UTF-8 decoding already uses for invalid bytes so the output
// carries one consistent "something unsafe was here" signal rather than two.
const controlMarker = "\uFFFD"

// This is a terminal-escape check, distinct from and in addition to secret
// redaction (internal/logutil.RedactHandler, ScrubSecrets): it has no notion
// of what text means, only of what it could do to the terminal that renders
// it. Apply it to text that originates outside okdctl's control — Proxmox
// node/storage/bridge/VM names, backend error strings, surfaced log lines —
// never to okdctl's own trusted chrome, which legitimately emits escapes
// (Hyperlink's OSC 8 links, lipgloss SGR styling) that this function cannot
// tell apart from a forged copy and would strip all the same.

// SanitizeTerminalEscapes renders untrusted text s safe for terminal display by replacing every C0 control, C1 control, and CSI/OSC/DCS/APC/PM/SOS escape sequence with controlMarker, leaving tab, ordinary printable text, and multi-byte UTF-8 (CJK, accents, emoji) untouched.
func SanitizeTerminalEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	skipTo := 0
	for i, r := range s {
		if i < skipTo {
			continue
		}
		switch {
		case r == '\x1b':
			// ESC is always a one-byte rune, so i+1 is exactly the byte
			// offset where the introduced sequence's body begins.
			skipTo = i + 1 + scanEscape(s[i+1:])
			b.WriteString(controlMarker)
		case r == '\t':
			// Tab is the one C0 control let through unmodified: unlike
			// ESC/CR/LF it cannot reposition the cursor, clear a region, or
			// fabricate a new line of fake chrome, and untrusted tabular
			// text (pasted error output, log excerpts) routinely carries
			// it for alignment.
			b.WriteByte('\t')
		case unicode.IsControl(r):
			// Every other C0 control (BEL, CR, LF, ...), DEL, and every C1
			// control (U+0080-U+009F) — the same hazards the ESC case
			// handles when they arrive already escape-introduced, here
			// arriving as lone bytes instead.
			b.WriteString(controlMarker)
		default:
			// Ordinary printable text, including invalid UTF-8: the range
			// loop above already folds any invalid byte into the harmless
			// U+FFFD replacement rune before this switch ever sees it, so
			// what reaches here is never a raw byte that could resynthesize
			// part of an escape.
			b.WriteRune(r)
		}
	}
	return b.String()
}

// scanEscape returns the number of bytes in rest — the text immediately
// following an ESC byte — that belong to the sequence ESC introduced, so the
// caller can mark the whole sequence, not just the ESC byte, as unsafe. It
// recognizes CSI ([), OSC/DCS/APC/PM/SOS (], P, _, ^, X), and the nF form (an
// intermediate-byte run narrowly introduced by 0x20-0x2f, ending in a final
// byte). Anything else right after ESC — including a byte ECMA-48 reserves
// as a single-byte escape final (e.g. 'c', RIS) but that is indistinguishable
// from an ordinary adjacent letter without that context — belongs to no
// family this scanner targets: only the lone ESC is marked, leaving the next
// byte for the caller to reprocess on its own. This never reopens the door
// to an escape: the one byte that can ever introduce one, ESC itself (or a
// literal C1 control), is always caught by the caller regardless of what
// scanEscape recognizes here, so under-matching here only risks leaving an
// orphaned, inert byte of body text, never a live control sequence.
func scanEscape(rest string) int {
	if rest == "" {
		return 0
	}
	switch rest[0] {
	case '[': // CSI: parameter/intermediate bytes 0x20-0x3f, final 0x40-0x7e.
		return 1 + scanFinal(rest[1:], 0x3f, 0x40)
	case ']', 'P', '_', '^', 'X': // OSC, DCS, APC, PM, SOS: a string body.
		return 1 + scanStringTerminator(rest[1:])
	}
	if rest[0] >= 0x20 && rest[0] <= 0x2f { // nF: intermediate bytes, then a final byte.
		return 1 + scanFinal(rest[1:], 0x2f, 0x30)
	}
	return 0
}

// scanFinal consumes bytes in [0x20,bodyHi] — the shared CSI/nF intermediate
// range — until it finds one in [finalLo,0x7e], returning the count
// including that final byte. It stops without consuming a byte outside both
// ranges, so a second escape sequence (or ordinary text) immediately
// following an unterminated one is never swallowed into it, and consumes to
// the end of rest if no final byte ever appears.
func scanFinal(rest string, bodyHi, finalLo byte) int {
	for i := 0; i < len(rest); i++ {
		switch b := rest[i]; {
		case b >= finalLo && b <= 0x7e:
			return i + 1
		case b < 0x20 || b > bodyHi:
			return i
		}
	}
	return len(rest)
}

// scanStringTerminator consumes rest up to and including the first string
// terminator for an OSC/DCS/APC/PM/SOS payload — BEL, the two-byte ESC '\'
// form, or the two-byte UTF-8 encoding of the single-rune C1 ST (U+009C) —
// swallowing any other byte as payload, and consumes to the end of rest if
// no terminator appears.
func scanStringTerminator(rest string) int {
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case 0x07:
			return i + 1
		case 0x1b:
			if i+1 < len(rest) && rest[i+1] == '\\' {
				return i + 2
			}
		case 0xc2:
			if i+1 < len(rest) && rest[i+1] == 0x9c {
				return i + 2
			}
		}
	}
	return len(rest)
}
