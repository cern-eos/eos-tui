package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lobis/eos-tui/eos"
)

const apollonSSHTarget = "root@eosops.cern.ch"
const apollonCLIPath = "/root/repair/apollon/apollon-cli"

func (m model) renderFileSystemsView(height int) string {
	filterLines := 0
	if len(m.fsFilter.filters) > 0 {
		filterLines = 1
	}
	fixedHeaderLines := 3 + filterLines // title+controls, blank, col headers [, filters]
	naturalListContent := fixedHeaderLines + len(m.visibleFileSystems())
	const fsDetailLines = 14 // fixed lines rendered by renderFileSystemDetails
	listHeight, detailHeight := adaptiveSplitHeights(height, naturalListContent, fsDetailLines)
	width := m.panelWidth()

	list := m.renderFileSystemsList(width, listHeight)
	details := m.renderFileSystemDetails(width, detailHeight)
	return lipgloss.JoinVertical(lipgloss.Left, list, details)
}

func (m model) renderFileSystemsList(width, height int) string {
	contentWidth := panelContentWidth(width)
	fileSystems := m.visibleFileSystems()

	// Pre-build data rows so column widths can be fitted to actual content.
	dataRows := make([][]string, len(fileSystems))
	for i, fs := range fileSystems {
		dataRows[i] = []string{
			fs.Host,
			fmt.Sprintf("%d", fs.Port),
			fmt.Sprintf("%d", fs.ID),
			fs.Path,
			fs.SchedGroup,
			fs.Geotag,
			fs.Boot,
			fs.ConfigStatus,
			fs.DrainStatus,
			fmt.Sprintf("%.2f", usagePercent(fs.UsedBytes, fs.CapacityBytes)),
			fs.Active,
			fs.Health,
		}
	}
	columnDefs := contentAwareColumns([]tableColumn{
		{title: "host", min: 4, weight: 4},
		{title: "port", min: 4, weight: 0, right: true},
		{title: "id", min: 2, weight: 0, right: true},
		{title: "path", min: 4, maxw: 28, weight: 3},
		{title: "schedgroup", min: 10, weight: 1},
		{title: "geotag", min: 6, weight: 1},
		{title: "boot", min: 4, weight: 0},
		{title: "configstatus", min: 12, weight: 0},
		{title: "drain", min: 5, weight: 0},
		{title: "usage %", min: 7, weight: 0, right: true},
		{title: "active", min: 6, weight: 0},
		{title: "health", min: 4, weight: 1},
	}, dataRows)
	columns := allocateTableColumns(contentWidth, columnDefs)

	title := m.styles.section.Render("EOS Filesystems")
	lines := []string{
		title + m.renderFileSystemControls(),
		"",
		m.renderFileSystemHeaderRow(columns),
	}
	if summary := m.renderFilterSummary(m.fsFilter.filters, func(col int) string {
		old := m.fsFilter.column
		m.fsFilter.column = col
		label := m.fsFilterColumnLabel()
		m.fsFilter.column = old
		return label
	}); summary != "" {
		lines = append(lines, summary)
	}

	if m.fileSystemsLoading && len(fileSystems) == 0 {
		lines = append(lines, "Loading filesystem state...")
	} else if m.fileSystemsErr != nil && len(fileSystems) == 0 {
		lines = append(lines, m.styles.error.Render(m.fileSystemsErr.Error()))
	} else if len(fileSystems) == 0 {
		lines = append(lines, "(no filesystems)")
	} else {
		start, end := visibleWindow(len(fileSystems), m.fsSelected, max(1, panelContentHeight(height)-len(lines)))
		lines[0] = renderInlineSuffix(title+m.renderFileSystemControls(), renderScrollSummary(start, end, len(fileSystems)), contentWidth)
		for i := start; i < end; i++ {
			line := formatTableRow(columns, dataRows[i])
			if i == m.fsSelected {
				line = m.styles.selected.Width(contentWidth).Render(line)
			}
			lines = append(lines, line)
		}
	}

	return m.styles.panel.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) renderFileSystemDetails(width, height int) string {
	contentWidth := panelContentWidth(width)
	fs, ok := m.selectedFileSystem()
	if !ok {
		return m.styles.panelDim.Width(width).Render(normalizePanelLines([]string{"No filesystem selected"}, contentWidth, panelContentHeight(height)))
	}

	lines := []string{
		m.renderSectionTitle("Selected Filesystem", contentWidth),
		truncate(fmt.Sprintf("%s:%d", fs.Host, fs.Port), max(10, width-4)),
		"",
		m.metricLine("ID", fmt.Sprintf("%d", fs.ID), "Group", fallback(fs.SchedGroup, "-")),
		m.metricLine("Boot", fallback(fs.Boot, "-"), "Config", fallback(fs.ConfigStatus, "-")),
		m.metricLine("Drain", fallback(fs.DrainStatus, "-"), "Active", fallback(fs.Active, "-")),
		m.metricLine("Geotag", fallback(fs.Geotag, "-"), "Health", truncate(fs.Health, 12)),
		m.metricLine("Capacity", humanBytes(fs.CapacityBytes), "Used", humanBytes(fs.UsedBytes)),
		m.metricLine("Free", humanBytes(fs.FreeBytes), "Files", fmt.Sprintf("%d", fs.UsedFiles)),
		m.metricLine("BW", fmt.Sprintf("%.0f MB/s", fs.DiskBWMB), "IOPS", fmt.Sprintf("%.0f", fs.DiskIOPS)),
		m.metricLine("Read", fmt.Sprintf("%.2f MB/s", fs.ReadRateMB), "Write", fmt.Sprintf("%.2f MB/s", fs.WriteRateMB)),
		"",
		m.renderSectionTitle("Mount Path", contentWidth),
		truncate(fs.Path, max(10, width-4)),
	}

	return m.styles.panelDim.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) selectedFileSystem() (eos.FileSystemRecord, bool) {
	fileSystems := m.visibleFileSystems()
	if len(fileSystems) == 0 || m.fsSelected < 0 || m.fsSelected >= len(fileSystems) {
		return eos.FileSystemRecord{}, false
	}

	return fileSystems[m.fsSelected], true
}

