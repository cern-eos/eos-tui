package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var splashEOS = []string{
	"███████╗ ██████╗ ███████╗",
	"██╔════╝██╔═══██╗██╔════╝",
	"█████╗  ██║   ██║███████╗",
	"██╔══╝  ██║   ██║╚════██║",
	"███████╗╚██████╔╝███████║",
	"╚══════╝ ╚═════╝ ╚══════╝",
}

var splashTUI = []string{
	"████████╗██╗   ██╗██╗",
	"╚══██╔══╝██║   ██║██║",
	"   ██║   ██║   ██║██║",
	"   ██║   ██║   ██║██║",
	"   ██║   ╚██████╔╝██║",
	"   ╚═╝    ╚═════╝ ╚═╝",
}

func (m model) renderHeader() string {
	maxWidth := m.contentWidth()
	parts := []string{m.styles.header.Render("EOS TUI"), "  "}
	for i, t := range orderedViewTabs {
		if i > 0 {
			parts = append(parts, " ")
		}
		if m.activeView == t.view {
			parts = append(parts, m.styles.tabActive.Render(t.label))
		} else {
			parts = append(parts, m.styles.tab.Render(t.label))
		}
	}

	left := lipgloss.JoinHorizontal(lipgloss.Left, parts...)
	rightLabel := m.styles.label.Render("target ")
	maxLeftWidth := max(0, maxWidth-1-lipgloss.Width(rightLabel))
	if lipgloss.Width(left) > maxLeftWidth {
		left = padVisibleWidth(left, maxLeftWidth)
	}
	availableRightValue := max(0, maxWidth-lipgloss.Width(left)-1-lipgloss.Width(rightLabel))
	right := rightLabel + m.styles.value.Render(truncate(m.endpoint, availableRightValue))
	spacerWidth := max(1, maxWidth-lipgloss.Width(left)-lipgloss.Width(right))

	return padVisibleWidth(lipgloss.JoinHorizontal(lipgloss.Left, left, strings.Repeat(" ", spacerWidth), right), maxWidth)
}

