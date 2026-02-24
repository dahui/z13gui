package gui

// focus.go — 2D grid gamepad focus navigation with modal slider editing.
//
// Items are arranged in a grid (row + col). D-pad up/down moves between rows
// (preserving column where possible), left/right moves within a row. A activates
// buttons/switches or enters edit mode for sliders (D-pad left/right adjusts,
// A commits, B cancels). Shoulder buttons jump between sections.
//
// Per-view focus lists: main, theme, and color views each have their own item
// list. swapFocusList switches between them on view change.

import (
	"log/slog"
	"sort"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// focusItem represents a single gamepad-navigable element.
type focusItem struct {
	widget     gtk.Widgetter // widget to highlight with .gamepad-focus
	row        int           // visual row number
	col        int           // column within row
	section    string        // section name for shoulder-button jumping
	isVisible  func() bool   // false if parent section is hidden; nil = always visible
	onActivate func()        // A button: toggle/activate (non-editable items)
	editable   bool          // true for sliders — A enters edit mode instead of activating
	onLeft     func()        // D-pad left while editing: decrease value
	onRight    func()        // D-pad right while editing: increase value
	getValue   func() float64 // read current value (for cancel/restore)
	setValue   func(float64)  // restore value on cancel
}

// visible returns true if this item should be navigable.
func (fi *focusItem) visible() bool {
	if fi.isVisible != nil && !fi.isVisible() {
		return false
	}
	return gtk.BaseWidget(fi.widget).IsVisible()
}

// visibleRows returns sorted unique row numbers that have at least one visible item.
func (w *Window) visibleRows() []int {
	seen := make(map[int]bool)
	var rows []int
	for i := range w.focusItems {
		fi := &w.focusItems[i]
		if fi.visible() && !seen[fi.row] {
			seen[fi.row] = true
			rows = append(rows, fi.row)
		}
	}
	sort.Ints(rows)
	return rows
}

// moveVertical moves focus to the nearest visible item in the next (dir=+1) or
// previous (dir=-1) row. Preserves column position where possible.
func (w *Window) moveVertical(dir int) {
	if len(w.focusItems) == 0 {
		return
	}
	rows := w.visibleRows()
	if len(rows) == 0 {
		return
	}
	current := w.focusItems[w.focusIdx]
	targetCol := current.col

	// Find current row's position in visible rows.
	curPos := -1
	for i, r := range rows {
		if r == current.row {
			curPos = i
			break
		}
	}
	if curPos == -1 {
		return
	}

	// Step to next/prev row (wrapping).
	nextPos := (curPos + dir + len(rows)) % len(rows)
	if nextPos == curPos {
		return // only one visible row
	}
	targetRow := rows[nextPos]

	// Find the item in targetRow with the closest column to targetCol.
	best := -1
	bestDist := 1<<31 - 1
	for i := range w.focusItems {
		fi := &w.focusItems[i]
		if fi.row == targetRow && fi.visible() {
			d := targetCol - fi.col
			if d < 0 {
				d = -d
			}
			if d < bestDist {
				bestDist = d
				best = i
			}
		}
	}
	if best >= 0 {
		w.setFocusIdx(best)
	}
}

// moveHorizontal moves focus to the next (dir=+1) or previous (dir=-1) visible
// item within the same row. Wraps at row edges.
func (w *Window) moveHorizontal(dir int) {
	if len(w.focusItems) == 0 {
		return
	}
	current := w.focusItems[w.focusIdx]

	// Collect visible items in the same row, sorted by column.
	var rowItems []int
	for i := range w.focusItems {
		fi := &w.focusItems[i]
		if fi.row == current.row && fi.visible() {
			rowItems = append(rowItems, i)
		}
	}
	if len(rowItems) <= 1 {
		return // single-item row, no horizontal movement
	}
	sort.Slice(rowItems, func(a, b int) bool {
		return w.focusItems[rowItems[a]].col < w.focusItems[rowItems[b]].col
	})

	// Find current position within the row.
	pos := -1
	for i, idx := range rowItems {
		if idx == w.focusIdx {
			pos = i
			break
		}
	}
	if pos == -1 {
		return
	}

	next := (pos + dir + len(rowItems)) % len(rowItems)
	w.setFocusIdx(rowItems[next])
}

// jumpSection jumps to the first visible item of the next (dir=+1) or
// previous (dir=-1) section.
func (w *Window) jumpSection(dir int) {
	if len(w.focusItems) == 0 {
		return
	}
	current := w.focusItems[w.focusIdx].section

	// Collect sections in row order.
	var sections []string
	seen := make(map[string]bool)
	rows := w.visibleRows()
	for _, r := range rows {
		for i := range w.focusItems {
			fi := &w.focusItems[i]
			if fi.row == r && fi.visible() && !seen[fi.section] {
				seen[fi.section] = true
				sections = append(sections, fi.section)
			}
		}
	}
	if len(sections) <= 1 {
		return
	}

	// Find current section position.
	curPos := -1
	for i, s := range sections {
		if s == current {
			curPos = i
			break
		}
	}
	if curPos == -1 {
		return
	}

	// Step to next/prev section (wrapping).
	nextPos := (curPos + dir + len(sections)) % len(sections)
	target := sections[nextPos]

	// Find first visible item in the target section.
	for i := range w.focusItems {
		fi := &w.focusItems[i]
		if fi.section == target && fi.visible() {
			w.setFocusIdx(i)
			return
		}
	}
}

// activateOrEdit handles the A button: if the focused item is editable (slider),
// enters edit mode. Otherwise calls onActivate directly.
func (w *Window) activateOrEdit() {
	if w.focusIdx >= len(w.focusItems) {
		return
	}
	fi := w.focusItems[w.focusIdx]
	if fi.editable {
		w.enterEditMode()
	} else if fi.onActivate != nil {
		fi.onActivate()
	}
}

// enterEditMode starts modal slider editing on the focused item.
func (w *Window) enterEditMode() {
	if w.focusIdx >= len(w.focusItems) {
		return
	}
	fi := w.focusItems[w.focusIdx]
	if fi.getValue != nil {
		w.editOriginalValue = fi.getValue()
	}
	w.focusEditing = true
	gtk.BaseWidget(fi.widget).AddCSSClass("gamepad-editing")
	slog.Debug("gamepad: edit mode entered", "section", fi.section)
}

// exitEditMode leaves slider editing. If commit is false, restores the
// original value before editing started.
func (w *Window) exitEditMode(commit bool) {
	if !w.focusEditing {
		return
	}
	w.focusEditing = false
	if w.focusIdx < len(w.focusItems) {
		fi := w.focusItems[w.focusIdx]
		gtk.BaseWidget(fi.widget).RemoveCSSClass("gamepad-editing")
		if !commit && fi.setValue != nil {
			fi.setValue(w.editOriginalValue)
		}
	}
	slog.Debug("gamepad: edit mode exited", "commit", commit)
}

// adjustFocus calls OnLeft (dir=-1) or OnRight (dir=+1) on the focused item.
func (w *Window) adjustFocus(dir int) {
	if w.focusIdx >= len(w.focusItems) {
		return
	}
	fi := w.focusItems[w.focusIdx]
	if dir < 0 && fi.onLeft != nil {
		fi.onLeft()
	} else if dir > 0 && fi.onRight != nil {
		fi.onRight()
	}
}

// setFocusIdx updates the focus index and moves the CSS highlight.
func (w *Window) setFocusIdx(idx int) {
	if w.focusIdx < len(w.focusItems) {
		gtk.BaseWidget(w.focusItems[w.focusIdx].widget).RemoveCSSClass("gamepad-focus")
	}
	w.focusIdx = idx
	if idx < len(w.focusItems) {
		gtk.BaseWidget(w.focusItems[idx].widget).AddCSSClass("gamepad-focus")
		slog.Debug("gamepad: focus", "idx", idx,
			"row", w.focusItems[idx].row, "col", w.focusItems[idx].col,
			"section", w.focusItems[idx].section)
	}
}

// showGamepadFocus enables the gamepad focus indicator.
func (w *Window) showGamepadFocus() {
	if w.gamepadActive {
		return
	}
	w.gamepadActive = true
	for i := range w.focusItems {
		if w.focusItems[i].visible() {
			w.setFocusIdx(i)
			return
		}
	}
}

// hideGamepadFocus removes the gamepad focus indicator (e.g. on mouse movement).
func (w *Window) hideGamepadFocus() {
	if !w.gamepadActive {
		return
	}
	if w.focusEditing {
		w.exitEditMode(true)
	}
	w.gamepadActive = false
	if w.focusIdx < len(w.focusItems) {
		gtk.BaseWidget(w.focusItems[w.focusIdx].widget).RemoveCSSClass("gamepad-focus")
	}
}

// swapFocusList switches to a different focus item list (e.g. on view change).
// Clears the current highlight and resets focus to the first visible item.
func (w *Window) swapFocusList(items []focusItem) {
	if w.focusIdx < len(w.focusItems) {
		gtk.BaseWidget(w.focusItems[w.focusIdx].widget).RemoveCSSClass("gamepad-focus")
		gtk.BaseWidget(w.focusItems[w.focusIdx].widget).RemoveCSSClass("gamepad-editing")
	}
	w.focusEditing = false
	w.focusItems = items
	w.focusIdx = 0
	if w.gamepadActive {
		for i := range w.focusItems {
			if w.focusItems[i].visible() {
				w.setFocusIdx(i)
				return
			}
		}
	}
}

// scaleAdjust returns onLeft/onRight/getValue/setValue functions for a slider.
func scaleAdjust(sc *gtk.Scale, step float64) (onLeft, onRight func(), getValue func() float64, setValue func(float64)) {
	adj := sc.Adjustment()
	onLeft = func() {
		v := adj.Value() - step
		if v < adj.Lower() {
			v = adj.Lower()
		}
		adj.SetValue(v)
	}
	onRight = func() {
		v := adj.Value() + step
		if v > adj.Upper() {
			v = adj.Upper()
		}
		adj.SetValue(v)
	}
	getValue = func() float64 { return adj.Value() }
	setValue = func(v float64) { adj.SetValue(v) }
	return
}
