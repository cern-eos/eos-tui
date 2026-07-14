package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lobis/eos-tui/eos"
)

func (m model) renderFSTView(height int) string {
	filterLines := 0
	if len(m.fstFilter.filters) > 0 {
		filterLines = 1
	}
	fixedHeaderLines := 3 + filterLines // title+controls, blank, col headers [, filters]
	naturalListContent := fixedHeaderLines + len(m.visibleFSTs())
	const fstDetailLines = 18 // fixed lines rendered by renderNodeDetails
	listHeight, detailHeight := adaptiveSplitHeights(height, naturalListContent, fstDetailLines)
	width := m.panelWidth()

	list := m.renderNodesList(width, listHeight)
	details := m.renderNodeDetails(width, detailHeight)
	return lipgloss.JoinVertical(lipgloss.Left, list, details)
}

func (m model) renderNodesList(width, height int) string {
	contentWidth := panelContentWidth(width)
	fsts := m.visibleFSTs()

	// Build data rows first so column widths can be fitted to content.
	dataRows := make([][]string, len(fsts))
	for i, node := range fsts {
		dataRows[i] = []string{
			node.Host,
			fmt.Sprintf("%d", node.Port),
			node.Geotag,
			node.Status,
			node.Activated,
			fmt.Sprintf("%d", node.HeartbeatDelta),
			fmt.Sprintf("%d", node.FileSystemCount),
			node.EOSVersion,
		}
	}
	columnDefs := contentAwareColumns([]tableColumn{
		{title: "host", min: 8, weight: 5},
		{title: "port", min: 5, weight: 0, right: true},
		{title: "geotag", min: 6, weight: 3},
		{title: "status", min: 6, weight: 0},
		{title: "activated", min: 9, weight: 0},
		{title: "heartbeatdelta", min: 14, weight: 0, right: true},
		{title: "nofs", min: 4, weight: 0, right: true},
		{title: "eos version", min: 11, weight: 0},
	}, dataRows)
	columns := allocateTableColumns(contentWidth, columnDefs)

	title := m.styles.section.Render("FST Nodes")
	lines := []string{
		title + m.renderNodeControls(),
		"",
		m.renderFstHeaderRow(columns),
	}
	if summary := m.renderFilterSummary(m.fstFilter.filters, func(col int) string {
		old := m.fstFilter.column
		m.fstFilter.column = col
		label := m.fstFilterColumnLabel()
		m.fstFilter.column = old
		return label
	}); summary != "" {
		lines = append(lines, summary)
	}

	if m.fstsLoading && len(fsts) == 0 {
		lines = append(lines, "Loading node list...")
	} else if m.fstsErr != nil && len(fsts) == 0 {
		lines = append(lines, m.styles.error.Render(m.fstsErr.Error()))
	} else if len(fsts) == 0 {
		lines = append(lines, "(no fsts)")
	} else {
		start, end := visibleWindow(len(fsts), m.fstSelected, max(1, panelContentHeight(height)-len(lines)))
		lines[0] = renderInlineSuffix(title+m.renderNodeControls(), renderScrollSummary(start, end, len(fsts)), contentWidth)
		for i := start; i < end; i++ {
			line := formatTableRow(columns, dataRows[i])
			if i == m.fstSelected {
				line = m.styles.selected.Width(contentWidth).Render(line)
			}
			lines = append(lines, line)
		}
	}

	return m.styles.panel.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) renderNodeDetails(width, height int) string {
	contentWidth := panelContentWidth(width)
	node, ok := m.selectedNode()
	if !ok {
		return m.styles.panelDim.Width(width).Render(normalizePanelLines([]string{"No node selected"}, contentWidth, panelContentHeight(height)))
	}

	lines := []string{
		m.renderSectionTitle("Selected Node", contentWidth),
		truncate(node.Host+":"+fmt.Sprintf("%d", node.Port), max(10, width-4)),
		"",
		m.metricLine("Type", fallback(node.Type, "-"), "EOS", fallback(node.EOSVersion, "-")),
		m.metricLine("Status", fallback(node.Status, "-"), "Activated", fallback(node.Activated, "-")),
		m.metricLine("Geotag", fallback(node.Geotag, "-"), "Filesystems", fmt.Sprintf("%d", node.FileSystemCount)),
		m.metricLine("Heartbeat", fmt.Sprintf("%ds", node.HeartbeatDelta), "Disk Load", fmt.Sprintf("%.2f", node.DiskLoad)),
		m.metricLine("Capacity", humanBytes(node.CapacityBytes), "Used", humanBytes(node.UsedBytes)),
		m.metricLine("Free", humanBytes(node.FreeBytes), "Files", fmt.Sprintf("%d", node.UsedFiles)),
		m.metricLine("RSS", humanBytes(node.RSSBytes), "VSize", humanBytes(node.VSizeBytes)),
		m.metricLine("Threads", fmt.Sprintf("%d", node.ThreadCount), "Read MB/s", fmt.Sprintf("%.2f", node.ReadRateMB)),
		m.metricLine("Write MB/s", fmt.Sprintf("%.2f", node.WriteRateMB), "", ""),
		"",
		m.renderSectionTitle("Uptime", contentWidth),
		truncate(strings.ReplaceAll(node.Uptime, "%20", " "), max(10, width-4)),
		"",
		m.renderSectionTitle("Kernel", contentWidth),
		truncate(node.Kernel, max(10, width-4)),
	}

	return m.styles.panelDim.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) selectedNode() (eos.FstRecord, bool) {
	fsts := m.visibleFSTs()
	if len(fsts) == 0 || m.fstSelected < 0 || m.fstSelected >= len(fsts) {
		return eos.FstRecord{}, false
	}

	return fsts[m.fstSelected], true
}

