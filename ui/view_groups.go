package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lobis/eos-tui/eos"
)

func (m model) renderGroupsView(height int) string {
	naturalListContent := 3 + len(m.visibleGroups())
	const groupDetailContent = 6
	listHeight, detailHeight := adaptiveSplitHeights(height, naturalListContent, groupDetailContent)

	width := m.panelWidth()
	list := m.renderGroupsList(width, listHeight)
	details := m.renderGroupDetails(width, detailHeight)

	return lipgloss.JoinVertical(lipgloss.Left, list, details)
}

func (m model) renderGroupsList(width, height int) string {
	contentWidth := panelContentWidth(width)

	groups := m.visibleGroups()
	dataRows := make([][]string, len(groups))
	for i, g := range groups {
		dataRows[i] = []string{
			g.Name,
			g.Status,
			fmt.Sprintf("%d", g.NoFS),
			humanBytes(g.CapacityBytes),
			humanBytes(g.UsedBytes),
			humanBytes(g.FreeBytes),
			fmt.Sprintf("%d", g.NumFiles),
		}
	}

	columnDefs := contentAwareColumns([]tableColumn{
		{title: "name", min: 10, weight: 3},
		{title: "status", min: 6, weight: 1},
		{title: "nofs", min: 4, weight: 0, right: true},
		{title: "capacity", min: 8, weight: 0, right: true},
		{title: "used", min: 8, weight: 0, right: true},
		{title: "free", min: 8, weight: 0, right: true},
		{title: "files", min: 5, weight: 0, right: true},
	}, dataRows)

	columns := allocateTableColumns(contentWidth, columnDefs)

	title := m.renderSectionTitle("EOS Groups", contentWidth)
	lines := []string{
		title,
		"",
		m.renderGroupHeaderRow(columns),
	}

	if m.groupsLoading && len(groups) == 0 {
		lines = append(lines, "Loading groups...")
	} else if m.groupsErr != nil && len(groups) == 0 {
		lines = append(lines, m.styles.error.Render(m.groupsErr.Error()))
	} else if len(groups) == 0 {
		lines = append(lines, "(no groups)")
	} else {
		start, end := visibleWindow(len(groups), m.groupsSelected, max(1, panelContentHeight(height)-len(lines)))
		lines[0] = renderInlineSuffix(title, renderScrollSummary(start, end, len(groups)), contentWidth)
		for i := start; i < end; i++ {
			g := groups[i]
			row := []string{
				g.Name,
				g.Status,
				fmt.Sprintf("%d", g.NoFS),
				humanBytes(g.CapacityBytes),
				humanBytes(g.UsedBytes),
				humanBytes(g.FreeBytes),
				fmt.Sprintf("%d", g.NumFiles),
			}
			line := formatTableRow(columns, row)
			if i == m.groupsSelected {
				line = m.styles.selected.Width(contentWidth).Render(line)
			}
			lines = append(lines, line)
		}
	}

	return m.styles.panel.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) renderGroupHeaderRow(columns []tableColumn) string {
	labels := []string{"name", "status", "nofs", "capacity", "used", "free", "files"}
	return m.renderSelectableHeaderRow(columns, labels, m.groupsColumnSelected, m.groupSort, m.groupFilter)
}

func (m model) renderGroupDetails(width, height int) string {
	groups := m.visibleGroups()
	if len(groups) == 0 || m.groupsSelected < 0 || m.groupsSelected >= len(groups) {
		return m.styles.panelDim.Width(width).Render(normalizePanelLines([]string{"no group selected"}, panelContentWidth(width), panelContentHeight(height)))
	}

	g := groups[m.groupsSelected]
	lines := []string{
		m.renderSectionTitle("Selected Group", panelContentWidth(width)),
		g.Name,
		"",
		m.metricLine("Status", g.Status, "Filesystems", fmt.Sprintf("%d", g.NoFS)),
		m.metricLine("Capacity", humanBytes(g.CapacityBytes), "Used", humanBytes(g.UsedBytes)),
		m.metricLine("Free", humanBytes(g.FreeBytes), "Files", fmt.Sprintf("%d", g.NumFiles)),
	}

	return m.styles.panelDim.Width(width).Render(normalizePanelLines(lines, panelContentWidth(width), panelContentHeight(height)))
}