func (m model) renderFooter() string {
	if m.log.active {
		filter := ""
		if m.log.filter != "" {
			filter = fmt.Sprintf("  •  filter: %q", m.log.filter)
		}
		tailHint := "t tail off"
		if !m.log.tailing {
			tailHint = "t tail on"
		}
		wrapHint := "w wrap on"
		if m.log.wrap {
			wrapHint = "w wrap off"
		}
		keys := fmt.Sprintf("↑↓/jk scroll  •  g top  •  G bottom  •  / filter  •  f plain  •  %s  •  %s  •  r reload  •  esc/ctrl+c close%s", wrapHint, tailHint, filter)
		if m.log.plain {
			keys = fmt.Sprintf("↑↓/jk scroll  •  g top  •  G bottom  •  / filter  •  f boxed  •  %s  •  %s  •  r reload  •  esc/ctrl+c close%s", wrapHint, tailHint, filter)
		}
		if m.log.filtering {
			keys = "type to filter  •  enter apply  •  esc cancel  •  ctrl+c close"
		}
		if len(m.log.logSources) > 1 && !m.log.filtering {
			keys = strings.Replace(keys, "r reload", "n/p log source  •  r reload", 1)
		}
		return m.styles.status.Render(padVisibleWidth(keys, m.contentWidth()))
	}

	hostViews := m.activeView == viewMGM || m.activeView == viewQDB ||
		m.activeView == viewFST || m.activeView == viewFileSystems
	var keys string
	switch m.activeView {
	case viewMGM, viewQDB:
		keys = "tab/0-9  •  ↑↓/jk  •  g/G top/bottom  •  c coup  •  r refresh  •  l logs  •  L commands  •  s shell  •  q quit"
	case viewNamespaceStats:
		keys = "tab/0-9  •  ↑↓/jk sections/rows  •  ←→ pane/col  •  / filter col  •  g/G top/bottom  •  r refresh  •  L commands  •  q quit"
	case viewNamespace:
		keys = "tab/0-9  •  ↑↓/jk  •  g/G top/bottom  •  → open  •  m mkdir  •  : goto  •  enter/a attrs  •  backspace back  •  L commands  •  q quit"
	case viewSpaces:
		if m.spaceStatusActive {
			keys = "tab/0-9  •  ↑↓/jk  •  enter edit  •  esc/backspace/← back  •  r refresh  •  L commands  •  q quit"
		} else {
			keys = "tab/0-9  •  ↑↓/jk  •  enter open  •  r refresh  •  L commands  •  q quit"
		}
	case viewIOShaping:
		keys = "tab/0-9  •  ↑↓/jk  •  a/u/g/n/p mode  •  m limits  •  N new  •  enter edit  •  d del  •  r  •  L commands"
	case viewGroups:
		keys = "tab/0-9  •  ↑↓/jk  •  ←→  •  S  •  /  •  enter status  •  A all status  •  r  •  L commands"
	case viewVID:
		keys = "tab/0-9  •  ↑↓/jk  •  ←→ scope  •  g/G top/bottom  •  r refresh  •  L commands  •  q quit"
	case viewAccess:
		keys = "tab/0-9  •  ↑↓/jk  •  ←→ col  •  / filter  •  c clear  •  enter action  •  s stall prompt  •  r refresh  •  L commands  •  q quit"
	case viewFST:
		keys = "tab/0-9  •  ↑↓/jk  •  ←→  •  S sort  •  / filter  •  c clear  •  o on/off  •  r  •  l logs  •  s shell  •  L commands"
	case viewFileSystems:
		keys = "tab/0-9  •  ↑↓/jk  •  ←→  •  S  •  /  •  enter cfg  •  A all cfg  •  x apollon  •  l logs  •  L commands  •  s shell"
	default:
		keys = "tab/0-9  •  ↑↓/jk  •  ←→ col  •  S sort  •  / filter  •  L commands  •  q quit"
		if hostViews {
			keys = "tab/0-9  •  ↑↓/jk  •  ←→ col  •  S sort  •  / filter  •  r refresh  •  l logs  •  L commands  •  s shell  •  q quit"
		}
	}

	return m.styles.status.Render(padVisibleWidth(keys, m.contentWidth()))
}

func (m model) renderBody(availableHeight int) string {
	switch m.activeView {
	case viewMGM, viewQDB:
		return m.renderMGMView(availableHeight)
	case viewFST:
		return m.renderFSTView(availableHeight)
	case viewFileSystems:
		return m.renderFileSystemsView(availableHeight)
	case viewNamespace:
		return m.renderNamespaceView(availableHeight)
	case viewSpaces:
		return m.renderSpacesView(availableHeight)
	case viewNamespaceStats:
		return m.renderNamespaceStatsView(availableHeight)
	case viewSpaceStatus:
		return m.renderSpaceStatusView(availableHeight)
	case viewIOShaping:
		return m.renderIOShapingView(availableHeight)
	case viewGroups:
		return m.renderGroupsView(availableHeight)
	case viewVID:
		return m.renderVIDView(availableHeight)
	case viewAccess:
		return m.renderAccessView(availableHeight)
	default:
		return ""
	}
}

