package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderVIDView(height int) string {
	naturalListContent := 3 + len(m.vidRecords)
	const vidDetailLines = 13
	listHeight, detailHeight := adaptiveSplitHeights(height, naturalListContent, vidDetailLines)

	width := m.panelWidth()
	list := m.renderVIDList(width, listHeight)
	details := m.renderVIDDetails(width, detailHeight)
	return lipgloss.JoinVertical(lipgloss.Left, list, details)
}

func (m model) renderVIDList(width, height int) string {
	contentWidth := panelContentWidth(width)
	dataRows := make([][]string, len(m.vidRecords))
	for i, record := range m.vidRecords {
		dataRows[i] = []string{record.Key, record.Value}
	}

	columns := allocateTableColumns(contentWidth, contentAwareColumns([]tableColumn{
		{title: "key", min: 28, maxw: 96, weight: 5},
		{title: "value", min: 10, weight: 2},
	}, dataRows))

	title := m.styles.label.Render("EOS VID  ") + m.renderVIDModeTabs()
	lines := []string{
		title,
		"",
		m.renderSimpleHeaderRow(columns, []string{"key", "value"}),
	}

	switch {
	case m.vidLoading && len(m.vidRecords) == 0:
		lines = append(lines, "Loading VID mappings...")
	case m.vidErr != nil && len(m.vidRecords) == 0:
		lines = append(lines, m.styles.error.Render(m.vidErr.Error()))
	case len(m.vidRecords) == 0:
		lines = append(lines, "(no entries)")
	default:
		start, end := visibleWindow(len(m.vidRecords), m.vidSelected, max(1, panelContentHeight(height)-len(lines)))
		lines[0] = renderInlineSuffix(title, renderScrollSummary(start, end, len(m.vidRecords)), contentWidth)
		for i := start; i < end; i++ {
			line := formatTableRow(columns, dataRows[i])
			if i == m.vidSelected {
				line = m.styles.selected.Width(contentWidth).Render(line)
			}
			lines = append(lines, line)
		}
	}

	return m.styles.panel.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) renderVIDDetails(width, height int) string {
	contentWidth := panelContentWidth(width)
	lines := []string{
		m.renderSectionTitle("VID Commands", contentWidth),
		m.metricLine("Scope", m.vidMode.label(), "List", m.vidMode.command()),
	}

	if record, ok := m.selectedVIDRecord(); ok {
		lines = append(lines,
			m.metricLine("Selected key", truncate(record.Key, 28), "Selected value", truncate(fallback(record.Value, "-"), 28)),
		)
	} else {
		lines = append(lines, m.metricLine("Selected key", "-", "Selected value", "-"))
	}

	lines = append(lines,
		m.renderSectionTitle("Mutation Syntax", contentWidth),
		truncate("eos vid set membership <uid> -uids|-gids [<list>]", contentWidth),
		truncate("eos vid set membership <uid> [+|-]sudo", contentWidth),
		truncate("eos vid set map -krb5|-gsi|-https|-sss|-unix|-voms|-grpc|-oauth2 <pattern> [vuid:<uid>] [vgid:<gid>]", contentWidth),
		truncate("eos vid set geotag <IP-prefix> <geotag>", contentWidth),
		truncate("eos vid rm <key> | eos vid rm membership <uid>", contentWidth),
		truncate("eos vid enable|disable krb5|gsi|sss|unix|https|grpc|oauth2|ztn", contentWidth),
		truncate("eos vid add|remove gateway <hostname> [prot]", contentWidth),
		truncate("eos vid publicaccesslevel <level>", contentWidth),
		truncate("eos vid tokensudo 0|1|2|3", contentWidth),
	)

	return m.styles.panelDim.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) renderVIDModeTabs() string {
	parts := make([]string, 0, len(orderedVIDModes))
	for _, mode := range orderedVIDModes {
		label := mode.label
		if m.vidMode == mode.mode {
			parts = append(parts, m.styles.tabActive.Render(label))
		} else {
			parts = append(parts, m.styles.tab.Render(label))
		}
	}
	return strings.Join(parts, " ")
}

func (m model) selectedVIDRecord() (record struct{ Key, Value string }, ok bool) {
	if len(m.vidRecords) == 0 || m.vidSelected < 0 || m.vidSelected >= len(m.vidRecords) {
		return record, false
	}
	selected := m.vidRecords[m.vidSelected]
	return struct{ Key, Value string }{Key: selected.Key, Value: selected.Value}, true
}

func (mode vidListMode) next(delta int) vidListMode {
	for i, tab := range orderedVIDModes {
		if tab.mode == mode {
			return orderedVIDModes[(i+delta+len(orderedVIDModes))%len(orderedVIDModes)].mode
		}
	}
	return vidListDefault
}

func (mode vidListMode) flag() string {
	for _, tab := range orderedVIDModes {
		if tab.mode == mode {
			return tab.flag
		}
	}
	return ""
}

func (mode vidListMode) label() string {
	for _, tab := range orderedVIDModes {
		if tab.mode == mode {
			return tab.label
		}
	}
	return "default"
}

func (mode vidListMode) command() string {
	flag := strings.TrimSpace(mode.flag())
	if flag == "" {
		return "eos vid ls"
	}
	return fmt.Sprintf("eos vid ls %s", flag)
}