func (m model) selectedGroup() (eos.GroupRecord, bool) {
	groups := m.visibleGroups()
	if len(groups) == 0 || m.groupsSelected < 0 || m.groupsSelected >= len(groups) {
		return eos.GroupRecord{}, false
	}
	return groups[m.groupsSelected], true
}

func (m model) startGroupDrainConfirm() (tea.Model, tea.Cmd) {
	group, ok := m.selectedGroup()
	if !ok {
		return m, nil
	}

	sel := 0
	for i, opt := range groupStatusOptions {
		if group.Status == opt {
			sel = i
			break
		}
	}

	m.groupDrain = groupDrainConfirm{
		active:   true,
		group:    group.Name,
		current:  group.Status,
		selected: sel,
		button:   buttonCancel,
	}
	return m, nil
}

func (m model) startGroupStatusEditAll() (tea.Model, tea.Cmd) {
	groups := m.visibleGroups()
	if len(groups) == 0 {
		return m, nil
	}

	targets := make([]string, 0, len(groups))
	for _, group := range groups {
		targets = append(targets, group.Name)
	}

	m.groupDrain = groupDrainConfirm{
		active:   true,
		selected: 0,
		applyAll: true,
		targets:  targets,
		button:   buttonCancel,
	}
	return m, nil
}

func (m model) renderGroupDrainConfirmPopup() string {
	if m.groupDrain.applyAll && m.groupDrain.confirm {
		cancelBtn := "[ Cancel ]"
		confirmBtn := "[ Confirm ]"
		if m.groupDrain.button == buttonCancel {
			cancelBtn = m.styles.selected.Render(cancelBtn)
		} else {
			confirmBtn = m.styles.selected.Render(confirmBtn)
		}

		chosen := groupStatusOptions[m.groupDrain.selected]
		lines := []string{
			m.styles.popupTitle.Render("Confirm bulk group status"),
			fmt.Sprintf("This will run against %d filtered groups.", len(m.groupDrain.targets)),
			"",
			m.styles.value.Render(fmt.Sprintf("status=%s", chosen)),
			"",
			lipgloss.JoinHorizontal(lipgloss.Left, cancelBtn, "  ", confirmBtn),
			"",
			m.styles.status.Render("g cancel  •  G confirm  •  enter apply  •  esc cancel"),
		}

		return m.renderModal(lines, lipgloss.Color("196"), 0)
	}

	lines := []string{
		m.styles.popupTitle.Render("Set group status"),
	}
	if m.groupDrain.applyAll {
		lines = append(lines,
			fmt.Sprintf("Targets: %d filtered groups", len(m.groupDrain.targets)),
			"Choose a status to apply to all visible groups.",
		)
	} else {
		lines = append(lines,
			fmt.Sprintf("Group: %s", m.styles.value.Render(m.groupDrain.group)),
			fmt.Sprintf("Current: %s", m.styles.value.Render(fallback(m.groupDrain.current, "-"))),
		)
	}
	lines = append(lines, "")
	for i, opt := range groupStatusOptions {
		if i == m.groupDrain.selected {
			lines = append(lines, m.styles.selected.Render("▶ "+opt))
		} else {
			lines = append(lines, "  "+opt)
		}
	}
	hint := "↑↓ select  •  g/G home/end  •  enter apply  •  esc cancel"
	if m.groupDrain.applyAll {
		hint = "↑↓ select  •  g/G home/end  •  enter continue  •  esc cancel"
	}
	lines = append(lines, "", m.styles.status.Render(hint))

	return m.renderModal(lines, lipgloss.Color("196"), 0)
}
