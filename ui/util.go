package ui

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lobis/eos-tui/eos"
)

func cleanPath(rawPath string) string {
	if rawPath == "" {
		return "/"
	}

	if !strings.HasPrefix(rawPath, "/") {
		return "/" + rawPath
	}

	return path.Clean(rawPath)
}

func parentPath(current string) string {
	if current == "/" {
		return "/"
	}

	return path.Dir(current)
}

// resolveNamespacePath turns user-entered text into an absolute, cleaned
// EOS namespace path. Absolute inputs (starting with "/") replace the
// current path; relative inputs are joined onto current. Empty input is
// treated as "stay where you are".
func resolveNamespacePath(current, input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return cleanPath(current)
	}
	if strings.HasPrefix(trimmed, "/") {
		return cleanPath(trimmed)
	}
	base := current
	if base == "" {
		base = "/"
	}
	return cleanPath(path.Join(base, trimmed))
}

func fallback(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}

	return value
}

func humanBytes(value uint64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}

	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	size := float64(value)
	unit := -1
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}

	if unit < 0 {
		return fmt.Sprintf("%d B", value)
	}

	return fmt.Sprintf("%.1f %s", size, units[unit])
}

func formatDuration(value time.Duration) string {
	if value <= 0 {
		return "-"
	}

	return value.Round(time.Second).String()
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}

	return value.Local().Format(time.RFC3339)
}

func formatTimeShort(value time.Time) string {
	if value.IsZero() {
		return "-"
	}

	return value.Local().Format("2006-01-02 15:04")
}

func truncate(value string, width int) string {
	// Ensure we don't have newlines which would break the 1-line layout.
	value = strings.ReplaceAll(value, "\n", " ")

	if width <= 0 || lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}

	runes := []rune(value)
	if len(runes) >= width {
		return string(runes[:width-1]) + "…"
	}

	return value
}

func padRight(value string, width int) string {
	value = truncate(value, width)
	return value + strings.Repeat(" ", max(0, width-lipgloss.Width(value)))
}

func padLeft(value string, width int) string {
	value = truncate(value, width)
	return strings.Repeat(" ", max(0, width-lipgloss.Width(value))) + value
}

func formatTableRow(columns []tableColumn, values []string) string {
	parts := make([]string, len(columns))
	for i, column := range columns {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		if column.right {
			parts[i] = padLeft(value, column.min)
		} else {
			parts[i] = padRight(value, column.min)
		}
	}
	return strings.Join(parts, " ")
}

func allocateTableColumns(width int, columns []tableColumn) []tableColumn {
	if len(columns) == 0 {
		return nil
	}

	available := max(len(columns), width-(len(columns)-1))
	allocated := make([]tableColumn, len(columns))
	copy(allocated, columns)

	totalMin := 0
	totalWeight := 0
	for i := range allocated {
		allocated[i].min = max(1, max(allocated[i].min, lipgloss.Width(allocated[i].title)))
		if allocated[i].maxw > 0 {
			allocated[i].min = min(allocated[i].min, allocated[i].maxw)
		}
		totalMin += allocated[i].min
		// Columns already at their max don't participate in extra-space distribution.
		if allocated[i].maxw > 0 && allocated[i].min >= allocated[i].maxw {
			allocated[i].weight = 0
		}
		totalWeight += max(allocated[i].weight, 0)
	}

	if totalMin > available {
		overflow := totalMin - available
		for overflow > 0 {
			changed := false
			for i := range allocated {
				if overflow == 0 {
					break
				}
				if allocated[i].min > 1 {
					allocated[i].min--
					overflow--
					changed = true
				}
			}
			if !changed {
				break
			}
		}
		return allocated
	}

	extra := available - totalMin
	if totalWeight == 0 {
		totalWeight = len(allocated)
		for i := range allocated {
			if allocated[i].maxw == 0 || allocated[i].min < allocated[i].maxw {
				allocated[i].weight = 1
			}
		}
	}

	for i := range allocated {
		if extra == 0 {
			break
		}
		share := (extra * max(allocated[i].weight, 0)) / totalWeight
		if allocated[i].maxw > 0 {
			share = min(share, max(0, allocated[i].maxw-allocated[i].min))
		}
		allocated[i].min += share
		extra -= share
		totalWeight -= max(allocated[i].weight, 0)
	}

	for i := range allocated {
		if extra == 0 {
			break
		}
		if allocated[i].maxw == 0 || allocated[i].min < allocated[i].maxw {
			allocated[i].min++
			extra--
		}
	}

	return allocated
}

