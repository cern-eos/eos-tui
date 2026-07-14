package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lobis/eos-tui/eos"
)

type statsSection struct {
	title   string
	summary string
	lines   []string
	table   *statsTable
}

type statsTable struct {
	labels  []string
	columns []tableColumn
	rows    [][]string
}

const statsListSummaryWidthCap = 40
const statsMinListWidth = 58
const statsMinDetailWidth = 44

func (m model) renderNamespaceStatsView(height int) string {
	panelHeight := height + 2
	sections := m.statsSections()
	m.statsSectionSelected = clampIndex(m.statsSectionSelected, len(sections))

	totalWidth := m.panelWidth() + 2
	if statsUsesSinglePane(totalWidth) {
		paneWidth := max(1, totalWidth-2)
		if m.statsPaneFocus == statsFocusDetail {
			return normalizeBlockWidth(m.renderStatsSectionDetails(paneWidth, panelHeight, sections), totalWidth)
		}
		return normalizeBlockWidth(m.renderStatsSectionList(paneWidth, panelHeight, sections), totalWidth)
	}
	gap := 1
	availableWidth := max(1, totalWidth-gap)
	listWidth, detailWidth := m.statsPaneWidths(availableWidth, sections)

	list := m.renderStatsSectionList(listWidth, panelHeight, sections)
	details := m.renderStatsSectionDetails(detailWidth, panelHeight, sections)
	for i := 0; i < 4; i++ {
		combinedWidth := lipgloss.Width(list) + gap + lipgloss.Width(details)
		if combinedWidth <= totalWidth {
			break
		}
		overflow := combinedWidth - totalWidth
		if detailWidth > 44 {
			shrink := min(overflow, detailWidth-44)
			detailWidth -= shrink
			overflow -= shrink
		}
		if overflow > 0 && listWidth > 40 {
			listWidth -= min(overflow, listWidth-40)
		}
		list = m.renderStatsSectionList(listWidth, panelHeight, sections)
		details = m.renderStatsSectionDetails(detailWidth, panelHeight, sections)
	}
	combinedWidth := lipgloss.Width(list) + gap + lipgloss.Width(details)
	if deficit := totalWidth - combinedWidth; deficit > 0 {
		detailWidth += deficit
		details = m.renderStatsSectionDetails(detailWidth, panelHeight, sections)
	}
	return normalizeBlockWidth(lipgloss.JoinHorizontal(lipgloss.Top, list, " ", details), totalWidth)
}

func (m model) statsSections() []statsSection {
	return []statsSection{
		m.clusterStatsSection(),
		m.namespaceOverviewSection(),
		m.namespaceCacheSection(),
		m.inspectorOverviewSection(),
		m.inspectorLayoutsSection(),
		m.inspectorUsersSection(),
		m.inspectorGroupsSection(),
		m.inspectorAccessAgeSection(),
		m.inspectorBirthAgeSection(),
	}
}

func (m model) clusterStatsSection() statsSection {
	section := statsSection{title: "Cluster Summary"}
	switch {
	case m.fstStatsLoading && m.nodeStats == (eos.NodeStats{}):
		section.summary = "loading"
		section.lines = []string{"Loading cluster summary..."}
	case m.nodeStatsErr != nil && m.nodeStats == (eos.NodeStats{}):
		section.summary = "error"
		section.lines = []string{m.nodeStatsErr.Error()}
	default:
		section.summary = fmt.Sprintf("%s • %d files • %d dirs", fallback(m.nodeStats.State, "-"), m.nodeStats.FileCount, m.nodeStats.DirCount)
		section.lines = []string{
			m.metricLine("Health", fallback(m.nodeStats.State, "-"), "Threads", fmt.Sprintf("%d", m.nodeStats.ThreadCount)),
			m.metricLine("Files", fmt.Sprintf("%d", m.nodeStats.FileCount), "Dirs", fmt.Sprintf("%d", m.nodeStats.DirCount)),
			m.metricLine("Uptime", formatDuration(m.nodeStats.Uptime), "FDs", fmt.Sprintf("%d", m.nodeStats.FileDescs)),
			m.metricLine("Current FID", fmt.Sprintf("%d", m.nodeStats.CurrentFID), "Current CID", fmt.Sprintf("%d", m.nodeStats.CurrentCID)),
			m.metricLine("Memory RSS", humanBytes(m.nodeStats.MemResident), "Memory VSize", humanBytes(m.nodeStats.MemVirtual)),
		}
	}
	return section
}

