package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lobis/eos-tui/eos"
)

type topologyHostKind int

const (
	topologyHostMGM topologyHostKind = iota
	topologyHostQDB
)

type topologyHostRow struct {
	kind    topologyHostKind
	host    string
	port    int
	role    string
	status  string
	version string
}

// renderMGMView shows the unified topology as two stacked tables:
// management nodes first, then the matching QDB hosts.
func (m model) renderMGMView(height int) string {
	width := m.panelWidth()
	contentWidth := panelContentWidth(width)
	mgmRows := m.topologyMGMRows()
	qdbRows := m.topologyQDBRows()

	title := m.styles.label.Render("management & quarkdb topology")
	lines := []string{title, ""}

	if m.mgmsLoading && len(m.mgms) == 0 {
		lines = append(lines, "loading management and quarkdb info...")
	} else if m.mgmsErr != nil && len(m.mgms) == 0 {
		lines = append(lines, m.styles.error.Render(m.mgmsErr.Error()))
	} else if len(mgmRows) == 0 && len(qdbRows) == 0 {
		lines = append(lines, "(no management nodes found)")
	} else {
		availableRows := max(2, panelContentHeight(height)-7)
		topRows := max(1, availableRows/2)
		bottomRows := max(1, availableRows-topRows)
		selectedKind, selectedLocal, _ := m.selectedTopologyHostRow()

		lines = append(lines, m.renderSectionTitle("Management Nodes (MGM)", contentWidth))
		lines = append(lines, m.renderTopologyRows(contentWidth, topRows, mgmRows, selectedKind, selectedLocal, topologyHostMGM)...)
		lines = append(lines, "")
		lines = append(lines, m.renderSectionTitle("QuarkDB Hosts (QDB)", contentWidth))
		lines = append(lines, m.renderTopologyRows(contentWidth, bottomRows, qdbRows, selectedKind, selectedLocal, topologyHostQDB)...)
	}

	return m.styles.panel.Width(width).Render(normalizePanelLines(lines, contentWidth, height))
}

func (m model) topologyMGMRows() []topologyHostRow {
	rows := make([]topologyHostRow, 0, len(m.mgms))
	for _, node := range m.mgms {
		if node.Host == "" {
			continue
		}
		rows = append(rows, topologyHostRow{
			kind:    topologyHostMGM,
			host:    node.Host,
			port:    node.Port,
			role:    strings.ToLower(node.Role),
			status:  strings.ToLower(node.Status),
			version: fallback(node.EOSVersion, "-"),
		})
	}
	sortTopologyRows(rows)
	return rows
}

func (m model) topologyQDBRows() []topologyHostRow {
	rows := make([]topologyHostRow, 0, len(m.mgms))
	for _, node := range m.mgms {
		if node.QDBHost == "" {
			continue
		}
		rows = append(rows, topologyHostRow{
			kind:    topologyHostQDB,
			host:    node.QDBHost,
			port:    node.QDBPort,
			role:    strings.ToLower(fallback(node.QDBRole, node.Role)),
			status:  strings.ToLower(fallback(node.QDBStatus, node.Status)),
			version: fallback(node.QDBVersion, "-"),
		})
	}
	sortTopologyRows(rows)
	return rows
}

func sortTopologyRows(rows []topologyHostRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].role != rows[j].role {
			return rows[i].role == "leader"
		}
		if rows[i].host != rows[j].host {
			return rows[i].host < rows[j].host
		}
		return rows[i].port < rows[j].port
	})
}

func (m model) topologySelectableRows() []topologyHostRow {
	rows := append([]topologyHostRow{}, m.topologyMGMRows()...)
	rows = append(rows, m.topologyQDBRows()...)
	return rows
}

func (m model) selectedTopologyHostRow() (topologyHostKind, int, bool) {
	rows := m.topologySelectableRows()
	if m.mgmSelected < 0 || m.mgmSelected >= len(rows) {
		return topologyHostMGM, -1, false
	}
	selected := rows[m.mgmSelected]
	local := 0
	switch selected.kind {
	case topologyHostMGM:
		for i, row := range m.topologyMGMRows() {
			if row.host == selected.host && row.port == selected.port {
				local = i
				break
			}
		}
	case topologyHostQDB:
		for i, row := range m.topologyQDBRows() {
			if row.host == selected.host && row.port == selected.port {
				local = i
				break
			}
		}
	}
	return selected.kind, local, true
}

func (m model) selectedTopologyHost() (topologyHostRow, bool) {
	rows := m.topologySelectableRows()
	if m.mgmSelected < 0 || m.mgmSelected >= len(rows) {
		return topologyHostRow{}, false
	}
	return rows[m.mgmSelected], true
}

func qdbCoupRemoteArgs() []string {
	return eos.QDBCoupArgs()
}

func qdbCoupDisplayCommand(client *eos.Client, host string) string {
	remoteArgs := qdbCoupRemoteArgs()
	if client == nil {
		return eos.ShellDisplayJoin(remoteArgs)
	}

	target, jump := client.SSHTargetForHost(host)
	if target == "" {
		return eos.ShellDisplayJoin(remoteArgs)
	}

	args := append([]string{"ssh"}, client.SSHArgs(true)...)
	if jump != "" {
		args = append(args, "-J", jump)
	}
	args = append(args, target)
	args = append(args, remoteArgs...)
	return eos.ShellDisplayJoin(args)
}

