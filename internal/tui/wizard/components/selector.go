package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
)

type OptionStyle int

const (
	OptionStyleDefault       OptionStyle = iota
	OptionStyleLatestStable              // Green - latest stable release
	OptionStyleStable                    // Cyan - stable release
	OptionStylePreview                   // Yellow/amber - preview/prerelease
	OptionStyleLatestPreview             // Yellow with badge - latest preview
	OptionStyleLTS                       // Cyan/blue - long-term support
)

type Option struct {
	ID           string
	Title        string
	Description  string
	Requirements string
	Recommended  bool
	Disabled     bool
	DisabledMsg  string
	Style        OptionStyle // Visual style for the option
	InDropdown   bool        // Marks options that are part of a scrollable dropdown region
}

type Selector struct {
	options              []Option
	selected             int
	focused              bool
	width                int
	height               int
	dropdownScrollOffset int // current scroll position within the dropdown
	maxDropdownVisible   int // max items to show in dropdown (default 5)

	// cachedStyles is a lazily-initialised cache of the lipgloss.Style objects
	// used when rendering options. Caching is safe because tui.Color* values
	// are only mutated during tui package init (via the HOMELAB_HIGH_CONTRAST
	// env var) and never change thereafter.
	cachedStyles *optionStyles
}

func NewSelector(options []Option) *Selector {
	return &Selector{
		options:              options,
		selected:             0,
		focused:              true,
		dropdownScrollOffset: 0,
		maxDropdownVisible:   5,
	}
}

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

func (s *Selector) SetMaxDropdownVisible(n int) {
	if n > 0 {
		s.maxDropdownVisible = n
	}
}

func (s *Selector) Selected() Option {
	if s.selected >= 0 && s.selected < len(s.options) {
		return s.options[s.selected]
	}
	return Option{}
}

func (s *Selector) SelectedIndex() int {
	return s.selected
}

func (s *Selector) SetSelected(index int) {
	if index >= 0 && index < len(s.options) {
		s.selected = index
	}
}

func (s *Selector) SetSelectedByID(id string) {
	for i, opt := range s.options {
		if opt.ID == id {
			s.selected = i
			return
		}
	}
}

func (s *Selector) SetFocused(focused bool) {
	s.focused = focused
}

func (s *Selector) SetSize(width, height int) {
	s.width = width
	s.height = height
}

func (s *Selector) Update(msg tea.Msg) (*Selector, tea.Cmd) {
	if !s.focused {
		return s, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
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
	titleDisabled    lipgloss.Style
	desc             lipgloss.Style
	req              lipgloss.Style
	recommended      lipgloss.Style
	disabledMsg      lipgloss.Style
	line             lipgloss.Style
}

func (s *Selector) getOptionStyles() optionStyles {
	if s.cachedStyles != nil {
		return *s.cachedStyles
	}
	styles := optionStyles{
		bulletSelected:   lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true),
		bulletUnselected: lipgloss.NewStyle().Foreground(tui.ColorSlate600),
		titleDisabled:    lipgloss.NewStyle().Foreground(tui.ColorSlate600).Strikethrough(true),
		desc:             lipgloss.NewStyle().Foreground(tui.ColorSlate500),
		req:              lipgloss.NewStyle().Foreground(tui.ColorSlate600).Italic(true),
		recommended:      lipgloss.NewStyle().Foreground(tui.ColorSuccess).Italic(true),
		disabledMsg:      lipgloss.NewStyle().Foreground(tui.ColorWarning).Italic(true),
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

func (s *Selector) View() string {
	var lines []string

	dropdownStart, dropdownEnd := s.getDropdownBounds()

	scrollIndicatorStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	dropdownBorderStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate700)

	i := 0
	for i < len(s.options) {
		opt := s.options[i]

		if !opt.InDropdown {
			isSelected := i == s.selected
			isLast := i == len(s.options)-1
			showConnector := !isLast && !s.options[i+1].InDropdown
			optView := s.renderOption(opt, isSelected, showConnector)
			lines = append(lines, optView)
			i++
		} else {
			dropdownLines := s.renderDropdownRegion(dropdownStart, dropdownEnd, scrollIndicatorStyle, dropdownBorderStyle)
			lines = append(lines, dropdownLines...)

			i = dropdownEnd + 1
		}
	}

	return strings.Join(lines, "\n")
}

func (s *Selector) renderOption(opt Option, selected, showConnector bool) string {
	return s.renderOptionWithPrefix(opt, selected, showConnector, "")
}

func (s *Selector) renderOptionWithPrefix(opt Option, selected, showConnector bool, prefix string) string {
	var result []string

	styles := s.getOptionStyles()
	titleStyle := s.getTitleStyle(opt.Style)

	var bullet, title string
	if opt.Disabled {
		bullet = styles.bulletUnselected.Render("○")
		title = styles.titleDisabled.Render(opt.Title)
	} else if selected {
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

	if opt.Requirements != "" {
		result = append(result, prefix+styles.line.Render("  │ ")+styles.req.Render("Requires: "+opt.Requirements))
	}

	if opt.Disabled && opt.DisabledMsg != "" {
		result = append(result, prefix+styles.line.Render("  │ ")+styles.disabledMsg.Render(opt.DisabledMsg))
	}

	if showConnector {
		result = append(result, prefix+styles.line.Render("  │"))
	}

	return strings.Join(result, "\n")
}

type CompactSelector struct {
	options  []string
	selected int
	focused  bool
	width    int
}

func NewCompactSelector(options []string) *CompactSelector {
	return &CompactSelector{
		options:  options,
		selected: 0,
		focused:  true,
	}
}

func (s *CompactSelector) Len() int {
	return len(s.options)
}

func (s *CompactSelector) SetOptions(options []string) {
	s.options = options
	if s.selected >= len(options) {
		s.selected = len(options) - 1
	}
	if s.selected < 0 {
		s.selected = 0
	}
}

func (s *CompactSelector) Selected() string {
	if s.selected >= 0 && s.selected < len(s.options) {
		return s.options[s.selected]
	}
	return ""
}

func (s *CompactSelector) SelectedIndex() int {
	return s.selected
}

func (s *CompactSelector) SetSelected(index int) {
	if index >= 0 && index < len(s.options) {
		s.selected = index
	}
}

func (s *CompactSelector) SetFocused(focused bool) {
	s.focused = focused
}

func (s *CompactSelector) SetWidth(width int) {
	s.width = width
}

func (s *CompactSelector) Update(msg tea.Msg) (*CompactSelector, tea.Cmd) {
	if !s.focused {
		return s, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			s.selected--
			if s.selected < 0 {
				s.selected = len(s.options) - 1
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			s.selected++
			if s.selected >= len(s.options) {
				s.selected = 0
			}
		}
	}

	return s, nil
}

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

func (s *CompactSelector) ViewHorizontal() string {
	var parts []string

	selectedStyle := lipgloss.NewStyle().
		Foreground(tui.ColorSlate900).
		Background(tui.ColorPrimary).
		Padding(0, 1).
		Bold(true)

	unselectedStyle := lipgloss.NewStyle().
		Foreground(tui.ColorSlate400).
		Padding(0, 1)

	for i, opt := range s.options {
		if i == s.selected {
			parts = append(parts, selectedStyle.Render(opt))
		} else {
			parts = append(parts, unselectedStyle.Render(opt))
		}
	}

	return strings.Join(parts, " ")
}