func nodeStatusDisplayCommand(host string, port int, status string) string {
	return eos.ShellDisplayJoin([]string{"eos", "node", "set", fmt.Sprintf("%s:%d", host, port), status})
}

func (m model) startNodeStatusToggleConfirm() (tea.Model, tea.Cmd) {
	node, ok := m.selectedNode()
	if !ok {
		m.status = "Select a node before changing its state"
		return m, nil
	}
	if node.Host == "" || node.Port == 0 {
		m.alert = errorAlert{
			active:  true,
			message: "Cannot change selected node state: host or port is missing",
		}
		return m, nil
	}

	currentState, targetState, ok := nodeStatusToggleTarget(node)
	if !ok {
		m.alert = errorAlert{
			active:  true,
			message: fmt.Sprintf("Cannot determine whether %s:%d is currently on or off", node.Host, node.Port),
		}
		return m, nil
	}

	m.nodeStatus = nodeStatusConfirm{
		active:  true,
		host:    node.Host,
		port:    node.Port,
		current: currentState,
		target:  targetState,
		command: nodeStatusDisplayCommand(node.Host, node.Port, targetState),
		button:  buttonCancel,
	}
	return m, nil
}

func nodeStatusToggleTarget(node eos.FstRecord) (current, target string, ok bool) {
	current = strings.TrimSpace(fallback(node.Activated, node.Status))
	normalized := strings.ToLower(current)
	switch normalized {
	case "on", "online", "enabled", "active":
		return current, "off", true
	case "off", "offline", "disabled", "inactive":
		return current, "on", true
	default:
		return current, "", false
	}
}

func (m model) renderNodeStatusConfirmPopup() string {
	cancelBtn := "[ Cancel ]"
	confirmBtn := fmt.Sprintf("[ Set %s ]", m.nodeStatus.target)

	if m.nodeStatus.button == buttonCancel {
		cancelBtn = m.styles.selected.Render(cancelBtn)
	} else {
		confirmBtn = m.styles.selected.Render(confirmBtn)
	}

	lines := []string{
		m.styles.popupTitle.Render("Confirm Node State Change"),
		fmt.Sprintf("Node:   %s", m.styles.value.Render(fmt.Sprintf("%s:%d", m.nodeStatus.host, m.nodeStatus.port))),
		fmt.Sprintf("Status: %s -> %s", m.nodeStatus.current, m.styles.value.Render(m.nodeStatus.target)),
		"",
		"The following command will be executed:",
		"",
		m.styles.value.Render(m.nodeStatus.command),
		"",
		lipgloss.JoinHorizontal(lipgloss.Left, cancelBtn, "  ", confirmBtn),
		"",
		m.styles.status.Render("g cancel  •  G confirm  •  enter apply  •  esc close"),
	}
	preferredWidth := max(48, min(110, m.contentWidth()-16))
	return m.renderModal(lines, lipgloss.Color("196"), preferredWidth)
}

func (m model) renderFstHeaderRow(columns []tableColumn) string {
	labels := []string{"host", "port", "geotag", "status", "activated", "heartbeatdelta", "nofs", "eos version"}
	return m.renderSelectableHeaderRow(columns, labels, m.fstColumnSelected, m.fstSort, m.fstFilter)
}

func (m model) renderNodeControls() string {
	return fmt.Sprintf("  [col:%s filters:%d current:%s]",
		m.fstSelectedColumnLabel(),
		len(m.fstFilter.filters),
		filterValueLabel(m.fstFilter.filters[m.fstColumnSelected], m.popup.active && m.popup.view == viewFST, m.popup.input.Value()),
	)
}