func (m model) namespaceOverviewSection() statsSection {
	section := statsSection{title: "Namespace Overview"}
	switch {
	case m.nsStatsLoading && m.namespaceStats == (eos.NamespaceStats{}):
		section.summary = "loading"
		section.lines = []string{"Loading namespace statistics..."}
	case m.nsStatsErr != nil && m.namespaceStats == (eos.NamespaceStats{}):
		section.summary = "error"
		section.lines = []string{m.nsStatsErr.Error()}
	default:
		stats := m.namespaceStats
		section.summary = fmt.Sprintf("%s • %d files", fallback(stats.MasterHost, "-"), stats.TotalFiles)
		section.lines = []string{
			m.metricLine("Master", fallback(stats.MasterHost, "-"), "Total Files", fmt.Sprintf("%d", stats.TotalFiles)),
			m.metricLine("Total Directories", fmt.Sprintf("%d", stats.TotalDirectories), "", ""),
			m.metricLine("Current File ID", fmt.Sprintf("%d", stats.CurrentFID), "Current Container ID", fmt.Sprintf("%d", stats.CurrentCID)),
			m.metricLine("Generated File IDs", fmt.Sprintf("%d", stats.GeneratedFID), "Generated Container IDs", fmt.Sprintf("%d", stats.GeneratedCID)),
		}
	}
	return section
}

func (m model) namespaceCacheSection() statsSection {
	section := statsSection{title: "Cache & Contention"}
	switch {
	case m.nsStatsLoading && m.namespaceStats == (eos.NamespaceStats{}):
		section.summary = "loading"
		section.lines = []string{"Loading namespace statistics..."}
	case m.nsStatsErr != nil && m.namespaceStats == (eos.NamespaceStats{}):
		section.summary = "error"
		section.lines = []string{m.nsStatsErr.Error()}
	default:
		stats := m.namespaceStats
		section.summary = fmt.Sprintf("files %d/%d • containers %d/%d", stats.CacheFilesOccup, stats.CacheFilesMax, stats.CacheContainersOccup, stats.CacheContainersMax)
		section.lines = []string{
			m.metricLine("Read Contention", fmt.Sprintf("%.2f", stats.ContentionRead), "Write Contention", fmt.Sprintf("%.2f", stats.ContentionWrite)),
			m.metricLine("File Cache Max", fmt.Sprintf("%d", stats.CacheFilesMax), "Occupancy", fmt.Sprintf("%d", stats.CacheFilesOccup)),
			m.metricLine("File Requests", fmt.Sprintf("%d", stats.CacheFilesRequests), "Hits", fmt.Sprintf("%d", stats.CacheFilesHits)),
			m.metricLine("Container Cache Max", fmt.Sprintf("%d", stats.CacheContainersMax), "Occupancy", fmt.Sprintf("%d", stats.CacheContainersOccup)),
			m.metricLine("Container Requests", fmt.Sprintf("%d", stats.CacheContainersRequests), "Hits", fmt.Sprintf("%d", stats.CacheContainersHits)),
		}
	}
	return section
}

func (m model) inspectorOverviewSection() statsSection {
	section := statsSection{title: "Inspector Overview"}
	switch {
	case m.inspectorLoading && !hasInspectorStatsData(m.inspectorStats):
		section.summary = "loading"
		section.lines = []string{"Loading inspector statistics..."}
	case m.inspectorErr != nil && !hasInspectorStatsData(m.inspectorStats):
		section.summary = inspectorErrorSummary(m.inspectorErr)
		section.lines = []string{m.inspectorErr.Error()}
	default:
		inspector := m.inspectorStats
		section.summary = fmt.Sprintf("avg %s • %d layouts", humanBytes(inspector.AvgFileSize), inspector.LayoutCount)
		section.lines = []string{
			m.metricLine("Avg File Size", humanBytes(inspector.AvgFileSize), "Layouts", fmt.Sprintf("%d", inspector.LayoutCount)),
			m.metricLine("Hardlinks", fmt.Sprintf("%d", inspector.HardlinkCount), "Symlinks", fmt.Sprintf("%d", inspector.SymlinkCount)),
			m.metricLine("Hardlink Volume", humanBytes(inspector.HardlinkVolume), "Top Layout", formatInspectorLayout(inspector.TopLayout)),
			m.metricLine("Top User Cost", formatInspectorCost(inspector.TopUserCost), "Top Group Cost", formatInspectorCost(inspector.TopGroupCost)),
		}
	}
	return section
}