func (m model) openFSConfigStatusEdit() (tea.Model, tea.Cmd) {
	fs, ok := m.selectedFileSystem()
	if !ok {
		return m, nil
	}
	// Find starting index matching the current configstatus.
	sel := 0
	for i, opt := range configStatusOptions {
		if fs.ConfigStatus == opt {
			sel = i
			break
		}
	}
	m.fsEdit = fsConfigStatusEdit{
		active:   true,
		fsID:     fs.ID,
		fsPath:   fs.Path,
		current:  fs.ConfigStatus,
		selected: sel,
		button:   buttonCancel,
	}
	return m, nil
}

func (m model) openFSConfigStatusEditAll() (tea.Model, tea.Cmd) {
	fileSystems := m.visibleFileSystems()
	if len(fileSystems) == 0 {
		return m, nil
	}

	targets := make([]fileSystemTarget, 0, len(fileSystems))
	for _, fs := range fileSystems {
		targets = append(targets, fileSystemTarget{id: fs.ID, path: fs.Path})
	}

	m.fsEdit = fsConfigStatusEdit{
		active:   true,
		selected: 0,
		applyAll: true,
		targets:  targets,
		button:   buttonCancel,
	}
	return m, nil
}

func (m model) renderFSConfigStatusEditPopup() string {
	if m.fsEdit.applyAll && m.fsEdit.confirm {
		cancelBtn := "[ Cancel ]"
		confirmBtn := "[ Confirm ]"
		if m.fsEdit.button == buttonCancel {
			cancelBtn = m.styles.selected.Render(cancelBtn)
		} else {
			confirmBtn = m.styles.selected.Render(confirmBtn)
		}

		chosen := configStatusOptions[m.fsEdit.selected]
		lines := []string{
			m.styles.popupTitle.Render("Confirm bulk configstatus"),
			fmt.Sprintf("This will run against %d filtered filesystems.", len(m.fsEdit.targets)),
			"",
			m.styles.value.Render(fmt.Sprintf("configstatus=%s", chosen)),
			"",
			lipgloss.JoinHorizontal(lipgloss.Left, cancelBtn, "  ", confirmBtn),
			"",
			m.styles.status.Render("g cancel  •  G confirm  •  enter apply  •  esc cancel"),
		}
		return m.renderModal(lines, lipgloss.Color("196"), 0)
	}

	lines := []string{
		m.styles.popupTitle.Render("Set configstatus"),
	}
	if m.fsEdit.applyAll {
		lines = append(lines,
			fmt.Sprintf("Targets: %d filtered filesystems", len(m.fsEdit.targets)),
			"Choose a value to apply to all visible filesystems.",
		)
	} else {
		lines = append(lines,
			fmt.Sprintf("Filesystem: %s (id %d)", m.fsEdit.fsPath, m.fsEdit.fsID),
			fmt.Sprintf("Current:    %s", m.styles.value.Render(fallback(m.fsEdit.current, "-"))),
		)
	}
	lines = append(lines,
		"",
	)
	for i, opt := range configStatusOptions {
		if i == m.fsEdit.selected {
			lines = append(lines, m.styles.selected.Render("▶ "+opt))
		} else {
			lines = append(lines, "  "+opt)
		}
	}
	hint := "↑↓ select  •  g/G home/end  •  enter apply  •  esc cancel"
	if m.fsEdit.applyAll {
		hint = "↑↓ select  •  g/G home/end  •  enter continue  •  esc cancel"
	}
	lines = append(lines, "", m.styles.status.Render(hint))
	return m.renderModal(lines, lipgloss.Color("62"), 0)
}

func apollonDrainRemoteArgs(fsID uint64, instance string) []string {
	return []string{
		apollonCLIPath,
		"drain",
		"--fsid",
		fmt.Sprintf("%d", fsID),
		"--instance",
		instance,
	}
}

