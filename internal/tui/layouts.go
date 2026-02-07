package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Layout dimension constants
const (
	// DefaultBoxWidth is the standard width for boxed sections across the CLI.
	DefaultBoxWidth = 90

	// MinBoxWidth is the minimum width for boxed sections.
	MinBoxWidth = 20

	// DefaultBoxWidthFallback is the fallback width when a box is too narrow.
	DefaultBoxWidthFallback = 60

	// TitlePadding is the padding around titles in boxed sections.
	TitlePadding = 4

	// TitlePaddingCompact is the extra padding for compact boxed sections.
	TitlePaddingCompact = 8

	// ContentPadding is the padding for content inside boxes.
	ContentPadding = 2
)

// ═══════════════════════════════════════════════════════════════════════════════
// BOXED PANELS - Rich-style panels with centered headers
// ═══════════════════════════════════════════════════════════════════════════════

// boxConfig contains style configuration for boxed sections.
type boxConfig struct {
	borderColor lipgloss.Color
	titleColor  lipgloss.Color
	compact     bool
}

// maxLineWidth returns the maximum visual width of any line in the content.
func maxLineWidth(content string) int {
	maxWidth := 0
	for _, line := range strings.Split(content, "\n") {
		if w := lipgloss.Width(line); w > maxWidth {
			maxWidth = w
		}
	}
	return maxWidth
}

// boxedSectionCore is the common implementation for all boxed section variants.
func boxedSectionCore(content, title string, width int, cfg boxConfig) string {
	if width < MinBoxWidth {
		width = DefaultBoxWidthFallback
	}

	borderStyle := lipgloss.NewStyle().Foreground(cfg.borderColor)
	titleStyle := lipgloss.NewStyle().Foreground(cfg.titleColor).Bold(true)

	lines := strings.Split(content, "\n")
	maxContentWidth := maxLineWidth(content)

	titleUpper := strings.ToUpper(title)
	titleLen := lipgloss.Width(titleUpper)

	var minWidthForTitle int
	if cfg.compact {
		minWidthForTitle = titleLen + ContentPadding + TitlePaddingCompact
	} else {
		minWidthForTitle = titleLen + TitlePadding
	}

	innerWidth := width - ContentPadding
	if maxContentWidth+ContentPadding > innerWidth {
		innerWidth = maxContentWidth + ContentPadding
	}
	if minWidthForTitle > innerWidth {
		innerWidth = minWidthForTitle
	}

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
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		padding := innerWidth - lineWidth
		if padding < 0 {
			padding = 0
		}
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

// BoxedSection creates a boxed panel with an ALL CAPS title centered in the header row.
// The box width dynamically expands to fit content if content is wider than specified width.
func BoxedSection(content string, title string, width int) string {
	return boxedSectionCore(content, title, width, boxConfig{
		borderColor: ColorSlate600,
		titleColor:  ColorSlate100,
		compact:     false,
	})
}

// BoxedSectionCompact creates a panel with the title embedded in the top border.
// The box width dynamically expands to fit content if content is wider than specified width.
func BoxedSectionCompact(content string, title string, width int) string {
	return boxedSectionCore(content, title, width, boxConfig{
		borderColor: ColorSlate600,
		titleColor:  ColorSlate300,
		compact:     true,
	})
}