func (m model) inspectorLayoutsSection() statsSection {
	section := statsSection{title: "Inspector Layouts"}
	switch {
	case m.inspectorLoading && len(m.inspectorStats.Layouts) == 0:
		section.summary = "loading"
		section.lines = []string{"Loading inspector layout data..."}
	case m.inspectorErr != nil && len(m.inspectorStats.Layouts) == 0:
		section.summary = inspectorErrorSummary(m.inspectorErr)
		section.lines = []string{m.inspectorErr.Error()}
	case len(m.inspectorStats.Layouts) == 0:
		section.summary = "none"
		section.lines = []string{"No layout statistics available"}
	default:
		section.summary = formatInspectorLayout(m.inspectorStats.TopLayout)
		section.lines = []string{
			m.metricLine("Top Layout", m.inspectorStats.TopLayout.Layout, "Type", fallback(m.inspectorStats.TopLayout.Type, "-")),
			m.metricLine("Volume", humanBytes(m.inspectorStats.TopLayout.VolumeBytes), "Physical", humanBytes(m.inspectorStats.TopLayout.PhysicalBytes)),
			m.metricLine("Locations", fmt.Sprintf("%d", m.inspectorStats.TopLayout.Locations), "Tracked Layouts", fmt.Sprintf("%d", len(m.inspectorStats.Layouts))),
			"",
		}
		section.table = m.topLayoutsTable()
	}
	return section
}

func (m model) inspectorUsersSection() statsSection {
	section := statsSection{title: "Inspector Users"}
	switch {
	case m.inspectorLoading && len(m.inspectorStats.UserCosts) == 0:
		section.summary = "loading"
		section.lines = []string{"Loading inspector user cost data..."}
	case m.inspectorErr != nil && len(m.inspectorStats.UserCosts) == 0:
		section.summary = inspectorErrorSummary(m.inspectorErr)
		section.lines = []string{m.inspectorErr.Error()}
	case len(m.inspectorStats.UserCosts) == 0:
		section.summary = "none"
		section.lines = []string{"No user cost statistics available"}
	default:
		section.summary = formatInspectorCost(m.inspectorStats.TopUserCost)
		section.lines = []string{
			m.metricLine("Top User", fallback(m.inspectorStats.TopUserCost.Name, "-"), "UID", fmt.Sprintf("%d", m.inspectorStats.TopUserCost.ID)),
			m.metricLine("Cost", fmt.Sprintf("%.2f", m.inspectorStats.TopUserCost.Cost), "TB Years", fmt.Sprintf("%.2f", m.inspectorStats.TopUserCost.TBYears)),
			"",
		}
		section.table = m.topCostsTable("user", m.inspectorStats.UserCosts)
	}
	return section
}

func (m model) inspectorGroupsSection() statsSection {
	section := statsSection{title: "Inspector Groups"}
	switch {
	case m.inspectorLoading && len(m.inspectorStats.GroupCosts) == 0:
		section.summary = "loading"
		section.lines = []string{"Loading inspector group cost data..."}
	case m.inspectorErr != nil && len(m.inspectorStats.GroupCosts) == 0:
		section.summary = inspectorErrorSummary(m.inspectorErr)
		section.lines = []string{m.inspectorErr.Error()}
	case len(m.inspectorStats.GroupCosts) == 0:
		section.summary = "none"
		section.lines = []string{"No group cost statistics available"}
	default:
		section.summary = formatInspectorCost(m.inspectorStats.TopGroupCost)
		section.lines = []string{
			m.metricLine("Top Group", fallback(m.inspectorStats.TopGroupCost.Name, "-"), "GID", fmt.Sprintf("%d", m.inspectorStats.TopGroupCost.ID)),
			m.metricLine("Cost", fmt.Sprintf("%.2f", m.inspectorStats.TopGroupCost.Cost), "TB Years", fmt.Sprintf("%.2f", m.inspectorStats.TopGroupCost.TBYears)),
			"",
		}
		section.table = m.topCostsTable("group", m.inspectorStats.GroupCosts)
	}
	return section
}

func (m model) inspectorAccessAgeSection() statsSection {
	return m.inspectorBinsSection("Inspector Access Age", m.inspectorStats.AccessFiles, m.inspectorStats.AccessVolume, m.inspectorLoading, m.inspectorErr)
}

