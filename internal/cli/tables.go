package cli

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// minLeaderWrapWidth floors the per-row wrap budget so a very long key never
// starves the value column down to nothing.
const minLeaderWrapWidth = 12

// headerName is the shared "NAME" column header used by every entity table
// (addon list, addon verify, node list, node status).
const headerName = "NAME"

// printLeaders prints rows as two-space-indented dotted key/value leaders at
// tui.DefaultKeyColWidth, wrapping each value to the terminal width.
func printLeaders(w io.Writer, rows [][2]string) error {
	const gutter = 2
	width := tui.TerminalWidth()

	for _, row := range rows {
		key, value := row[0], row[1]

		// The key+dots prefix DottedKeyValueFull actually renders can exceed
		// tui.DefaultKeyColWidth once dotsNeeded's floor engages (long key),
		// so measure it per row rather than assuming a fixed column.
		prefix := lipgloss.Width(tui.DottedKeyValueFull(key, "", tui.DefaultKeyColWidth, 0))
		avail := max(width-gutter-prefix, minLeaderWrapWidth)
		wrapped := tui.WrapLines(value, avail)

		if _, err := fmt.Fprintln(w, strings.Repeat(" ", gutter)+tui.DottedKeyValueFull(key, wrapped[0], tui.DefaultKeyColWidth, 0)); err != nil {
			return err
		}
		continuationIndent := strings.Repeat(" ", gutter+prefix)
		for _, cont := range wrapped[1:] {
			if _, err := fmt.Fprintln(w, tui.Downsample(continuationIndent+cont)); err != nil {
				return err
			}
		}
	}
	return nil
}

// printTable writes headers and rows as a tui.Table, downsampling each line
// to honor NO_COLOR and non-terminal output.
func printTable(w io.Writer, headers []string, rows [][]string, opts tui.TableOptions) error {
	for _, line := range tui.Table(headers, rows, opts) {
		if _, err := fmt.Fprintln(w, tui.Downsample(line)); err != nil {
			return err
		}
	}
	return nil
}
