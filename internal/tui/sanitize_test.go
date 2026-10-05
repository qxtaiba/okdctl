package tui

import (
	"bytes"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/colorprofile"
)

func TestSanitizeTerminalEscapes_Families(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain ascii", "node-01", "node-01"},
		{"cjk", "存储池-01", "存储池-01"},
		{"accents", "brücke-café", "brücke-café"},
		{"emoji", "node 🔥 hot", "node 🔥 hot"},
		{"tab preserved", "a\tb", "a\tb"},
		{"bel", "a\x07b", "a" + controlMarker + "b"},
		{"cr", "a\rb", "a" + controlMarker + "b"},
		{"lf", "a\nb", "a" + controlMarker + "b"},
		{"other c0", "a\x01b", "a" + controlMarker + "b"},
		{"del", "a\x7fb", "a" + controlMarker + "b"},
		{"c1 control literal", "a\u0085b", "a" + controlMarker + "b"},
		// ESC immediately followed by an ordinary letter is not one of the
		// recognized families (CSI/OSC/DCS/APC/PM/SOS/nF): only the ESC byte
		// -- the one byte that could have introduced a sequence -- is
		// neutralized, and the unrelated letter after it survives as plain
		// text instead of being swallowed as collateral damage.
		{"esc then ordinary letter", "a\x1bbc", "a" + controlMarker + "bc"},
		{"csi sgr", "a\x1b[31mred\x1b[0mb", "a" + controlMarker + "red" + controlMarker + "b"},
		{"csi cursor move", "a\x1b[2;5Hb", "a" + controlMarker + "b"},
		{"osc window title", "a\x1b]2;evil title\x07b", "a" + controlMarker + "b"},
		{"osc with st terminator", "a\x1b]2;evil title\x1b\\b", "a" + controlMarker + "b"},
		{"osc with c1 st terminator", "a\x1b]2;evil title\u009cb", "a" + controlMarker + "b"},
		{"dcs", "a\x1bPsome dcs payload\x1b\\b", "a" + controlMarker + "b"},
		{"apc", "a\x1b_payload\x1b\\b", "a" + controlMarker + "b"},
		{"pm", "a\x1b^payload\x1b\\b", "a" + controlMarker + "b"},
		{"sos", "a\x1bXpayload\x1b\\b", "a" + controlMarker + "b"},
		{"nf charset designation", "a\x1b(Bb", "a" + controlMarker + "b"},
		// RIS ('c', an Fs-range byte) is a real, dangerous xterm reset
		// sequence, but it is destroyed all the same: the terminal can only
		// act on it by reading a literal ESC immediately followed by 'c', and
		// the ESC itself is gone from the output no matter what follows it.
		{"esc then reserved fs byte (RIS)", "a\x1bcb", "a" + controlMarker + "cb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeTerminalEscapes(tc.in)
			if got != tc.want {
				t.Fatalf("SanitizeTerminalEscapes(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("SanitizeTerminalEscapes(%q) produced invalid UTF-8: %q", tc.in, got)
			}
		})
	}
}

// TestSanitizeTerminalEscapes_NestedStraddlingSequences proves an OSC
// smuggled inside what looks like a CSI parameter list — the kind of input a
// single greedy regex scanner could misparse by matching from the first ESC
// all the way to a later unrelated final byte — is still recognized as two
// separate sequences: the unterminated CSI stops at the embedded ESC instead
// of swallowing it, and the OSC is then scanned fresh from there.
func TestSanitizeTerminalEscapes_NestedStraddlingSequences(t *testing.T) {
	in := "x\x1b[1;\x1b]0;title\x07m-tail"
	got := SanitizeTerminalEscapes(in)

	if strings.ContainsRune(got, '\x1b') {
		t.Fatalf("output still contains ESC: %q", got)
	}
	want := "x" + controlMarker + controlMarker + "m-tail"
	if got != want {
		t.Fatalf("SanitizeTerminalEscapes(%q) = %q, want %q", in, got, want)
	}
}