func (m model) renderOverlay(body string, popup string, height int) string {
	bodyLines := strings.Split(body, "\n")
	popupLines := strings.Split(popup, "\n")
	width := m.contentWidth()

	for len(bodyLines) < height {
		bodyLines = append(bodyLines, strings.Repeat(" ", width))
	}

	popupHeight := len(popupLines)
	popupWidth := 0
	for _, line := range popupLines {
		popupWidth = max(popupWidth, lipgloss.Width(line))
	}
	popupWidth = min(popupWidth, width)
	topPad := max(0, (height-popupHeight)/2)
	leftPad := max(0, (width-popupWidth)/2)

	for i := 0; i < popupHeight && topPad+i < len(bodyLines); i++ {
		bodyLine := padVisibleWidth(bodyLines[topPad+i], width)
		popupLine := padVisibleWidth(popupLines[i], popupWidth)
		left := ansi.Cut(bodyLine, 0, leftPad)
		right := ansi.Cut(bodyLine, leftPad+popupWidth, width)
		bodyLines[topPad+i] = left + popupLine + right
	}

	if len(bodyLines) > height {
		bodyLines = bodyLines[:height]
	}
	return strings.Join(bodyLines, "\n")
}

func (m model) renderStartupSplash(height int) string {
	base := m.normalizeRenderedBlock("", height)
	loaderFrames := []string{
		"[=     ]",
		"[==    ]",
		"[===   ]",
		"[ ===  ]",
		"[  === ]",
		"[   ===]",
	}
	loader := loaderFrames[m.splash.frame%len(loaderFrames)]
	titleStyle := m.styles.splash
	if m.splash.frame%2 == 1 {
		titleStyle = m.styles.splash.Foreground(lipgloss.Color("159"))
	}

	lines := []string{}
	for _, line := range splashEOS {
		lines = append(lines, titleStyle.Render(line))
	}
	lines = append(lines, "")
	for _, line := range splashTUI {
		lines = append(lines, titleStyle.Render(line))
	}
	lines = append(lines, "")
	lines = append(lines, m.styles.splashDim.Render("initializing cluster view"))
	lines = append(lines, m.styles.status.Render(loader))

	box := m.styles.splashBox.Render(lipgloss.JoinVertical(lipgloss.Center, lines...))
	return m.renderOverlay(base, box, height)
}

func (m model) normalizeRenderedBlock(block string, height int) string {
	if height <= 0 {
		return ""
	}

	lines := strings.Split(block, "\n")
	width := m.contentWidth()
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = padVisibleWidth(lines[i], width)
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

func (m model) splitMainAndCommandHeights(total int) (mainHeight, commandHeight int) {
	if !m.commandLog.active {
		return total, 0
	}

	commandHeight = min(11, max(6, total/3))
	if total-commandHeight < 4 {
		commandHeight = total - 4
	}
	if commandHeight < 4 || total-commandHeight < 4 {
		return total, 0
	}
	return total - commandHeight, commandHeight
}

func (m model) metricLine(leftLabel, leftValue, rightLabel, rightValue string) string {
	left := m.styles.label.Render(leftLabel+" ") + m.styles.value.Render(leftValue)
	if rightLabel == "" {
		return left
	}

	right := m.styles.label.Render(rightLabel+" ") + m.styles.value.Render(rightValue)
	return fmt.Sprintf("%-42s %s", left, right)
}

func (m model) renderSectionTitle(title string, width int) string {
	titleText := m.styles.section.Render(title)
	if width <= 0 {
		return titleText
	}

	remaining := width - lipgloss.Width(titleText) - 1
	if remaining <= 0 {
		return titleText
	}

	return titleText + " " + m.styles.sectionRule.Render(strings.Repeat("─", remaining))
}

func (m model) contentWidth() int {
	return max(20, m.width-2)
}

func (m model) panelWidth() int {
	return max(18, m.contentWidth()-2)
}

func (m model) renderSimpleHeaderRow(columns []tableColumn, labels []string) string {
	cells := make([]string, len(columns))
	for i, col := range columns {
		label := ""
		if i < len(labels) {
			label = labels[i]
		}
		var cell string
		if col.right {
			cell = padLeft(label, col.min)
		} else {
			cell = padRight(label, col.min)
		}
		cells[i] = m.styles.label.Render(cell)
	}
	return strings.Join(cells, " ")
}

func (m model) renderSelectableHeaderRow(columns []tableColumn, labels []string, selected int, sortState sortState, filterState filterState) string {
	cells := make([]string, 0, len(columns))
	for i, column := range columns {
		label := ""
		if i < len(labels) {
			label = labels[i]
		}
		if sortState.column == i {
			if sortState.desc {
				label += "↓"
			} else {
				label += "↑"
			}
		}
		if filterState.filters[i] != "" {
			label += "*"
		}
		if i == selected {
			label = "[" + label + "]"
		}
		cell := padRight(label, column.min)
		if i == selected {
			cell = m.styles.selected.Render(cell)
		} else {
			cell = m.styles.label.Render(cell)
		}
		cells = append(cells, cell)
	}
	return strings.Join(cells, " ")
}

// renderFilterSummary returns a line showing all active filters (for display
// below the column header row).  labelFn maps column index → label string.
func (m model) renderFilterSummary(filters map[int]string, labelFn func(int) string) string {
	cols := make([]int, 0, len(filters))
	for col, v := range filters {
		if v != "" {
			cols = append(cols, col)
		}
	}
	if len(cols) == 0 {
		return ""
	}
	sort.Ints(cols)
	parts := make([]string, 0, len(cols))
	for _, col := range cols {
		parts = append(parts, m.styles.label.Render(labelFn(col)+"=")+m.styles.value.Render(filters[col]))
	}
	return m.styles.label.Render("active filters: ") + strings.Join(parts, m.styles.status.Render("  •  "))
}

func (m model) renderFilterPopup() string {
	title := "Filter " + m.activeFilterColumnLabel()
	if m.popup.view == viewFileSystems {
		title = "Filter " + m.fsFilterColumnLabel()
	}

	contentWidth := min(80, max(40, m.contentWidth()-8))
	inputView := m.popup.input.View()
	tableView := m.popup.table.View()
	hint := m.styles.status.Render("Enter apply selected value • Esc cancel")

	box := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.popupTitle.Render(title),
		"",
		inputView,
		"",
		tableView,
		"",
		hint,
	)

	return m.styles.panelDim.Width(contentWidth).Render(box)
}

