package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// OptionStyle categorises a Selector option so the renderer can apply a
// consistent color and badge treatment.
type OptionStyle int

// OptionStyle values, all mapped from releases.ReleaseType by
// steps/distribution_loader.go::releaseTypeToOptionStyle.
const (
	OptionStyleDefault       OptionStyle = iota
	OptionStyleLatestStable              // Green - latest stable release
	OptionStyleStable                    // Cyan - stable release
	OptionStylePreview                   // Yellow/amber - preview/prerelease
	OptionStyleLatestPreview             // Yellow with badge - latest preview
	OptionStyleLTS                       // Cyan/blue - long-term support
)

// Option is a single styled entry in a Selector.
type Option struct {
	ID          string
	Title       string
	Description string
	Recommended bool
	Style       OptionStyle
	InDropdown  bool // part of the scrollable dropdown region
}

// Selector is a vertical option list with a scrollable dropdown region.
type Selector struct {
	options              []Option
	selected             int
	focused              bool
	dropdownScrollOffset int

	// cachedStyles is a lazily-initialised cache of the lipgloss.Style objects
	// used when rendering options. Caching is safe because tui.Color* values
	// are only mutated during tui package init (via the HOMELAB_HIGH_CONTRAST
	// env var) and never change thereafter.
	cachedStyles *optionStyles
}

// NewSelector builds a Selector starting focused on the first option.
func NewSelector(options []Option) *Selector {
	return &Selector{
		options:  options,
		selected: 0,
		focused:  true,
	}
}

// SetOptions replaces the option list and clamps the selection index to the
// new slice length.
func (s *Selector) SetOptions(options []Option) {
	s.options = options
	s.dropdownScrollOffset = 0
	if s.selected >= len(options) {
		s.selected = len(options) - 1
	}
	if s.selected < 0 {
		s.selected = 0
	}
}

// Selected returns the currently highlighted Option, or the zero value when
// the option list is empty.
func (s *Selector) Selected() Option {
	if s.selected >= 0 && s.selected < len(s.options) {
		return s.options[s.selected]
	}
	return Option{}
}

// SelectedIndex returns the current selection's index in the option list.
func (s *Selector) SelectedIndex() int {
	return s.selected
}

// SetSelectedByID moves the selection to the first option whose ID matches.
// Unknown IDs are silently ignored.
func (s *Selector) SetSelectedByID(id string) {
	for i, opt := range s.options {
		if opt.ID == id {
			s.selected = i
			return
		}
	}
}

// SetFocused toggles keyboard focus on the selector.
func (s *Selector) SetFocused(focused bool) {
	s.focused = focused
}

// Update handles up/down and j/k key presses to move the selection.
func (s *Selector) Update(msg tea.Msg) (*Selector, tea.Cmd) {
	if !s.focused {
		return s, nil
	}

	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			s.moveUp()
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			s.moveDown()
		}
	}

	return s, nil
}

type optionStyles struct {
	bulletSelected   lipgloss.Style
	bulletUnselected lipgloss.Style
	desc             lipgloss.Style
	recommended      lipgloss.Style
	line             lipgloss.Style
}

func (s *Selector) getOptionStyles() optionStyles {
	if s.cachedStyles != nil {
		return *s.cachedStyles
	}
	styles := optionStyles{
		bulletSelected:   lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true),
		bulletUnselected: lipgloss.NewStyle().Foreground(tui.ColorSlate600),
		desc:             lipgloss.NewStyle().Foreground(tui.ColorSlate500),
		recommended:      lipgloss.NewStyle().Foreground(tui.ColorSuccess).Italic(true),
		line:             lipgloss.NewStyle().Foreground(tui.ColorSlate700),
	}
	s.cachedStyles = &styles
	return styles
}