// TestSanitizeTerminalEscapes_EscapeEmbeddedMidUTF8 splices a CSI sequence
// into the middle of a valid 3-byte UTF-8 rune (orphaning its lead and
// trailing continuation bytes), and checks the corruption stays local: the
// escape is neutralized, no byte panics or round-trips into garbage, and an
// untouched multi-byte rune elsewhere in the same string survives intact.
func TestSanitizeTerminalEscapes_EscapeEmbeddedMidUTF8(t *testing.T) {
	ri := "日" // E6 97 A5
	lead, mid, tail := ri[0:1], ri[1:2], ri[2:3]
	in := "x" + lead + "\x1b[31m" + mid + tail + "y 日本語"

	got := SanitizeTerminalEscapes(in)

	if !utf8.ValidString(got) {
		t.Fatalf("output is invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, '\x1b') {
		t.Fatalf("output still contains ESC: %q", got)
	}
	if !strings.HasSuffix(got, "y 日本語") {
		t.Fatalf("untouched trailing CJK text was corrupted: %q", got)
	}
}

// TestSanitizeTerminalEscapes_InvalidUTF8 checks a grab-bag of malformed
// byte sequences — a lone continuation byte, an overlong lead byte with
// nothing following, a truncated multi-byte rune at end of string — never
// panics and always yields valid UTF-8 output.
func TestSanitizeTerminalEscapes_InvalidUTF8(t *testing.T) {
	cases := []string{
		"\x80",
		"\xc2",
		"a\xe6\x97b",
		"\xff\xfe\xfd",
		"ok\xc0\x80end",
	}
	for _, in := range cases {
		got := SanitizeTerminalEscapes(in)
		if !utf8.ValidString(got) {
			t.Fatalf("SanitizeTerminalEscapes(%q) produced invalid UTF-8: %q", in, got)
		}
	}
}

// TestSanitizeTerminalEscapes_LoneEscAtEndOfString checks a trailing,
// unintroduced ESC with nothing after it is still neutralized rather than
// left dangling.
func TestSanitizeTerminalEscapes_LoneEscAtEndOfString(t *testing.T) {
	got := SanitizeTerminalEscapes("tail\x1b")
	want := "tail" + controlMarker
	if got != want {
		t.Fatalf("SanitizeTerminalEscapes(%q) = %q, want %q", "tail\x1b", got, want)
	}
}

// TestSanitizeTerminalEscapes_NeutralizesUntrustedOSC8Hyperlink proves that
// an OSC 8 hyperlink — the exact escape family okdctl's own Hyperlink helper
// legitimately emits for trusted output — is fully neutralized when it
// arrives as untrusted text instead, e.g. a Proxmox node name engineered to
// render a spoofed link.
func TestSanitizeTerminalEscapes_NeutralizesUntrustedOSC8Hyperlink(t *testing.T) {
	malicious := "\x1b]8;;http://attacker.example/evil\x1b\\clickme\x1b]8;;\x1b\\"

	got := SanitizeTerminalEscapes(malicious)

	if strings.ContainsRune(got, '\x1b') {
		t.Fatalf("hyperlink escape survived sanitization: %q", got)
	}
	if !strings.Contains(got, "clickme") {
		t.Fatalf("visible link text was lost, not just the escapes: %q", got)
	}
	if strings.Contains(got, "attacker.example") {
		t.Fatalf("the spoofed URL leaked into the sanitized output: %q", got)
	}
}

// TestSanitizeTerminalEscapes_WouldAlsoStripOwnHyperlink documents why
// SanitizeTerminalEscapes must gate on trust at the call site, not on
// content: it cannot distinguish okdctl's own legitimate Hyperlink() OSC 8
// escapes from a forged copy, so running trusted, already-rendered output
// back through it breaks the very link it tried to produce.
func TestSanitizeTerminalEscapes_WouldAlsoStripOwnHyperlink(t *testing.T) {
	forced := colorprofile.TrueColor
	outputProfile.Store(&forced)
	t.Cleanup(func() { SetColorProfileFor(&bytes.Buffer{}) })

	trusted := Hyperlink("https://okdctl.dev", "click here")
	if !strings.Contains(trusted, "\x1b]8;;") {
		t.Fatalf("test setup failed to make Hyperlink emit an OSC 8 escape: %q", trusted)
	}

	got := SanitizeTerminalEscapes(trusted)

	if strings.Contains(got, "\x1b]8;;") {
		t.Fatalf("expected Sanitize to strip even okdctl's own hyperlink escape: %q", got)
	}
	if !strings.Contains(got, "click here") {
		t.Fatalf("visible text should still survive: %q", got)
	}
}

func FuzzSanitizeTerminalEscapes(f *testing.F) {
	seeds := []string{
		"",
		"plain text",
		"\x1b[31mred\x1b[0m",
		"\x1b]8;;http://x\x1b\\t\x1b]8;;\x1b\\",
		"\x1bX\x1b^\x1b_\x1bP",
		"\x1b[1;\x1b]0;title\x07m",
		string([]byte{0xff, 0xfe, 0x1b, 0x80}),
		"héllo 日本語 😀",
		"\x1b",
		"a\tb\nc\rd",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := SanitizeTerminalEscapes(s)
		if !utf8.ValidString(got) {
			t.Fatalf("output is invalid UTF-8 for input %q: %q", s, got)
		}
		for _, r := range got {
			if r == '\x1b' {
				t.Fatalf("output contains ESC for input %q: %q", s, got)
			}
			if unicode.IsControl(r) && r != '\t' {
				t.Fatalf("output contains control rune %U for input %q: %q", r, s, got)
			}
		}
	})
}
