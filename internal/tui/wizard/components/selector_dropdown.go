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

	dropdownPrefix := borderStyle.Render("  │ ")

	// Render the body (header + visible rows) before either border line, so
	// the border can close on the right at the body's own widest row
	// instead of a fixed dash count that has no relation to the table it
	// brackets — dropdownBorderWidth (a 20-column stub) is only the floor
	// for a short/empty body, not a target.
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
	// "  │ " (the dropdownPrefix) costs 4 columns of bodyWidth that the
	// border's own "  ╭"/"  ╰" corner already accounts for; one extra gap
	// column keeps the widest row off the right wall, matching fieldBox's
	// one-column padding.
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

	// Each body entry is a block (title, description, connector rows);
	// every line inside it gets padded to the wall column.
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
