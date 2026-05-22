package ui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/lobis/eos-tui/eos"
)

// ioShapingMergedRows returns a total traffic row followed by the union of
// traffic records and policy records for the current mode, sorted
// alphabetically by id. Rows with traffic but no policy, policy but no traffic,
// or both are all included.
func (m model) ioShapingMergedRows() []ioShapingMergedRow {
	policyByID := make(map[string]eos.IOShapingPolicyRecord)
	if policyType, ok := ioShapingPolicyTypeForMode(m.ioShapingMode); ok {
		for _, p := range m.ioShapingPolicies {
			if strings.ToLower(p.Type) == policyType {
				policyByID[p.ID] = p
			}
		}
	}

	seen := make(map[string]bool)
	var rows []ioShapingMergedRow
	total := eos.IOShapingRecord{
		ID:   ioShapingTotalLabel(m.ioShapingMode),
		Type: "total",
	}
	for i := range m.ioShaping {
		r := &m.ioShaping[i]
		seen[r.ID] = true
		total.ReadBps += r.ReadBps
		total.WriteBps += r.WriteBps
		total.ReadIOPS += r.ReadIOPS
		total.WriteIOPS += r.WriteIOPS
		if r.WindowSec > total.WindowSec {
			total.WindowSec = r.WindowSec
		}
		row := ioShapingMergedRow{id: r.ID, traffic: r}
		if p, ok := policyByID[r.ID]; ok {
			row.policy = &p
		}
		rows = append(rows, row)
	}
	for id, p := range policyByID {
		if !seen[id] {
			pc := p
			rows = append(rows, ioShapingMergedRow{id: id, policy: &pc})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
	if len(m.ioShaping) > 0 {
		rows = append([]ioShapingMergedRow{{
			id:      total.ID,
			total:   true,
			traffic: &total,
		}}, rows...)
	}
	return rows
}

func ioShapingTotalLabel(mode eos.IOShapingMode) string {
	switch mode {
	case eos.IOShapingUsers:
		return "[total users]"
	case eos.IOShapingGroups:
		return "[total groups]"
	case eos.IOShapingNodes:
		return "[total nodes]"
	default:
		return "[total apps]"
	}
}

func ioShapingPolicyTypeForMode(mode eos.IOShapingMode) (string, bool) {
	switch mode {
	case eos.IOShapingApps:
		return "app", true
	case eos.IOShapingUsers:
		return "uid", true
	case eos.IOShapingGroups:
		return "gid", true
	default:
		return "", false
	}
}

func ioShapingModeHasPolicies(mode eos.IOShapingMode) bool {
	_, ok := ioShapingPolicyTypeForMode(mode)
	return ok
}

func (m model) renderIOShapingView(height int) string {
	if m.ioShapingMode == eos.IOShapingPressure {
		return m.renderIOShapingPressureView(height)
	}

	width := m.panelWidth()
	contentWidth := panelContentWidth(width)

	idLabel := "application"
	switch m.ioShapingMode {
	case eos.IOShapingUsers:
		idLabel = "uid"
	case eos.IOShapingGroups:
		idLabel = "gid"
	case eos.IOShapingNodes:
		idLabel = "node"
	}

	indicator := ""
	if m.ioShapingLoading {
		indicator = m.styles.status.Render("  ↻")
	}

	if m.ioShapingErr != nil {
		message := m.ioShapingErr.Error()
		if errors.Is(m.ioShapingErr, eos.ErrIOShapingUnsupported) {
			message = "IO traffic shaping is not available on this EOS instance.\nThe `io shaping` subcommand is missing — check `eos io --help` on the MGM."
		}
		lines := []string{
			m.styles.label.Render("IO Traffic Shaping") + indicator,
			"",
			m.styles.error.Render(message),
		}
		return m.styles.panelDim.Width(width).Render(fitLines(lines, height))
	}

	rows := m.ioShapingMergedRows()

	formatLimit := func(v float64) string {
		if v == 0 {
			return "-"
		}
		return humanBytesRate(v)
	}
	enabledStr := func(p *eos.IOShapingPolicyRecord) string {
		if p == nil {
			return "-"
		}
		if p.Enabled {
			return "yes"
		}
		return "no"
	}

	dataRows := make([][]string, len(rows))
	for i, r := range rows {
		readRate, writeRate, readIOPS, writeIOPS := "-", "-", "-", "-"
		if r.traffic != nil {
			readRate = humanBytesRate(r.traffic.ReadBps)
			writeRate = humanBytesRate(r.traffic.WriteBps)
			readIOPS = fmt.Sprintf("%.1f", r.traffic.ReadIOPS)
			writeIOPS = fmt.Sprintf("%.1f", r.traffic.WriteIOPS)
		}
		limRead, limWrite, resRead, resWrite := "-", "-", "-", "-"
		if r.policy != nil {
			limRead = formatLimit(r.policy.LimitReadBytesPerSec)
			limWrite = formatLimit(r.policy.LimitWriteBytesPerSec)
			resRead = formatLimit(r.policy.ReservationReadBytesPerSec)
			resWrite = formatLimit(r.policy.ReservationWriteBytesPerSec)
		}
		dataRows[i] = []string{
			r.id,
			readRate, writeRate, readIOPS, writeIOPS,
			enabledStr(r.policy),
			limRead, limWrite, resRead, resWrite,
		}
	}

	headers := []string{idLabel, "read rate", "write rate", "read iops", "write iops", "enabled", "lim read", "lim write", "res read", "res write"}
	columns := allocateTableColumns(contentWidth, contentAwareColumns([]tableColumn{
		{title: idLabel, min: 10, weight: 4},
		{title: "read rate", min: 10, weight: 1, right: true},
		{title: "write rate", min: 10, weight: 1, right: true},
		{title: "read iops", min: 9, weight: 0, right: true},
		{title: "write iops", min: 10, weight: 0, right: true},
		{title: "enabled", min: 7, weight: 0},
		{title: "lim read", min: 10, weight: 0, right: true},
		{title: "lim write", min: 10, weight: 0, right: true},
		{title: "res read", min: 10, weight: 0, right: true},
		{title: "res write", min: 10, weight: 0, right: true},
	}, dataRows))

	limitsState := "limits ?"
	if m.ioShapingConfigLoaded {
		limitsState = "limits off"
		if m.ioShapingConfig.LimitsEnabled {
			limitsState = "limits on"
		}
	} else if m.ioShapingConfigErr != nil {
		limitsState = "limits err"
	}

	title := m.styles.label.Render("IO Traffic  ") +
		m.styles.label.Render("5s window  ") +
		m.styles.label.Render("m "+limitsState+"  ") +
		modeTabLabel(m.ioShapingMode, eos.IOShapingApps, "a apps", m.styles) + "  " +
		modeTabLabel(m.ioShapingMode, eos.IOShapingUsers, "u users", m.styles) + "  " +
		modeTabLabel(m.ioShapingMode, eos.IOShapingGroups, "g groups", m.styles) + "  " +
		modeTabLabel(m.ioShapingMode, eos.IOShapingNodes, "n nodes", m.styles) + "  " +
		modeTabLabel(m.ioShapingMode, eos.IOShapingPressure, "p pressure", m.styles) +
		indicator

	lines := []string{title, "", m.renderSimpleHeaderRow(columns, headers)}

	if m.ioShapingLoading && len(rows) == 0 {
		lines = append(lines, "Loading...")
	} else if len(rows) == 0 {
		lines = append(lines, "(no data)")
	} else {
		start, end := visibleWindow(len(rows), m.ioShapingSelected, max(1, height-len(lines)))
		lines[0] = title + renderScrollSummary(start, end, len(rows))
		for i := start; i < end; i++ {
			line := formatTableRow(columns, dataRows[i])
			if i == m.ioShapingSelected {
				line = m.styles.selected.Width(contentWidth).Render(line)
			}
			lines = append(lines, line)
		}
	}

	return m.styles.panel.Width(width).Render(fitLines(lines, height))
}

func (m model) renderIOShapingPressureView(height int) string {
	width := m.panelWidth()
	contentWidth := panelContentWidth(width)

	indicator := ""
	if m.ioShapingLoading {
		indicator = m.styles.status.Render("  ↻")
	}

	if m.ioShapingErr != nil {
		message := m.ioShapingErr.Error()
		if errors.Is(m.ioShapingErr, eos.ErrIOShapingUnsupported) {
			message = "IO shaping pressure is not available on this EOS instance.\nThe `io shaping pressure ls` subcommand is missing on the MGM."
		}
		lines := []string{
			m.styles.label.Render("IO Traffic Pressure") + indicator,
			"",
			m.styles.error.Render(message),
		}
		return m.styles.panelDim.Width(width).Render(fitLines(lines, height))
	}

	records := append([]eos.IOShapingPressureRecord(nil), m.ioShapingPressure...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].App == records[j].App {
			return records[i].NodeID < records[j].NodeID
		}
		return records[i].App < records[j].App
	})

	dataRows := make([][]string, len(records))
	for i, r := range records {
		dataRows[i] = []string{
			r.App,
			r.NodeID,
			fmt.Sprintf("%.3f", r.NodeIOPressure),
			humanBytesRate(r.ReadRateBps),
			humanBytesRate(r.WriteRateBps),
			humanBytesRate(r.ReservationReadBytesPerSec),
			humanBytesRate(r.ReservationWriteBytesPerSec),
			humanBytesRate(r.ReadReservationDeficitBps),
			humanBytesRate(r.WriteReservationDeficitBps),
			ioShapingPressureFlags(r),
		}
	}

	headers := []string{"application", "node", "pressure", "read", "write", "res read", "res write", "def read", "def write", "active"}
	columns := allocateTableColumns(contentWidth, contentAwareColumns([]tableColumn{
		{title: "application", min: 12, weight: 3},
		{title: "node", min: 16, weight: 3},
		{title: "pressure", min: 8, weight: 0, right: true},
		{title: "read", min: 10, weight: 0, right: true},
		{title: "write", min: 10, weight: 0, right: true},
		{title: "res read", min: 10, weight: 0, right: true},
		{title: "res write", min: 10, weight: 0, right: true},
		{title: "def read", min: 10, weight: 0, right: true},
		{title: "def write", min: 10, weight: 0, right: true},
		{title: "active", min: 8, weight: 1},
	}, dataRows))

	limitsState := "limits ?"
	if m.ioShapingConfigLoaded {
		limitsState = "limits off"
		if m.ioShapingConfig.LimitsEnabled {
			limitsState = "limits on"
		}
	} else if m.ioShapingConfigErr != nil {
		limitsState = "limits err"
	}

	title := m.styles.label.Render("IO Traffic  ") +
		m.styles.label.Render("pressure  ") +
		m.styles.label.Render("m "+limitsState+"  ") +
		modeTabLabel(m.ioShapingMode, eos.IOShapingApps, "a apps", m.styles) + "  " +
		modeTabLabel(m.ioShapingMode, eos.IOShapingUsers, "u users", m.styles) + "  " +
		modeTabLabel(m.ioShapingMode, eos.IOShapingGroups, "g groups", m.styles) + "  " +
		modeTabLabel(m.ioShapingMode, eos.IOShapingNodes, "n nodes", m.styles) + "  " +
		modeTabLabel(m.ioShapingMode, eos.IOShapingPressure, "p pressure", m.styles) +
		indicator

	lines := []string{title, "", m.renderSimpleHeaderRow(columns, headers)}
	if m.ioShapingLoading && len(records) == 0 {
		lines = append(lines, "Loading...")
	} else if len(records) == 0 {
		lines = append(lines, "(no pressure data)")
	} else {
		start, end := visibleWindow(len(records), m.ioShapingSelected, max(1, height-len(lines)))
		lines[0] = title + renderScrollSummary(start, end, len(records))
		for i := start; i < end; i++ {
			line := formatTableRow(columns, dataRows[i])
			if i == m.ioShapingSelected {
				line = m.styles.selected.Width(contentWidth).Render(line)
			}
			lines = append(lines, line)
		}
	}

	return m.styles.panel.Width(width).Render(fitLines(lines, height))
}