func (m model) inspectorBirthAgeSection() statsSection {
	return m.inspectorBinsSection("Inspector Birth Age", m.inspectorStats.BirthFiles, m.inspectorStats.BirthVolume, m.inspectorLoading, m.inspectorErr)
}

func (m model) inspectorBinsSection(title string, files, volume []eos.InspectorBin, loading bool, err error) statsSection {
	section := statsSection{title: title}
	switch {
	case loading && len(files) == 0 && len(volume) == 0:
		section.summary = "loading"
		section.lines = []string{"Loading inspector age-bucket data..."}
	case err != nil && len(files) == 0 && len(volume) == 0:
		section.summary = inspectorErrorSummary(err)
		section.lines = []string{err.Error()}
	case len(files) == 0 && len(volume) == 0:
		section.summary = "none"
		section.lines = []string{"No age-bucket statistics available"}
	default:
		section.summary = m.formatInspectorBinSummary(files, volume)
		section.table = m.inspectorBinsTable(files, volume)
	}
	return section
}

func inspectorErrorSummary(err error) string {
	if err == nil {
		return ""
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "permission denied"), strings.Contains(lower, "unavailable"):
		return "unavailable"
	case strings.Contains(lower, "disabled"):
		return "disabled"
	default:
		return "error"
	}
}

func (m model) renderStatsSectionList(width, height int, sections []statsSection) string {
	contentWidth := panelContentWidth(width)
	dataRows := make([][]string, len(sections))
	sectionWidth := lipgloss.Width("section")
	for i, section := range sections {
		dataRows[i] = []string{section.title, section.summary}
		sectionWidth = max(sectionWidth, lipgloss.Width(section.title))
	}
	maxSectionWidth := max(1, contentWidth-lipgloss.Width("summary")-1)
	sectionWidth = min(sectionWidth, maxSectionWidth)
	summaryWidth := max(1, min(statsListSummaryWidthCap, contentWidth-sectionWidth-1))
	columns := allocateTableColumns(contentWidth, []tableColumn{
		{title: "section", min: sectionWidth, maxw: sectionWidth, weight: 0},
		{title: "summary", min: summaryWidth, maxw: summaryWidth, weight: 0},
	})

	title := m.renderSectionTitle("General Statistics", contentWidth)
	lines := []string{
		title,
		"",
		m.renderSimpleHeaderRow(columns, []string{"section", "summary"}),
	}
	start, end := visibleWindow(len(sections), m.statsSectionSelected, max(1, panelContentHeight(height)-len(lines)))
	lines[0] = renderInlineSuffix(title, renderScrollSummary(start, end, len(sections)), contentWidth)
	for i := start; i < end; i++ {
		line := formatTableRow(columns, dataRows[i])
		if i == m.statsSectionSelected {
			line = m.styles.selected.Width(contentWidth).Render(line)
		}
		lines = append(lines, line)
	}

	style := m.styles.panel
	if m.statsPaneFocus == statsFocusDetail {
		style = m.styles.panelDim
	}
	return style.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) renderStatsSectionDetails(width, height int, sections []statsSection) string {
	contentWidth := panelContentWidth(width)
	if len(sections) == 0 {
		style := m.styles.panelDim
		if m.statsPaneFocus == statsFocusDetail {
			style = m.styles.panel
		}
		return style.Width(width).Render(normalizePanelLines([]string{"No statistics available"}, contentWidth, panelContentHeight(height)))
	}

	selected := sections[clampIndex(m.statsSectionSelected, len(sections))]
	lines := []string{m.renderSectionTitle(selected.title, contentWidth)}
	if selected.table == nil {
		for _, line := range selected.lines {
			lines = append(lines, truncate(line, contentWidth))
		}
	} else {
		if len(m.statsFilter.filters) > 0 {
			lines = append(lines, truncate(m.renderFilterSummary(m.statsFilter.filters, m.statsFilterColumnLabel), contentWidth))
		}
		for _, line := range selected.lines {
			lines = append(lines, cropStatsLine(line, 0, contentWidth))
		}
		filteredRows := m.visibleStatsTableRows(selected)
		capacity := max(1, panelContentHeight(height)-len(lines)-1)
		selectedRow := clampIndex(m.statsDetailSelected, len(filteredRows))
		start, end := visibleWindow(len(filteredRows), selectedRow, capacity)
		lines[0] = renderInlineSuffix(lines[0], renderScrollSummary(start, end, len(filteredRows)), contentWidth)
		xOffset := m.statsAdjustedOffsetX(selected, contentWidth)
		header := m.renderSelectableHeaderRow(selected.table.columns, selected.table.labels, m.statsDetailColumnSelected, sortState{column: -1}, m.statsFilter)
		lines = append(lines, cropStatsLine(header, xOffset, contentWidth))
		for i := start; i < end; i++ {
			line := cropStatsLine(formatTableRow(selected.table.columns, filteredRows[i]), xOffset, contentWidth)
			if m.statsPaneFocus == statsFocusDetail && i == selectedRow {
				line = m.styles.selected.Width(contentWidth).Render(line)
			}
			lines = append(lines, line)
		}
	}
	style := m.styles.panelDim
	if m.statsPaneFocus == statsFocusDetail {
		style = m.styles.panel
	}
	return style.Width(width).Render(normalizePanelLines(lines, contentWidth, panelContentHeight(height)))
}