func (m model) renderCommandPanel(height int) string {
	width := max(18, m.contentWidth()-2)
	innerWidth := max(1, width-4)
	innerHeight := max(1, height-2)

	title := m.styles.label.Render("Recent commands")
	if m.commandLog.filePath != "" {
		title += m.styles.status.Render("  " + m.commandLog.filePath)
	}

	lines := []string{padVisibleWidth(title, innerWidth)}
	entrySlots := max(0, innerHeight-1)

	var entries []string
	switch {
	case m.commandLog.loading:
		entries = []string{m.styles.status.Render("Loading command history...")}
	case m.commandLog.err != nil:
		entries = []string{m.styles.error.Render(m.commandLog.err.Error())}
	case len(m.commandLog.lines) == 0:
		entries = []string{m.styles.status.Render("No commands recorded yet.")}
	default:
		entries = make([]string, len(m.commandLog.lines))
		for i, line := range m.commandLog.lines {
			entries[i] = m.styles.value.Render(line)
		}
	}

	if len(entries) > entrySlots {
		entries = entries[len(entries)-entrySlots:]
	}
	for _, line := range entries {
		lines = append(lines, padVisibleWidth(line, innerWidth))
	}
	for len(lines) < innerHeight {
		lines = append(lines, strings.Repeat(" ", innerWidth))
	}

	return m.styles.panelDim.Width(width).Render(strings.Join(lines, "\n"))
}

func padVisibleWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := lipgloss.Width(s)
	if w >= width {
		return ansi.Cut(s, 0, width)
	}
	return s + strings.Repeat(" ", width-w)
}

func filterValueLabel(current string, active bool, input string) string {
	if active {
		return fmt.Sprintf("%q*", input)
	}
	if current == "" {
		return "\"\""
	}
	return fmt.Sprintf("%q", current)
}
