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

func (s *Selector) dropdownBudget() int {
	if s.maxVisible == 0 {
		return maxDropdownVisible
	}
	return s.maxVisible
}

// moveUp wraps from the first option to the last; the dropdown's first
// patch flows into the parent above rather than trapping the cursor.
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

// moveDown wraps from the last option to the first; the dropdown's last
// patch flows into the next parent below.
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

// clampDropdownOffset keeps the selection inside the visible window and the
// window inside the list on every render, since a resized budget can
// strand the offset either way.
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

// renderDropdownRegion renders the dropdown border and visible options,
// returning the selected option's index within lines, or -1 when off-screen.
func (s *Selector) renderDropdownRegion(start, end int, scrollStyle, borderStyle *lipgloss.Style) (lines []string, selectedRow int) {
	selectedRow = -1

	budget := s.dropdownBudget()
	s.clampDropdownOffset(start, end, budget)

	visibleStart := start + s.dropdownScrollOffset
	visibleEnd := min(visibleStart+budget-1, end)

	itemsAbove := s.dropdownScrollOffset
	itemsBelow := end - visibleEnd

	dropdownPrefix := borderStyle.Render("  │ ")

	// The body renders before the borders so the border closes at the body's
	// widest row; dropdownBorderWidth is only the floor for a short body.
	var body []string
	if s.DropdownHeader != "" {
		body = append(body, dropdownPrefix+"  "+s.DropdownHeader)
	}
	for i := visibleStart; i <= visibleEnd; i++ {
		opt := &s.options[i]
		isSelected := i == s.selected
		isLast := i == visibleEnd
		optView := s.renderOptionWithPrefix(opt, isSelected, !isLast, dropdownPrefix)
		if isSelected {
			selectedRow = len(body)
		}
		body = append(body, optView)
	}

	bodyWidth := 0
	for _, l := range body {
		if w := lipgloss.Width(l); w > bodyWidth {
			bodyWidth = w
		}
	}
	// The prefix costs 4 columns the corner already accounts for; one gap
	// column keeps the widest row off the wall.
	inner := max(bodyWidth-4, dropdownBorderWidth) + 2
	boxWidth := inner + 4 // two-space lead, corner, dashes, corner

	topDashes := inner
	topBorder := "  ╭"
	if itemsAbove > 0 {
		hint := " ↑ " + strconv.Itoa(itemsAbove) + " more "
		topBorder += scrollStyle.Render(hint)
		topDashes = max(topDashes-lipgloss.Width(hint), 1)
	}
	topBorder += strings.Repeat("─", topDashes) + "╮"
	lines = append(lines, borderStyle.Render(topBorder))

	wall := borderStyle.Render("│")
	for _, block := range body {
		walled := strings.Split(block, "\n")
		for i, ln := range walled {
			pad := boxWidth - 1 - lipgloss.Width(ln)
			walled[i] = ln + strings.Repeat(" ", max(pad, 0)) + wall
		}
		lines = append(lines, strings.Join(walled, "\n"))
	}
	if selectedRow >= 0 {
		selectedRow++ // shift past the topBorder line just prepended
	}

	bottomDashes := inner
	bottomBorder := "  ╰"
	if itemsBelow > 0 {
		hint := " ↓ " + strconv.Itoa(itemsBelow) + " more "
		bottomBorder += scrollStyle.Render(hint)
		bottomDashes = max(bottomDashes-lipgloss.Width(hint), 1)
	}
	bottomBorder += strings.Repeat("─", bottomDashes) + "╯"
	lines = append(lines, borderStyle.Render(bottomBorder))

	return lines, selectedRow
}
