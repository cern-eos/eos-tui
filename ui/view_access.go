package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lobis/eos-tui/eos"
)

func (m model) renderAccessView(height int) string {
	filterLines := 0
	if len(m.accessFilter.filters) > 0 {
		filterLines = 1
	}
	naturalListContent := 3 + filterLines + len(m.visibleAccessRecords())
	const accessDetailLines = 15
	listHeight, detailHeight := adaptiveSplitHeights(height, naturalListContent, accessDetailLines)

	width := m.panelWidth()
	list := m.renderAccessList(width, listHeight)
	details := m.renderAccessDetails(width, detailHeight)
	return lipgloss.JoinVertical(lipgloss.Left, list, details)
}

func (m model) renderAccessList(width, height int) string {
	contentWidth := panelContentWidth(width)
	records := m.visibleAccessRecords()
	dataRows := make([][]string, len(records))
	for i, record := range records {
		dataRows[i] = []string{record.Category, record.Rule, record.Value}
	}

	columns := allocateTableColumns(contentWidth, contentAwareColumns([]tableColumn{
		{title: "category", min: 10, maxw: 14, weight: 0},
		{title: "rule", min: 12, maxw: 40, weight: 2},
		{title: "value", min: 14, weight: 3},
	}, dataRows))

	title := m.styles.label.Render("EOS Access")
	lines := []string{
		title + m.renderAccessControls(),
		"",
		m.renderSelectableHeaderRow(columns, []string{"category", "rule", "value"}, m.accessColumnSelected, sortState{column: -1}, m.accessFilter),
	}
	if summary := m.renderFilterSummary(m.accessFilter.filters, m.accessFilterColumnLabelFor); summary != "" {
		lines = append(lines, summary)
	}

	switch {
	case m.accessLoading && len(records) == 0:
		lines = append(lines, "Loading access rules...")
	case m.accessErr != nil && len(records) == 0:
		lines = append(lines, m.styles.error.Render(m.accessErr.Error()))
	case len(records) == 0:
		lines = append(lines, "(no entries)")
	default:
		start, end := visibleWindow(len(records), m.accessSelected, max(1, panelContentHeight(height)-len(lines)))
		lines[0] = renderInlineSuffix(title+m.renderAccessControls(), renderScrollSummary(start, end, len(records)), contentWidth)
		for i := start; i < end; i++ {
			line := formatTableRow(columns, dataRows[i])
			if i == m.accessSelected {
				line = m.styles.selected.Width(contentWidth).Render(line)
			}
			lines = append(lines, line)
		}
	}

	return m.styles.panel.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) renderAccessDetails(width, height int) string {
	contentWidth := panelContentWidth(width)
	lines := []string{
		m.renderSectionTitle("Access Summary", contentWidth),
		m.metricLine("Allowed users", fmt.Sprintf("%d", m.accessCount("user", "allowed")), "Banned users", fmt.Sprintf("%d", m.accessCount("user", "banned"))),
		m.metricLine("Allowed groups", fmt.Sprintf("%d", m.accessCount("group", "allowed")), "Banned groups", fmt.Sprintf("%d", m.accessCount("group", "banned"))),
	}

	if record, ok := m.selectedAccessRecord(); ok {
		lines = append(lines,
			m.metricLine("Selected key", truncate(record.RawKey, 28), "Selected value", truncate(fallback(record.Value, "-"), 28)),
			m.metricLine("Actions", truncate(fallback(accessAvailableActionsLabel(record), "-"), 28), "Identifier", truncate(record.Value, 28)),
		)
	} else {
		lines = append(lines,
			m.metricLine("Selected key", "-", "Selected value", "-"),
			m.metricLine("Actions", "-", "Identifier", "-"),
		)
	}

	lines = append(lines,
		"",
		m.renderSectionTitle("Mutation Syntax", contentWidth),
		truncate("eos access ls -m", contentWidth),
		truncate("eos access allow user|group|host|domain <identifier>", contentWidth),
		truncate("eos access unallow user|group|host|domain <identifier>", contentWidth),
		truncate("eos access ban user|group|host|domain <identifier>", contentWidth),
		truncate("eos access unban user|group|host|domain <identifier>", contentWidth),
		truncate("eos access set redirect <target-host> [r|w|ENOENT|ENONET]", contentWidth),
		truncate("eos access set stall <stall-time> [r|w|ENOENT|ENONET]", contentWidth),
		truncate("eos access set limit <frequency> rate:{user,group}:{name}:<counter>", contentWidth),
		truncate("eos access rm redirect|stall|limit ...", contentWidth),
	)

	return m.styles.panelDim.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) selectedAccessRecord() (eos.AccessRecord, bool) {
	records := m.visibleAccessRecords()
	if len(records) == 0 || m.accessSelected < 0 || m.accessSelected >= len(records) {
		return eos.AccessRecord{}, false
	}
	return records[m.accessSelected], true
}

