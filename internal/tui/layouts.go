package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Layout constants for BoxedSection rendering. DefaultBoxWidth is the
// nominal content width (90 leaves room for the 1-col lipgloss border on
// each side plus 2-col ContentPadding in a 94-col terminal).
const (
	DefaultBoxWidth         = 90
	MinBoxWidth             = 20
	DefaultBoxWidthFallback = 60
	TitlePadding            = 4
	TitlePaddingCompact     = 8
	ContentPadding          = 2
)

type boxConfig struct {
	borderColor color.Color
	titleColor  color.Color
	compact     bool
}

func maxLineWidth(content string) int {
	var m int
	for line := range strings.SplitSeq(content, "\n") {
		m = max(m, lipgloss.Width(line))
	}
	return m
}

func boxedSectionCore(content, title string, width int, cfg boxConfig) string {
	if width < MinBoxWidth {
		width = DefaultBoxWidthFallback
	}

	borderStyle := lipgloss.NewStyle().Foreground(cfg.borderColor)
	titleStyle := lipgloss.NewStyle().Foreground(cfg.titleColor).Bold(true)

	maxContentWidth := maxLineWidth(content)

	titleUpper := strings.ToUpper(title)
	titleLen := lipgloss.Width(titleUpper)

	var minWidthForTitle int
	if cfg.compact {
		minWidthForTitle = titleLen + ContentPadding + TitlePaddingCompact
	} else {
		minWidthForTitle = titleLen + TitlePadding
	}

	innerWidth := max(width-ContentPadding, maxContentWidth+ContentPadding, minWidthForTitle)

	var topBorder string
	var titleRow string
	var separator string

	if cfg.compact {
		titleLenWithSpaces := titleLen + ContentPadding
		if titleLenWithSpaces >= innerWidth-TitlePadding {
			return boxedSectionCore(content, title, width, boxConfig{
				borderColor: cfg.borderColor,
				titleColor:  cfg.titleColor,
				compact:     false,
			})
		}

		leftDashes := (innerWidth - titleLenWithSpaces) / 2
		rightDashes := innerWidth - titleLenWithSpaces - leftDashes

		topBorder = borderStyle.Render("╭") +
			borderStyle.Render(strings.Repeat("─", leftDashes)) +
			titleStyle.Render(" "+titleUpper+" ") +
			borderStyle.Render(strings.Repeat("─", rightDashes)) +
			borderStyle.Render("╮")
	} else {
		topBorder = borderStyle.Render("╭" + strings.Repeat("─", innerWidth) + "╮")

		leftPad := (innerWidth - titleLen) / 2
		rightPad := innerWidth - titleLen - leftPad
		titleRow = borderStyle.Render("│") +
			strings.Repeat(" ", leftPad) +
			titleStyle.Render(titleUpper) +
			strings.Repeat(" ", rightPad) +
			borderStyle.Render("│")

		separator = borderStyle.Render("├" + strings.Repeat("─", innerWidth) + "┤")
	}

	var contentRows []string
	for line := range strings.SplitSeq(content, "\n") {
		padding := max(innerWidth-lipgloss.Width(line), 0)
		row := borderStyle.Render("│") + line + strings.Repeat(" ", padding) + borderStyle.Render("│")
		contentRows = append(contentRows, row)
	}

	bottomBorder := borderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯")

	var result []string
	if cfg.compact {
		result = []string{topBorder}
	} else {
		result = []string{topBorder, titleRow, separator}
	}
	result = append(result, contentRows...)
	result = append(result, bottomBorder)

	return strings.Join(result, "\n")
}

// BoxedSectionCompact renders content inside a single-line-titled box using
// the muted slate palette.
func BoxedSectionCompact(content, title string, width int) string {
	return boxedSectionCore(content, title, width, boxConfig{
		borderColor: ColorSlate600,
		titleColor:  ColorSlate300,
		compact:     true,
	})
}
