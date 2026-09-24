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

// dropdownBudget returns the effective visible-row budget: maxVisible when
// set, or the maxDropdownVisible floor for a Selector that never called
// SetDropdownBudget.
func (s *Selector) dropdownBudget() int {
	if s.maxVisible == 0 {
		return maxDropdownVisible
	}
	return s.maxVisible
}

// moveUp moves selection up by one option, wrapping to the last option from
// the first; a dropdown's first patch flows straight into the parent minor
// above it rather than trapping the cursor inside the dropdown.
func (s *Selector) moveUp() {
	if len(s.options) == 0 {
		return
	}

	wasInDropdown := s.options[s.selected].InDropdown

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

// moveDown moves selection down by one option, wrapping to the first option
// from the last; a dropdown's last patch flows straight into the next
// parent below it rather than trapping the cursor inside the dropdown.
func (s *Selector) moveDown() {
	if len(s.options) == 0 {
		return
	}

	wasInDropdown := s.options[s.selected].InDropdown

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

	s.clampDropdownOffset(dropdownStart, dropdownEnd, s.dropdownBudget())
}

// clampDropdownOffset re-clamps the scroll offset so the current selection
// (when it sits inside [start,end]) stays within the visible window, and
// the window itself stays within [0, itemCount-budget]; called on every
// render as well as every cursor move, since a budget that grows or shrinks
// between renders (a resize) can otherwise strand the offset either
// direction — past where a larger budget would show everything, or past
// the selection when a smaller budget no longer reaches it.
func (s *Selector) clampDropdownOffset(start, end, budget int) {
	if s.selected >= start && s.selected <= end {
		posInDropdown := s.selected - start
		if posInDropdown < s.dropdownScrollOffset {
			s.dropdownScrollOffset = posInDropdown
		} else if posInDropdown >= s.dropdownScrollOffset+budget {
			s.dropdownScrollOffset = posInDropdown - budget + 1
		}
	}

	maxOffset := max(end-start+1-budget, 0)
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

// renderDropdownRegion renders the dropdown's border and visible options,
// returning the index within lines of the selected option, or -1 when the
// selection sits outside the visible window.
func (s *Selector) renderDropdownRegion(start, end int, scrollStyle, borderStyle *lipgloss.Style) (lines []string, selectedRow int) {
	selectedRow = -1

	budget := s.dropdownBudget()
	s.clampDropdownOffset(start, end, budget)

	visibleStart := start + s.dropdownScrollOffset
	visibleEnd := min(visibleStart+budget-1, end)

	itemsAbove := s.dropdownScrollOffset
	itemsBelow := end - visibleEnd

	topBorder := "  ┌"
	if itemsAbove > 0 {
		topBorder += scrollStyle.Render(" ↑ " + strconv.Itoa(itemsAbove) + " more ")
	}
	topBorder += strings.Repeat("─", dropdownBorderWidth)
	lines = append(lines, borderStyle.Render(topBorder))

	dropdownPrefix := borderStyle.Render("  │ ")
	if s.DropdownHeader != "" {
		lines = append(lines, dropdownPrefix+"  "+s.DropdownHeader)
	}

	for i := visibleStart; i <= visibleEnd; i++ {
		opt := &s.options[i]
		isSelected := i == s.selected
		isLast := i == visibleEnd
		optView := s.renderOptionWithPrefix(opt, isSelected, !isLast, dropdownPrefix)
		if isSelected {
			selectedRow = len(lines)
		}
		lines = append(lines, optView)
	}

	bottomBorder := "  └"
	if itemsBelow > 0 {
		bottomBorder += scrollStyle.Render(" ↓ " + strconv.Itoa(itemsBelow) + " more ")
	}
	bottomBorder += strings.Repeat("─", dropdownBorderWidth)
	lines = append(lines, borderStyle.Render(bottomBorder))

	return lines, selectedRow
}