func (m model) accessCount(category, rule string) int {
	total := 0
	for _, record := range m.accessRecords {
		if record.Category == category && record.Rule == rule {
			total++
		}
	}
	return total
}

func (m model) renderAccessControls() string {
	return fmt.Sprintf("  [col:%s filters:%d current:%s]",
		m.accessFilterColumnLabel(),
		len(m.accessFilter.filters),
		filterValueLabel(m.accessFilter.filters[m.accessColumnSelected], m.popup.active && m.popup.view == viewAccess, m.popup.input.Value()),
	)
}

func (m model) accessFilterColumnLabelFor(col int) string {
	old := m.accessFilter.column
	m.accessFilter.column = col
	label := m.accessFilterColumnLabel()
	m.accessFilter.column = old
	return label
}

func (m model) startAccessActionPopup() (tea.Model, tea.Cmd) {
	record, ok := m.selectedAccessRecord()
	if !ok {
		m.status = "No access record selected"
		return m, nil
	}

	actions := accessActionsForRecord(record)
	if len(actions) == 0 {
		m.status = fmt.Sprintf("No direct actions for %s", record.RawKey)
		return m, nil
	}

	m.accessAction = accessActionPopup{
		active:      true,
		title:       "Access Actions",
		description: fmt.Sprintf("%s = %s", record.RawKey, fallback(record.Value, "-")),
		record:      record,
		actions:     actions,
	}
	return m, nil
}

func (m model) startAccessStallPopup() (tea.Model, tea.Cmd) {
	input := textinput.New()
	input.Placeholder = "stall seconds"
	input.SetValue("300")
	input.Focus()
	input.CursorEnd()

	m.accessAction = accessActionPopup{
		active:      true,
		title:       "Global Stall",
		description: "Set a global access stall. Edit the number of seconds below.",
		actions: []accessActionOption{
			{
				kind:    accessActionSetStall,
				label:   "Apply global stall",
				command: "eos access set stall <seconds>",
			},
		},
		input:      input,
		focusInput: true,
	}
	return m, nil
}

func (m model) renderAccessActionPopup() string {
	lines := []string{
		m.styles.popupTitle.Render(m.accessAction.title),
		m.accessAction.description,
		"",
	}
	if m.accessAction.focusInput {
		lines = append(lines,
			m.accessAction.input.View(),
			"",
			m.styles.status.Render("enter apply  •  esc cancel"),
		)

		return m.renderModal(lines, lipgloss.Color("196"), 0)
	}
	for i, action := range m.accessAction.actions {
		line := action.label
		if action.command != "" {
			line += "  " + m.styles.status.Render(action.command)
		}
		if i == m.accessAction.selected {
			lines = append(lines, m.styles.selected.Render("▶ "+line))
		} else {
			lines = append(lines, "  "+line)
		}
	}
	lines = append(lines, "", m.styles.status.Render("↑↓ select  •  enter apply  •  esc cancel"))

	return m.renderModal(lines, lipgloss.Color("196"), 0)
}

func accessActionsForRecord(record eos.AccessRecord) []accessActionOption {
	category := strings.ToLower(strings.TrimSpace(record.Category))
	value := strings.TrimSpace(record.Value)
	if value == "" {
		return nil
	}

	switch category {
	case "user", "group", "host", "domain":
	default:
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(record.Rule)) {
	case "allowed":
		return []accessActionOption{
			{kind: accessActionUnallow, label: "Unallow selected identity", command: fmt.Sprintf("eos access unallow %s %s", category, value)},
			{kind: accessActionBan, label: "Ban selected identity", command: fmt.Sprintf("eos access ban %s %s", category, value)},
		}
	case "banned":
		return []accessActionOption{
			{kind: accessActionUnban, label: "Unban selected identity", command: fmt.Sprintf("eos access unban %s %s", category, value)},
			{kind: accessActionAllow, label: "Allow selected identity", command: fmt.Sprintf("eos access allow %s %s", category, value)},
		}
	default:
		return nil
	}
}

func accessAvailableActionsLabel(record eos.AccessRecord) string {
	actions := accessActionsForRecord(record)
	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		labels = append(labels, action.label)
	}
	return strings.Join(labels, " / ")
}

func accessActionVerb(kind accessActionKind) string {
	switch kind {
	case accessActionAllow:
		return "allow"
	case accessActionUnallow:
		return "unallow"
	case accessActionBan:
		return "ban"
	case accessActionUnban:
		return "unban"
	default:
		return ""
	}
}