func (s *Selector) getTitleStyle(style OptionStyle) lipgloss.Style {
	switch style {
	case OptionStyleLatestStable:
		return lipgloss.NewStyle().Foreground(tui.ColorSuccess)
	case OptionStyleStable:
		return lipgloss.NewStyle().Foreground(tui.ColorCyan400)
	case OptionStylePreview, OptionStyleLatestPreview:
		return lipgloss.NewStyle().Foreground(tui.ColorWarning)
	case OptionStyleLTS:
		return lipgloss.NewStyle().Foreground(tui.ColorInfo)
	default:
		return lipgloss.NewStyle().Foreground(tui.ColorSlate300)
	}
}

// View renders the selector as a vertical list with top-of-list options
// above the scrollable dropdown region.
func (s *Selector) View() string {
	var lines []string

	dropdownStart, dropdownEnd := s.getDropdownBounds()

	scrollIndicatorStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	dropdownBorderStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate700)

	i := 0
	for i < len(s.options) {
		opt := &s.options[i]

		if !opt.InDropdown {
			isSelected := i == s.selected
			isLast := i == len(s.options)-1
			showConnector := !isLast && !s.options[i+1].InDropdown
			optView := s.renderOption(opt, isSelected, showConnector)
			lines = append(lines, optView)
			i++
		} else {
			dropdownLines := s.renderDropdownRegion(dropdownStart, dropdownEnd, &scrollIndicatorStyle, &dropdownBorderStyle)
			lines = append(lines, dropdownLines...)

			i = dropdownEnd + 1
		}
	}

	return strings.Join(lines, "\n")
}

func (s *Selector) renderOption(opt *Option, selected, showConnector bool) string {
	return s.renderOptionWithPrefix(opt, selected, showConnector, "")
}

func (s *Selector) renderOptionWithPrefix(opt *Option, selected, showConnector bool, prefix string) string {
	var result []string

	styles := s.getOptionStyles()
	titleStyle := s.getTitleStyle(opt.Style)

	var bullet, title string
	if selected {
		bullet = styles.bulletSelected.Render("●")
		title = titleStyle.Bold(true).Render(opt.Title)
	} else {
		bullet = styles.bulletUnselected.Render("○")
		title = titleStyle.Render(opt.Title)
	}

	titleLine := prefix + bullet + " " + title
	if opt.Recommended {
		titleLine += " " + styles.recommended.Render("(recommended)")
	}
	result = append(result, titleLine)

	if opt.Description != "" {
		descLines := strings.Split(opt.Description, "\n")
		for _, line := range descLines {
			result = append(result, prefix+styles.line.Render("  │ ")+styles.desc.Render(line))
		}
	}

	if showConnector {
		result = append(result, prefix+styles.line.Render("  │"))
	}

	return strings.Join(result, "\n")
}

// CompactSelector is a lightweight Selector variant that renders a simple
// radio-style list without dropdown scrolling.
type CompactSelector struct {
	options  []string
	selected int
	focused  bool
}

// NewCompactSelector builds a CompactSelector starting focused on index 0.
func NewCompactSelector(options []string) *CompactSelector {
	return &CompactSelector{
		options:  options,
		selected: 0,
		focused:  true,
	}
}

// Len returns the number of options currently in the selector.
func (s *CompactSelector) Len() int {
	return len(s.options)
}

// SelectedIndex returns the current selection's index.
func (s *CompactSelector) SelectedIndex() int {
	return s.selected
}

// SetFocused toggles keyboard focus on the selector.
func (s *CompactSelector) SetFocused(focused bool) {
	s.focused = focused
}

// Update handles up/down and j/k key presses to move the selection.
func (s *CompactSelector) Update(msg tea.Msg) (*CompactSelector, tea.Cmd) {
	if !s.focused {
		return s, nil
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "k"))):
			s.selected--
			if s.selected < 0 {
				s.selected = len(s.options) - 1
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "j"))):
			s.selected++
			if s.selected >= len(s.options) {
				s.selected = 0
			}
		}
	}

	return s, nil
}

// View renders the options as a vertical radio-style list, one per line.
func (s *CompactSelector) View() string {
	var lines []string

	selectedStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	unselectedStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate400)

	for i, opt := range s.options {
		var line string
		if i == s.selected {
			line = selectedStyle.Render("● " + opt)
		} else {
			line = unselectedStyle.Render("○ " + opt)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}