func ioShapingPressureFlags(r eos.IOShapingPressureRecord) string {
	var flags []string
	if r.ReadPressureActive {
		flags = append(flags, "read")
	}
	if r.WritePressureActive {
		flags = append(flags, "write")
	}
	if r.ReadReservationDeficitActive {
		flags = append(flags, "read-def")
	}
	if r.WriteReservationDeficitActive {
		flags = append(flags, "write-def")
	}
	if r.ReadTriggersCompetitorThrottling {
		flags = append(flags, "read-throttle")
	}
	if r.WriteTriggersCompetitorThrottling {
		flags = append(flags, "write-throttle")
	}
	if r.NodeHasPressuredReadReservation {
		flags = append(flags, "node-read")
	}
	if r.NodeHasPressuredWriteReservation {
		flags = append(flags, "node-write")
	}
	if len(flags) == 0 {
		return "-"
	}
	return strings.Join(flags, ",")
}

func modeTabLabel(current, target eos.IOShapingMode, label string, s styles) string {
	if current == target {
		return s.tabActive.Render(label)
	}
	return s.tab.Render(label)
}

func humanBytesRate(bps float64) string {
	switch {
	case bps >= 1e9:
		return fmt.Sprintf("%.2f GB/s", bps/1e9)
	case bps >= 1e6:
		return fmt.Sprintf("%.2f MB/s", bps/1e6)
	case bps >= 1e3:
		return fmt.Sprintf("%.2f KB/s", bps/1e3)
	default:
		return fmt.Sprintf("%.0f B/s", bps)
	}
}