func contentAwareColumns(columns []tableColumn, rows [][]string) []tableColumn {
	result := make([]tableColumn, len(columns))
	copy(result, columns)
	for i := range result {
		w := lipgloss.Width(result[i].title)
		for _, row := range rows {
			if i < len(row) {
				if cw := lipgloss.Width(row[i]); cw > w {
					w = cw
				}
			}
		}
		if result[i].maxw > 0 && w > result[i].maxw {
			w = result[i].maxw
		}
		if w > result[i].min {
			result[i].min = w
		}
	}
	return result
}

func visibleWindow(total, selected, capacity int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if capacity <= 0 || total <= capacity {
		return 0, total
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= total {
		selected = total - 1
	}

	start := selected - capacity/2
	if start < 0 {
		start = 0
	}
	maxStart := total - capacity
	if start > maxStart {
		start = maxStart
	}
	return start, min(total, start+capacity)
}

func renderScrollSummary(start, end, total int) string {
	if total <= 0 || end-start >= total {
		return ""
	}

	return fmt.Sprintf("  [%d-%d/%d]", start+1, end, total)
}

func panelContentWidth(width int) int {
	return max(1, width-4)
}

func panelContentHeight(height int) int {
	return max(1, height-2)
}

func fitLines(lines []string, height int) string {
	if height <= 0 {
		return ""
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func normalizePanelLines(lines []string, contentWidth, contentHeight int) string {
	if contentHeight <= 0 {
		return ""
	}

	// Treat embedded newlines as physical rows before applying the height
	// budget. This keeps a multi-line error from pushing the panel's bottom
	// border out of the viewport. Every row is then cut and padded to the exact
	// inner width so Lip Gloss never wraps it while adding the border.
	rows := strings.Split(strings.Join(lines, "\n"), "\n")
	if len(rows) > contentHeight {
		rows = rows[:contentHeight]
	}
	for len(rows) < contentHeight {
		rows = append(rows, "")
	}
	for i, row := range rows {
		rows[i] = padVisibleWidth(row, contentWidth)
	}
	return strings.Join(rows, "\n")
}

func splitViewHeights(total int) (int, int) {
	if total <= 3 {
		return max(1, total-1), 1
	}

	available := max(2, total-1)
	listHeight := max(1, (available*2)/3)
	detailHeight := max(1, available-listHeight)

	if detailHeight < 4 {
		shift := min(listHeight-1, 4-detailHeight)
		listHeight -= shift
		detailHeight += shift
	}
	if listHeight < 4 {
		shift := min(detailHeight-1, 4-listHeight)
		detailHeight -= shift
		listHeight += shift
	}

	if listHeight+detailHeight > available {
		overflow := listHeight + detailHeight - available
		if listHeight >= detailHeight {
			listHeight = max(1, listHeight-overflow)
		} else {
			detailHeight = max(1, detailHeight-overflow)
		}
	}

	return listHeight, detailHeight
}

// adaptiveSplitHeights is like splitViewHeights but shrinks the list panel when
// its content is smaller than the default 2/3 allocation, giving the surplus to
// the detail panel.  naturalListContent is the number of content lines the list
// actually needs (excluding the 2-line border).
//
// The two panel borders together account for 4 lines (2 each), but because the
// body must fill height+2 rendered lines (to offset the -2 in View's bodyHeight
// formula), the net target is height+2.
func adaptiveSplitHeights(height, naturalListContent, naturalDetailContent int) (int, int) {
	target := height + 2
	naturalList := naturalListContent + 2
	naturalDetail := naturalDetailContent + 2

	// Constants for minimum usable heights.
	const minList = 6

	// Case 1: Everything fits. Expand the list to fill the target height
	// so the details box stays at its natural size at the bottom.
	if naturalList+naturalDetail <= target {
		return target - naturalDetail, naturalDetail
	}

	// Case 2: Space is tight. Prioritize the details box (bottom) natural height.
	// The list (top) should be shortened first.
	detailHeight := naturalDetail
	if target-detailHeight < minList {
		detailHeight = target - minList
	}

	listHeight := target - detailHeight

	// Final clamp to ensure nothing is below the absolute minimum of 4.
	listH := max(4, listHeight)
	return listH, target - listH
}

func usagePercent(used, capacity uint64) float64 {
	if capacity == 0 {
		return 0
	}

	return (float64(used) / float64(capacity)) * 100
}

func max(a, b int) int {
	if a > b {
		return a
	}

	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}

	return b
}

func clampIndex(index, length int) int {
	if length <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func max64(a int64, b int64) int64 {
	if a > b {
		return a
	}

	return b
}

func entryTypeLabel(entry eos.Entry) string {
	if entry.Kind == eos.EntryKindContainer {
		return "DIR"
	}

	return "FILE"
}

func entrySize(entry eos.Entry) string {
	if entry.Kind == eos.EntryKindContainer {
		return "-"
	}

	return humanBytes(entry.Size)
}
