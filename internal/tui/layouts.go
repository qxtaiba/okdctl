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

// normalizeVerticalPadding trims blank lines from both ends of content and
// re-adds exactly one, so every box carries symmetric one-row top/bottom
// padding regardless of how many trailing Newline() calls a Builder made.
func normalizeVerticalPadding(content string) string {
	lines := strings.Split(content, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return "\n" + strings.Join(lines, "\n") + "\n"
}

func boxedSectionCore(content, title string, width int, cfg boxConfig) string {
	if width < MinBoxWidth {
		width = DefaultBoxWidthFallback
	}

	content = normalizeVerticalPadding(content)

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

	return Downsample(strings.Join(result, "\n"))
}

// BoxedSectionCompact renders content inside a single-line-titled box. The
// title carries the brand purple and the border a dimmed brand tint so every
// box reads as okdctl; color is stripped when the active profile is not a
// truecolor TTY.
func BoxedSectionCompact(content, title string, width int) string {
	return boxedSectionCore(content, title, width, boxConfig{
		borderColor: ColorPrimaryDim,
		titleColor:  ColorPrimary,
		compact:     true,
	})
}

// BoxedSectionAccent renders a compact box whose title and border take a
// caller-chosen accent color, used by the error box to skin the same chrome in
// red without duplicating the layout core.
func BoxedSectionAccent(content, title string, width int, accent color.Color) string {
	return boxedSectionCore(content, title, width, boxConfig{
		borderColor: accent,
		titleColor:  accent,
		compact:     true,
	})
}
