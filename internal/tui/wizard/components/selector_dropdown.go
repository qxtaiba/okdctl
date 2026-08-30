package components

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	dropdownBorderWidth = 20
	maxDropdownVisible  = 5
)

// moveUp moves selection up, trapped within dropdown boundaries (no wrap-around).
func (s *Selector) moveUp() {
	if len(s.options) == 0 {
		return
	}

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

	if s.options[s.selected].InDropdown {
		s.adjustDropdownScroll()
	} else if wasInDropdown {
		s.dropdownScrollOffset = 0
	}
}

func (s *Selector) moveDown() {
	if len(s.options) == 0 {
		return
	}

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

	if s.options[s.selected].InDropdown {
		s.adjustDropdownScroll()
	} else if wasInDropdown {
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
	} else if posInDropdown >= s.dropdownScrollOffset+maxDropdownVisible {
		s.dropdownScrollOffset = posInDropdown - maxDropdownVisible + 1
	}

	maxOffset := max(dropdownCount-maxDropdownVisible, 0)
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

func (s *Selector) renderDropdownRegion(start, end int, scrollStyle, borderStyle *lipgloss.Style) []string {
	var lines []string

	visibleStart := start + s.dropdownScrollOffset
	visibleEnd := min(visibleStart+maxDropdownVisible-1, end)

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
		opt := &s.options[i]
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