func (m model) statsListNaturalWidth(sections []statsSection) int {
	sectionWidth := lipgloss.Width("section")
	for _, section := range sections {
		sectionWidth = max(sectionWidth, lipgloss.Width(section.title))
	}
	summaryWidth := statsListSummaryWidthCap
	contentWidth := max(lipgloss.Width("General Statistics"), sectionWidth+1+summaryWidth) + 2
	return contentWidth + 4
}

func (m model) statsPaneWidths(totalWidth int, sections []statsSection) (listWidth, detailWidth int) {
	listNaturalWidth := max(statsMinListWidth, m.statsListNaturalWidth(sections))

	if statsUsesSinglePane(totalWidth) {
		listWidth = max(1, totalWidth/2)
		return listWidth, max(1, totalWidth-listWidth)
	}

	listWidth = min(listNaturalWidth, totalWidth-statsMinDetailWidth)
	listWidth = max(statsMinListWidth, listWidth)
	detailWidth = totalWidth - listWidth

	return listWidth, detailWidth
}

func statsUsesSinglePane(totalWidth int) bool {
	return totalWidth < statsMinListWidth+statsMinDetailWidth+1
}

func (m model) visibleStatsTableRows(section statsSection) [][]string {
	if section.table == nil {
		return nil
	}
	rows := make([][]string, 0, len(section.table.rows))
	for _, row := range section.table.rows {
		if m.matchesStatsFilters(row) {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return [][]string{{fmt.Sprintf("No matches for %q", strings.TrimSpace(m.statsFilter.filters[statsFilterQueryColumn]))}}
	}
	return rows
}

func (m model) matchesStatsFilters(row []string) bool {
	for col, filter := range m.statsFilter.filters {
		if filter == "" {
			continue
		}
		if col < 0 || col >= len(row) || !matchesFilterQuery(row[col], filter) {
			return false
		}
	}
	return true
}

func (m model) matchesStatsFiltersExcept(row []string, excludeColumn int) bool {
	for col, filter := range m.statsFilter.filters {
		if col == excludeColumn || filter == "" {
			continue
		}
		if col < 0 || col >= len(row) || !matchesFilterQuery(row[col], filter) {
			return false
		}
	}
	return true
}

func (m model) statsFilterColumnLabel(col int) string {
	sections := m.statsSections()
	if len(sections) == 0 {
		return "detail"
	}
	selected := sections[clampIndex(m.statsSectionSelected, len(sections))]
	if selected.table == nil || col < 0 || col >= len(selected.table.labels) {
		return "detail"
	}
	return selected.table.labels[col]
}

func (m model) statsCurrentSectionHasTable(sections []statsSection) bool {
	if len(sections) == 0 {
		return false
	}
	return sections[clampIndex(m.statsSectionSelected, len(sections))].table != nil
}

func (m model) statsAdjustedOffsetX(section statsSection, contentWidth int) int {
	if section.table == nil || contentWidth <= 0 {
		return 0
	}
	offset := max(0, m.statsDetailOffsetX)
	column := clampIndex(m.statsDetailColumnSelected, len(section.table.columns))
	start := 0
	for i := 0; i < column; i++ {
		start += section.table.columns[i].min + 1
	}
	end := start + section.table.columns[column].min
	if start < offset {
		offset = start
	}
	if end > offset+contentWidth {
		offset = end - contentWidth
	}
	maxOffset := m.statsDetailMaxOffsetXForWidth(section, contentWidth)
	return min(max(0, offset), maxOffset)
}

func (m model) statsVisibleDetailLines(section statsSection) []string {
	if len(section.lines) == 0 {
		return []string{"No statistics available"}
	}
	return append([]string(nil), section.lines...)
}

func (m model) currentStatsBodyHeight() int {
	headerHeight := lipgloss.Height(m.renderHeader())
	footerHeight := lipgloss.Height(m.renderFooter())
	middleHeight := max(0, m.height-headerHeight-footerHeight)
	availableHeight := max(4, middleHeight-2)
	bodyHeight, _ := m.splitMainAndCommandHeights(availableHeight)
	return bodyHeight
}

func (m model) currentStatsDetailContentWidth(sections []statsSection) int {
	totalWidth := m.panelWidth() + 2
	if statsUsesSinglePane(totalWidth) {
		return panelContentWidth(max(1, totalWidth-2))
	}
	gap := 1
	availableWidth := max(1, totalWidth-gap)
	_, detailWidth := m.statsPaneWidths(availableWidth, sections)
	return panelContentWidth(detailWidth)
}

func (m model) currentStatsDetailCapacity() int {
	capacity := panelContentHeight(m.currentStatsBodyHeight()+2) - 1
	if len(m.statsFilter.filters) > 0 {
		capacity--
	}
	return max(1, capacity)
}

func (m model) statsDetailMaxOffsetXForWidth(section statsSection, contentWidth int) int {
	lines := m.statsVisibleDetailLines(section)
	if section.table != nil {
		lines = append(lines, m.renderSimpleHeaderRow(section.table.columns, section.table.labels))
		for _, row := range section.table.rows {
			lines = append(lines, formatTableRow(section.table.columns, row))
		}
	}
	maxWidth := 0
	for _, line := range lines {
		maxWidth = max(maxWidth, lipgloss.Width(line))
	}
	return max(0, maxWidth-contentWidth)
}

func (m model) statsDetailMaxOffsetX(sections []statsSection) int {
	if len(sections) == 0 {
		return 0
	}
	selected := sections[clampIndex(m.statsSectionSelected, len(sections))]
	return m.statsDetailMaxOffsetXForWidth(selected, m.currentStatsDetailContentWidth(sections))
}

func (m model) statsDetailMaxOffsetY(sections []statsSection) int {
	if len(sections) == 0 {
		return 0
	}
	selected := sections[clampIndex(m.statsSectionSelected, len(sections))]
	lines := m.statsVisibleDetailLines(selected)
	return max(0, len(lines)-m.currentStatsDetailCapacity())
}

func (m model) statsDetailLineCount(sections []statsSection) int {
	if len(sections) == 0 {
		return 0
	}
	selected := sections[clampIndex(m.statsSectionSelected, len(sections))]
	if selected.table != nil {
		return len(m.visibleStatsTableRows(selected))
	}
	return len(m.statsVisibleDetailLines(selected))
}

func (m model) statsPopupValues() []string {
	sections := m.statsSections()
	if len(sections) == 0 {
		return nil
	}
	selected := sections[clampIndex(m.statsSectionSelected, len(sections))]
	if selected.table == nil {
		return nil
	}
	col := min(max(0, m.statsDetailColumnSelected), len(selected.table.labels)-1)
	values := make([]string, 0, len(selected.table.rows))
	for _, row := range selected.table.rows {
		if !m.matchesStatsFiltersExcept(row, col) {
			continue
		}
		if col < len(row) {
			values = append(values, row[col])
		}
	}
	return values
}

func statsVisibleOffsetWindow(total, offset, capacity int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if capacity <= 0 || total <= capacity {
		return 0, total
	}
	offset = min(max(0, offset), max(0, total-capacity))
	return offset, min(total, offset+capacity)
}

func cropStatsLine(line string, offset, width int) string {
	if width <= 0 {
		return ""
	}
	start := max(0, offset)
	if start >= lipgloss.Width(line) {
		return strings.Repeat(" ", width)
	}
	return padVisibleWidth(ansi.Cut(line, start, start+width), width)
}

func (m model) topLayoutsTable() *statsTable {
	rows := make([][]string, 0, len(m.inspectorStats.Layouts))
	for _, layout := range m.inspectorStats.Layouts {
		rows = append(rows, []string{
			layout.Layout,
			fallback(layout.Type, "-"),
			humanBytes(layout.VolumeBytes),
			humanBytes(layout.PhysicalBytes),
			fmt.Sprintf("%d", layout.Locations),
		})
	}
	columns := contentAwareColumns([]tableColumn{
		{title: "layout", min: 8, weight: 2},
		{title: "type", min: 7, weight: 1},
		{title: "volume", min: 10, weight: 1, right: true},
		{title: "physical", min: 10, weight: 1, right: true},
		{title: "locations", min: 9, weight: 0, right: true},
	}, rows)
	return &statsTable{
		labels:  []string{"layout", "type", "volume", "physical", "locations"},
		columns: columns,
		rows:    rows,
	}
}

func (m model) topCostsTable(kind string, costs []eos.InspectorCostRecord) *statsTable {
	rows := make([][]string, 0, len(costs))
	for _, record := range costs {
		rows = append(rows, []string{
			record.Name,
			fmt.Sprintf("%d", record.ID),
			fmt.Sprintf("%.2f", record.Cost),
			fmt.Sprintf("%.2f", record.TBYears),
		})
	}
	columns := contentAwareColumns([]tableColumn{
		{title: kind, min: 10, weight: 2},
		{title: "id", min: 6, weight: 0, right: true},
		{title: "cost", min: 8, weight: 1, right: true},
		{title: "tbyears", min: 8, weight: 1, right: true},
	}, rows)
	return &statsTable{
		labels:  []string{kind, "id", "cost", "tbyears"},
		columns: columns,
		rows:    rows,
	}
}

func (m model) formatInspectorBinSummary(files, volume []eos.InspectorBin) string {
	fileLabel := "-"
	if len(files) > 0 {
		last := files[len(files)-1]
		fileLabel = fmt.Sprintf("%s=%d files", formatInspectorBinLabel(last.BinSeconds), last.Value)
	}
	volumeLabel := "-"
	if len(volume) > 0 {
		last := volume[len(volume)-1]
		volumeLabel = fmt.Sprintf("%s=%s", formatInspectorBinLabel(last.BinSeconds), humanBytes(last.Value))
	}
	return fileLabel + " • " + volumeLabel
}

func (m model) inspectorBinsTable(files, volume []eos.InspectorBin) *statsTable {
	volumes := make(map[uint64]uint64, len(volume))
	for _, item := range volume {
		volumes[item.BinSeconds] = item.Value
	}

	rows := make([][]string, 0, max(len(files), len(volume)))
	for _, item := range files {
		rows = append(rows, []string{
			formatInspectorBinLabel(item.BinSeconds),
			fmt.Sprintf("%d", item.Value),
			humanBytes(volumes[item.BinSeconds]),
		})
	}
	if len(rows) == 0 {
		for _, item := range volume {
			rows = append(rows, []string{
				formatInspectorBinLabel(item.BinSeconds),
				"-",
				humanBytes(item.Value),
			})
		}
	}
	columns := contentAwareColumns([]tableColumn{
		{title: "age bucket", min: 10, weight: 2},
		{title: "files", min: 7, weight: 1, right: true},
		{title: "volume", min: 8, weight: 1, right: true},
	}, rows)
	return &statsTable{
		labels:  []string{"age bucket", "files", "volume"},
		columns: columns,
		rows:    rows,
	}
}

func formatInspectorLayout(layout eos.InspectorLayoutSummary) string {
	if layout.Layout == "" {
		return "-"
	}
	return fmt.Sprintf("%s %s %s", layout.Layout, fallback(layout.Type, "-"), humanBytes(layout.VolumeBytes))
}

func formatInspectorCost(record eos.InspectorCostRecord) string {
	if record.Name == "" {
		return "-"
	}
	return fmt.Sprintf("%s %.2f", record.Name, record.Cost)
}

func formatInspectorBinLabel(seconds uint64) string {
	if seconds == 0 {
		return "now"
	}
	d := time.Duration(seconds) * time.Second
	switch {
	case seconds%31536000 == 0:
		return fmt.Sprintf("%dy", seconds/31536000)
	case seconds%86400 == 0:
		return fmt.Sprintf("%dd", seconds/86400)
	case seconds%3600 == 0:
		return fmt.Sprintf("%dh", seconds/3600)
	default:
		return strings.TrimSuffix(formatDuration(d), "0s")
	}
}
