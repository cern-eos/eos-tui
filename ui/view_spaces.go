package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/lobis/eos-tui/eos"
)

func (m model) renderSpacesView(height int) string {
	if m.spaceStatusActive {
		return m.renderSpaceStatusView(height)
	}

	const fixedHeaderLines = 3 // title, blank, column headers
	naturalListContent := fixedHeaderLines + len(m.visibleSpaces())
	const spaceDetailLines = 8 // fixed lines rendered by renderSpaceDetails
	listHeight, detailHeight := adaptiveSplitHeights(height, naturalListContent, spaceDetailLines)
	width := m.panelWidth()

	list := m.renderSpacesList(width, listHeight)
	details := m.renderSpaceDetails(width, detailHeight)
	return lipgloss.JoinVertical(lipgloss.Left, list, details)
}

func (m model) renderSpacesList(width, height int) string {
	contentWidth := panelContentWidth(width)
	spaces := m.visibleSpaces()

	dataRows := make([][]string, len(spaces))
	for i, space := range spaces {
		dataRows[i] = []string{
			space.Name,
			space.Type,
			space.Status,
			fmt.Sprintf("%d", space.Groups),
			fmt.Sprintf("%d", space.NumFiles),
			fmt.Sprintf("%d", space.NumContainers),
			fmt.Sprintf("%.2f", usagePercent(space.UsedBytes, space.CapacityBytes)),
		}
	}
	columnDefs := contentAwareColumns([]tableColumn{
		{title: "name", min: 4, weight: 3},
		{title: "type", min: 4, weight: 1},
		{title: "status", min: 6, weight: 1},
		{title: "groups", min: 6, weight: 0, right: true},
		{title: "files", min: 5, weight: 0, right: true},
		{title: "dirs", min: 4, weight: 0, right: true},
		{title: "usage %", min: 7, weight: 0, right: true},
	}, dataRows)
	columns := allocateTableColumns(contentWidth, columnDefs)

	title := m.renderSectionTitle("EOS Spaces", contentWidth)
	lines := []string{
		title,
		"",
		m.renderSpaceHeaderRow(columns),
	}

	if m.spacesLoading && len(spaces) == 0 {
		lines = append(lines, "Loading spaces...")
	} else if m.spacesErr != nil && len(spaces) == 0 {
		lines = append(lines, m.styles.error.Render(m.spacesErr.Error()))
	} else if len(spaces) == 0 {
		lines = append(lines, "(no spaces)")
	} else {
		start, end := visibleWindow(len(spaces), m.spacesSelected, max(1, panelContentHeight(height)-len(lines)))
		lines[0] = renderInlineSuffix(title, renderScrollSummary(start, end, len(spaces)), contentWidth)
		for i := start; i < end; i++ {
			line := formatTableRow(columns, dataRows[i])
			if i == m.spacesSelected {
				line = m.styles.selected.Width(contentWidth).Render(line)
			}
			lines = append(lines, line)
		}
	}

	return m.styles.panel.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) renderSpaceDetails(width, height int) string {
	contentWidth := panelContentWidth(width)
	spaces := m.visibleSpaces()
	if len(spaces) == 0 || m.spacesSelected >= len(spaces) {
		return m.styles.panelDim.Width(width).Render(normalizePanelLines([]string{"No space selected"}, contentWidth, panelContentHeight(height)))
	}

	space := spaces[m.spacesSelected]

	lines := []string{
		m.renderSectionTitle("Selected Space", contentWidth),
		truncate(space.Name, max(10, width-4)),
		"",
		m.metricLine("Type", space.Type, "Status", space.Status),
		m.metricLine("Groups", fmt.Sprintf("%d", space.Groups), "Files", fmt.Sprintf("%d", space.NumFiles)),
		m.metricLine("Directories", fmt.Sprintf("%d", space.NumContainers), "", ""),
		m.metricLine("Capacity", humanBytes(space.CapacityBytes), "Used", humanBytes(space.UsedBytes)),
		m.metricLine("Free", humanBytes(space.FreeBytes), "", ""),
	}

	return m.styles.panelDim.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) renderSpaceHeaderRow(columns []tableColumn) string {
	return m.renderSelectableHeaderRow(columns, []string{"name", "type", "status", "groups", "files", "dirs", "usage %"}, m.spacesColumnSelected, m.spaceSort, m.spaceFilter)
}

func (m model) selectedSpace() (eos.SpaceRecord, bool) {
	spaces := m.visibleSpaces()
	if len(spaces) == 0 || m.spacesSelected < 0 || m.spacesSelected >= len(spaces) {
		return eos.SpaceRecord{}, false
	}
	return spaces[m.spacesSelected], true
}
