package tui

import (
	"strings"
	"unicode"
)

// controlMarker makes every neutralised byte visible with the glyph Go's own
// UTF-8 decoding uses, so a sanitised value never passes as a faithful copy.
const controlMarker = "\uFFFD"

// This is a terminal-escape check, not secret redaction: apply it to text
// from outside okdctl (Proxmox names, backend errors), never to okdctl's own
// chrome, whose OSC 8 links and SGR styling it would strip the same way.

// SanitizeTerminalEscapes replaces every C0/C1 control and escape sequence
// in s with controlMarker, leaving tab, printable text and multi-byte UTF-8
// untouched.
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
			skipTo = i + 1 + scanEscape(s[i+1:])
			b.WriteString(controlMarker)
		case r == '\t':
			// Tab cannot move the cursor or fake chrome, and pasted tabular text
			// carries it for alignment.
			b.WriteByte('\t')
		case unicode.IsControl(r):
			b.WriteString(controlMarker)
		default:
			// The range loop already folded invalid bytes into U+FFFD, so nothing here
			// can resynthesise part of an escape.
			b.WriteRune(r)
		}
	}
	return b.String()
}

// scanEscape returns how many bytes after an ESC belong to the sequence it
// introduced (CSI, OSC/DCS/APC/PM/SOS, nF); anything else marks the lone ESC
// only, which is safe because the caller catches every ESC and C1 byte
// regardless.
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

// scanFinal consumes [0x20,bodyHi] bytes up to and including the first in
// [finalLo,0x7e], never swallowing a following sequence.
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

// scanStringTerminator consumes up to and including BEL, ESC '\' or the
// UTF-8 C1 ST (U+009C), or all of rest when none appears.
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
