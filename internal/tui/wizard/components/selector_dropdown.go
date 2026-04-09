package components

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

const dropdownBorderWidth = 20

// moveUp moves selection up, skipping disabled options.
// Navigation is trapped within dropdown boundaries and won't wrap around.
func (s *Selector) moveUp() {
	if len(s.options) == 0 {
		return
	}

	original := s.selected
	wasInDropdown := s.options[s.selected].InDropdown

	if wasInDropdown {
		dropdownStart, _ := s.getDropdownBounds()

		if s.selected == dropdownStart {
			return
		}

		nextIndex := s.selected - 1
		if nextIndex < 0 || !s.options[nextIndex].InDropdown {
			return
		}
	}

	s.selected--

	if s.selected < 0 {
		s.selected = len(s.options) - 1
	}

	attempts := 0
	for s.options[s.selected].Disabled && attempts < len(s.options) {
		s.selected--
		if s.selected < 0 {
			s.selected = len(s.options) - 1
		}
		attempts++
	}
	if attempts >= len(s.options) {
		s.selected = original
	}

	nowInDropdown := s.options[s.selected].InDropdown
	if nowInDropdown {
		s.adjustDropdownScroll()
	} else if wasInDropdown && !nowInDropdown {
		s.dropdownScrollOffset = 0
	}
}

func (s *Selector) moveDown() {
	if len(s.options) == 0 {
		return
	}

	original := s.selected
	wasInDropdown := s.options[s.selected].InDropdown

	if wasInDropdown {
		_, dropdownEnd := s.getDropdownBounds()

		if s.selected == dropdownEnd {
			return
		}

		nextIndex := s.selected + 1
		if nextIndex >= len(s.options) || !s.options[nextIndex].InDropdown {
			return
		}
	}

	s.selected++

	if s.selected >= len(s.options) {
		s.selected = 0
	}

	attempts := 0
	for s.options[s.selected].Disabled && attempts < len(s.options) {
		s.selected++
		if s.selected >= len(s.options) {
			s.selected = 0
		}
		attempts++
	}
	if attempts >= len(s.options) {
		s.selected = original
	}

	nowInDropdown := s.options[s.selected].InDropdown
	if nowInDropdown {
		s.adjustDropdownScroll()
	} else if wasInDropdown && !nowInDropdown {
		s.dropdownScrollOffset = 0
	}
}

func (s *Selector) adjustDropdownScroll() {
	if len(s.options) == 0 || !s.options[s.selected].InDropdown {
		return
	}

	dropdownStart, dropdownEnd := s.getDropdownBounds()
	if dropdownStart < 0 {
		return
	}

	posInDropdown := s.selected - dropdownStart
	dropdownCount := dropdownEnd - dropdownStart + 1

	if posInDropdown < s.dropdownScrollOffset {
		s.dropdownScrollOffset = posInDropdown
	} else if posInDropdown >= s.dropdownScrollOffset+s.maxDropdownVisible {
		s.dropdownScrollOffset = posInDropdown - s.maxDropdownVisible + 1
	}

	maxOffset := dropdownCount - s.maxDropdownVisible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if s.dropdownScrollOffset > maxOffset {
		s.dropdownScrollOffset = maxOffset
	}
	if s.dropdownScrollOffset < 0 {
		s.dropdownScrollOffset = 0
	}
}

func (s *Selector) getDropdownBounds() (start, end int) {
	start = -1
	end = -1
	for i, opt := range s.options {
		if opt.InDropdown {
			if start < 0 {
				start = i
			}
			end = i
		}
	}
	return start, end
}

func (s *Selector) renderDropdownRegion(start, end int, scrollStyle, borderStyle lipgloss.Style) []string {
	var lines []string

	visibleStart := start + s.dropdownScrollOffset
	visibleEnd := visibleStart + s.maxDropdownVisible - 1
	if visibleEnd > end {
		visibleEnd = end
	}

	itemsAbove := s.dropdownScrollOffset
	itemsBelow := end - visibleEnd

	topBorder := "  ┌"
	if itemsAbove > 0 {
		topBorder += scrollStyle.Render(" ↑ " + strconv.Itoa(itemsAbove) + " more ")
	}
	topBorder += strings.Repeat("─", dropdownBorderWidth)
	lines = append(lines, borderStyle.Render(topBorder))

	dropdownPrefix := borderStyle.Render("  │ ")
	for i := visibleStart; i <= visibleEnd; i++ {
		opt := s.options[i]
		isSelected := i == s.selected
		isLast := i == visibleEnd
		optView := s.renderOptionWithPrefix(opt, isSelected, !isLast, dropdownPrefix)
		lines = append(lines, optView)
	}

	bottomBorder := "  └"
	if itemsBelow > 0 {
		bottomBorder += scrollStyle.Render(" ↓ " + strconv.Itoa(itemsBelow) + " more ")
	}
	bottomBorder += strings.Repeat("─", dropdownBorderWidth)
	lines = append(lines, borderStyle.Render(bottomBorder))

	return lines
}