func apollonDrainSSHArgs(fsID uint64, instance string) []string {
	return []string{
		"-o",
		"LogLevel=ERROR",
		apollonSSHTarget,
		eos.ShellJoin(apollonDrainRemoteArgs(fsID, instance)),
	}
}

func apollonDrainDisplayCommand(fsID uint64, instance string) string {
	return "ssh -o LogLevel=ERROR " + apollonSSHTarget + " " + eos.ShellDisplayJoin(apollonDrainRemoteArgs(fsID, instance))
}

func apollonInstanceCandidatesFromEndpoint(endpoint string) []string {
	endpoint = strings.TrimSpace(endpoint)
	if !strings.HasPrefix(endpoint, "ssh ") {
		return nil
	}

	raw := strings.TrimSpace(strings.TrimPrefix(endpoint, "ssh "))
	parts := strings.Split(raw, "→")
	candidates := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			candidates = append(candidates, part)
		}
	}
	return candidates
}

func (m model) apollonInstance() string {
	if m.client != nil {
		if instance := eos.NormalizeClusterInstance(m.client.OriginalSSHTarget()); instance != "" {
			return instance
		}
		if instance := eos.NormalizeClusterInstance(m.client.ResolvedSSHTarget()); instance != "" {
			return instance
		}
	}
	for _, candidate := range apollonInstanceCandidatesFromEndpoint(m.endpoint) {
		if instance := eos.NormalizeClusterInstance(candidate); instance != "" {
			return instance
		}
	}
	return ""
}

func (m model) startApollonDrainConfirm() (tea.Model, tea.Cmd) {
	fs, ok := m.selectedFileSystem()
	if !ok {
		return m, nil
	}

	instance := m.apollonInstance()
	if instance == "" {
		m.alert = errorAlert{
			active:  true,
			message: "Cannot determine Apollon instance from the current SSH target",
		}
		return m, nil
	}

	m.apollon = apollonDrainConfirm{
		active:   true,
		fsID:     fs.ID,
		fsPath:   fs.Path,
		instance: instance,
		command:  apollonDrainDisplayCommand(fs.ID, instance),
		button:   buttonCancel,
	}
	return m, nil
}

func (m model) renderApollonDrainConfirmPopup() string {
	cancelBtn := "[ Cancel ]"
	confirmBtn := "[ Confirm ]"

	if m.apollon.button == buttonCancel {
		cancelBtn = m.styles.selected.Render(cancelBtn)
	} else {
		confirmBtn = m.styles.selected.Render(confirmBtn)
	}

	// Show the command without wrapping so it's easy to copy.
	commandLines := []string{m.apollon.command}
	lines := []string{
		m.styles.popupTitle.Render("Confirm Apollon Drain"),
		fmt.Sprintf("Filesystem: %s (id %d)", m.apollon.fsPath, m.apollon.fsID),
		fmt.Sprintf("Instance:   %s", m.styles.value.Render(m.apollon.instance)),
		"",
		"The following command will be executed on eosops:",
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

	return m.renderModal(lines, lipgloss.Color("196"), 0)
}

func (m model) renderErrorAlert() string {
	footer := "enter / esc  close"
	if m.alert.fatal {
		footer = "press any key to quit"
	}
	// Clamp content lines to a reasonable popup width so no single long line
	// (e.g. a gRPC error) breaks the box layout or causes a missing border.
	maxContentWidth := min(max(40, m.contentWidth()-12), 100)
	lines := []string{
		m.styles.popupTitle.Render("Error"),
		"",
	}
	// Split the message so embedded newlines become separate elements;
	// passing a multi-line string as a single lipgloss element breaks the box.
	// Also strip \r (SSH stderr uses \r\n line endings) and ANSI escape codes
	// from each line — a \r in rendered output moves the terminal cursor to
	// column 0, causing the popup's closing border to overwrite the line start.
	for _, msgLine := range strings.Split(m.alert.message, "\n") {
		msgLine = strings.TrimRight(msgLine, "\r")
		msgLine = ansi.Strip(msgLine)
		lines = append(lines, ansi.Truncate(msgLine, maxContentWidth, "…"))
	}
	lines = append(lines, "", m.styles.status.Render(footer))
	return m.renderModal(lines, lipgloss.Color("196"), 0)
}

func (m model) renderFileSystemHeaderRow(columns []tableColumn) string {
	labels := []string{"host", "port", "id", "path", "schedgroup", "geotag", "boot", "configstatus", "drain", "usage %", "active", "health"}
	return m.renderSelectableHeaderRow(columns, labels, m.fsColumnSelected, m.fsSort, m.fsFilter)
}

func (m model) renderFileSystemControls() string {
	return fmt.Sprintf("  [col:%s filters:%d current:%s]",
		m.fsSelectedColumnLabel(),
		len(m.fsFilter.filters),
		filterValueLabel(m.fsFilter.filters[m.fsColumnSelected], m.popup.active && m.popup.view == viewFileSystems, m.popup.input.Value()),
	)
}