func (m model) startQDBCoupConfirm() (tea.Model, tea.Cmd) {
	row, ok := m.selectedTopologyHost()
	if !ok {
		m.status = "Select a QDB host before attempting coup"
		return m, nil
	}
	if row.kind != topologyHostQDB {
		m.status = "Coup can only be attempted on a selected QDB host"
		return m, nil
	}

	m.qdbCoup = qdbCoupConfirm{
		active:  true,
		host:    row.host,
		command: qdbCoupDisplayCommand(m.client, row.host),
		button:  buttonCancel,
	}
	return m, nil
}

func (m model) renderQDBCoupConfirmPopup() string {
	cancelBtn := "[ Cancel ]"
	confirmBtn := "[ Attempt QDB coup ]"

	if m.qdbCoup.button == buttonCancel {
		cancelBtn = m.styles.selected.Render(cancelBtn)
	} else {
		confirmBtn = m.styles.selected.Render(confirmBtn)
	}

	popupWidth := m.modalContentWidth(max(48, min(120, m.contentWidth()-16)))
	commandLines := wrappedQDBPopupLines(m.qdbCoup.command, popupWidth)
	lines := []string{
		m.styles.popupTitle.Render("Confirm QDB Raft Coup"),
		fmt.Sprintf("Host: %s", m.styles.value.Render(m.qdbCoup.host)),
		"",
		"This asks the selected QuarkDB node to become raft leader.",
		"It does not change the EOS MGM leader.",
		"",
		"The following command will be executed:",
		"",
	}
	for _, line := range commandLines {
		lines = append(lines, m.styles.value.Render(line))
	}
	lines = append(lines,
		"",
		lipgloss.JoinHorizontal(lipgloss.Left, cancelBtn, "  ", confirmBtn),
		"",
		m.styles.status.Render("g cancel  •  G confirm  •  enter apply  •  esc close"),
	)
	return m.renderModal(lines, lipgloss.Color("196"), popupWidth)
}

func (m model) renderQDBCoupResultPopup() string {
	title := "QDB Raft Coup Result"
	statusLine := "Command completed successfully."
	borderColor := lipgloss.Color("86")
	if m.qdbCoupDone.err != nil {
		statusLine = fmt.Sprintf("Command failed: %v", m.qdbCoupDone.err)
		borderColor = lipgloss.Color("196")
	}

	output := strings.TrimSpace(m.qdbCoupDone.output)
	if output == "" {
		output = "(no output)"
	}
	popupWidth := m.modalContentWidth(max(48, min(120, m.contentWidth()-16)))
	outputLines := wrappedQDBPopupLines(output, popupWidth)

	lines := []string{
		m.styles.popupTitle.Render(title),
		fmt.Sprintf("Host: %s", m.styles.value.Render(m.qdbCoupDone.host)),
		"",
		statusLine,
		"",
		"Command output:",
		"",
	}
	for _, line := range outputLines {
		lines = append(lines, m.styles.value.Render(line))
	}
	lines = append(lines,
		"",
		m.styles.status.Render("enter / esc close"),
	)
	return m.renderModal(lines, borderColor, popupWidth)
}

func wrappedQDBPopupLines(text string, width int) []string {
	lines := sanitizeLogLines(strings.Split(text, "\n"))
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}
		wrapped := ansi.Hardwrap(line, width, true)
		for _, part := range strings.Split(wrapped, "\n") {
			out = append(out, ansi.Truncate(part, width, "…"))
		}
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func (m model) renderTopologyRows(contentWidth, maxRows int, rows []topologyHostRow, selectedKind topologyHostKind, selectedLocal int, sectionKind topologyHostKind) []string {
	dataRows := make([][]string, len(rows))
	for i, row := range rows {
		dataRows[i] = []string{
			row.host,
			fmt.Sprintf("%d", row.port),
			row.role,
			row.status,
			row.version,
		}
	}

	columns := allocateTableColumns(contentWidth, contentAwareColumns([]tableColumn{
		{title: "host", min: 15, weight: 1},
		{title: "port", min: 4, weight: 0, right: true},
		{title: "role", min: 10, weight: 0},
		{title: "status", min: 10, weight: 0},
		{title: "version", min: 10, weight: 0},
	}, dataRows))

	lines := []string{
		m.renderSimpleHeaderRow(columns, []string{"host", "port", "role", "status", "version"}),
	}

	sectionSelected := -1
	if selectedKind == sectionKind {
		sectionSelected = selectedLocal
	}
	if len(rows) == 0 {
		return append(lines, "(none)")
	}

	for _, idx := range visibleTableIndices(len(rows), sectionSelected, maxRows) {
		row := formatTableRow(columns, dataRows[idx])
		if idx == sectionSelected {
			row = m.styles.selected.Width(contentWidth).Render(row)
		}
		lines = append(lines, row)
	}

	return lines
}

func visibleTableIndices(total, selected, maxRows int) []int {
	if total <= 0 || maxRows <= 0 {
		return nil
	}
	if total <= maxRows {
		out := make([]int, total)
		for i := 0; i < total; i++ {
			out[i] = i
		}
		return out
	}

	if selected < 0 {
		selected = 0
	}
	start := max(0, selected-maxRows/2)
	if start+maxRows > total {
		start = total - maxRows
	}
	end := min(total, start+maxRows)

	out := make([]int, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, i)
	}
	return out
}
