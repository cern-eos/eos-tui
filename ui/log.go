package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// selectedHostForView returns the hostname of the currently selected row in the
// active view, or empty string when no row is selected or the view has no host.
func (m model) selectedHostForView() string {
	switch m.activeView {
	case viewMGM, viewQDB:
		rows := m.topologySelectableRows()
		if m.mgmSelected >= 0 && m.mgmSelected < len(rows) {
			return rows[m.mgmSelected].host
		}
	case viewFST:
		fsts := m.visibleFSTs()
		if m.fstSelected >= 0 && m.fstSelected < len(fsts) {
			return fsts[m.fstSelected].Host
		}
	case viewFileSystems:
		fss := m.visibleFileSystems()
		if m.fsSelected >= 0 && m.fsSelected < len(fss) {
			return fss[m.fsSelected].Host
		}
	}
	return ""
}

func fstRTLogQueue(host string, port int) string {
	return fmt.Sprintf("/eos/%s:%d/fst", host, port)
}

func rtlogSourceLabel(queue string, seconds int, tag string) string {
	return fmt.Sprintf("eos rtlog %s %d %s", queue, seconds, tag)
}

func mgmLogTargets(host string) []logTarget {
	const rtlogSeconds = 600
	const rtlogTag = "info"

	return []logTarget{
		{
			title:      "MGM Log",
			source:     rtlogSourceLabel(".", rtlogSeconds, rtlogTag),
			host:       host,
			rtlogQueue: ".",
			rtlogTag:   rtlogTag,
			rtlogSecs:  rtlogSeconds,
		},
		{
			title:    "MGM xrdlog",
			source:   "/var/log/eos/mgm/xrdlog.mgm",
			host:     host,
			filePath: "/var/log/eos/mgm/xrdlog.mgm",
		},
		{
			title:    "MGM Traffic Shaping",
			source:   "/var/log/eos/mgm/TrafficShaping.log",
			host:     host,
			filePath: "/var/log/eos/mgm/TrafficShaping.log",
		},
		{
			title:    "MGM Balancer",
			source:   "/var/log/eos/mgm/Balancer.log",
			host:     host,
			filePath: "/var/log/eos/mgm/Balancer.log",
		},
		{
			title:    "MGM Converter",
			source:   "/var/log/eos/mgm/Converter.log",
			host:     host,
			filePath: "/var/log/eos/mgm/Converter.log",
		},
		{
			title:    "MGM Drain",
			source:   "/var/log/eos/mgm/Drain.log",
			host:     host,
			filePath: "/var/log/eos/mgm/Drain.log",
		},
	}
}

func (m model) logTargetsForView() ([]logTarget, bool) {
	const rtlogSeconds = 600
	const rtlogTag = "info"

	switch m.activeView {
	case viewMGM, viewQDB:
		rows := m.topologySelectableRows()
		if m.mgmSelected < 0 || m.mgmSelected >= len(rows) {
			return nil, false
		}
		selected := rows[m.mgmSelected]
		if selected.kind == topologyHostQDB {
			host := selected.host
			if host == "" {
				return nil, false
			}
			return []logTarget{{
				title:    "QDB Log",
				source:   "/var/log/eos/quarkdb/xrdlog.quarkdb",
				host:     host,
				filePath: "/var/log/eos/quarkdb/xrdlog.quarkdb",
			}}, true
		}
		host := selected.host
		if host == "" {
			return nil, false
		}
		return mgmLogTargets(host), true
	case viewFST, viewFileSystems:
		host := m.selectedHostForView()
		if host == "" {
			return nil, false
		}
		port := 0
		if m.activeView == viewFST {
			fsts := m.visibleFSTs()
			if m.fstSelected < 0 || m.fstSelected >= len(fsts) {
				return nil, false
			}
			port = fsts[m.fstSelected].Port
		} else {
			fss := m.visibleFileSystems()
			if m.fsSelected < 0 || m.fsSelected >= len(fss) {
				return nil, false
			}
			port = int(fss[m.fsSelected].Port)
		}
		queue := fstRTLogQueue(host, port)
		return []logTarget{{
			title:      "FST Log",
			source:     rtlogSourceLabel(queue, rtlogSeconds, rtlogTag),
			host:       host,
			rtlogQueue: queue,
			rtlogTag:   rtlogTag,
			rtlogSecs:  rtlogSeconds,
		}}, true
	default:
		return nil, false
	}
}

func (m model) logTargetForView() (logTarget, bool) {
	targets, ok := m.logTargetsForView()
	if !ok || len(targets) == 0 {
		return logTarget{}, false
	}
	return targets[0], true
}

func logTitleWithHost(target logTarget) string {
	if target.host == "" {
		return target.title
	}
	return fmt.Sprintf("%s  [%s]", target.title, target.host)
}

func (m model) currentLogTarget() logTarget {
	return logTarget{
		title:      m.log.title,
		source:     m.log.source,
		host:       m.log.host,
		filePath:   m.log.filePath,
		rtlogQueue: m.log.rtlogQueue,
		rtlogTag:   m.log.rtlogTag,
		rtlogSecs:  m.log.rtlogSecs,
	}
}

func (m model) logSourceLabel() string {
	return fallback(m.log.source, m.log.filePath)
}

func (m model) setActiveLogTarget(target logTarget, loading bool) model {
	m.log.host = target.host
	m.log.filePath = target.filePath
	m.log.source = target.source
	m.log.rtlogQueue = target.rtlogQueue
	m.log.rtlogTag = target.rtlogTag
	m.log.rtlogSecs = target.rtlogSecs
	m.log.title = logTitleWithHost(target)
	m.log.err = nil
	m.log.notice = ""
	m.log.loading = loading
	m.log.inFlight = loading
	m.log.allLines = nil
	m.log.filtered = nil
	m.log.vp.SetContent("Loading...")
	return m
}

func (m model) switchLogSource(delta int) (tea.Model, tea.Cmd) {
	if len(m.log.logSources) < 2 {
		return m, nil
	}
	m.log.sourceIndex = (m.log.sourceIndex + delta + len(m.log.logSources)) % len(m.log.logSources)
	target := m.log.logSources[m.log.sourceIndex]
	m.logGeneration++
	m = m.setActiveLogTarget(target, true)
	if m.log.tailing {
		return m, tea.Batch(
			loadLogCmd(m.client, target, m.logGeneration),
			logTickCmd(m.logGeneration),
		)
	}
	return m, loadLogCmd(m.client, target, m.logGeneration)
}

func (m model) openLogOverlay() (tea.Model, tea.Cmd) {
	if m.client == nil {
		return m, nil
	}
	targets, ok := m.logTargetsForView()
	if !ok || len(targets) == 0 {
		// Views without a supported log target do not open the log overlay.
		return m, nil
	}
	target := targets[0]
	m.logGeneration++

	logInput := textinput.New()
	logInput.Prompt = "grep> "
	logInput.CharLimit = 256

	vp := viewport.New(m.contentWidth()-4, max(4, m.height-10))
	vp.SetContent("Loading...")

	m.log = logOverlay{
		active:      true,
		logSources:  targets,
		sourceIndex: 0,
		tailing:     true,
		wrap:        true,
		vp:          vp,
		input:       logInput,
	}
	m = m.setActiveLogTarget(target, true)
	return m, tea.Batch(
		loadLogCmd(m.client, target, m.logGeneration),
		logTickCmd(m.logGeneration),
	)
}

func (m model) logViewportWidth() int {
	width := m.contentWidth()
	if !m.log.plain {
		// In lipgloss v1, Width(w) includes padding but NOT borders.  We call
		// Width(contentWidth-2) so the outer panel = contentWidth.  The actual
		// inner content area = (contentWidth-2) - 2*padding = contentWidth - 4.
		width -= 4
	}
	return max(1, width)
}

func renderWrappedLogLines(lines []string, width int, wrap bool) []string {
	if width <= 0 {
		return append([]string(nil), lines...)
	}

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if wrap {
			wrapped := ansi.Hardwrap(line, width, true)
			out = append(out, strings.Split(wrapped, "\n")...)
		} else {
			// Truncate so long lines never overflow the panel and eat the right border.
			out = append(out, ansi.Truncate(line, width, ""))
		}
	}
	return out
}

func (m *model) refreshLogViewportContent(preserveOffset bool) {
	width := m.logViewportWidth()
	wasAtBottom := m.log.vp.AtBottom()
	prevOffset := m.log.vp.YOffset
	m.log.vp.Width = width
	rendered := renderWrappedLogLines(m.log.filtered, width, m.log.wrap)
	m.log.vp.SetContent(strings.Join(rendered, "\n"))
	if !preserveOffset {
		return
	}
	if wasAtBottom {
		m.log.vp.GotoBottom()
		return
	}
	maxOffset := max(0, m.log.vp.TotalLineCount()-m.log.vp.Height)
	m.log.vp.SetYOffset(min(prevOffset, maxOffset))
}

func (m model) renderLogOverlay(height int) string {
	width := m.contentWidth()
	vpWidth := m.logViewportWidth()
	hasCachedContent := len(m.log.allLines) > 0

	// Keep viewport sized to available space.
	filterHeight := 0
	if m.log.filtering {
		filterHeight = 2
	}
	maxBoxViewportHeight := max(1, height-3-filterHeight) // title line + border (2) [+ filter input block]
	vpHeight := maxBoxViewportHeight
	if m.log.plain {
		vpHeight = max(4, height-filterHeight)
	} else if m.log.err != nil && !m.log.loading && !hasCachedContent {
		vpHeight = 1
	}
	// Always use the full available height so the panel fills the screen
	// even when only a few log lines have been loaded.
	m.log.vp.Width = vpWidth
	m.log.vp.Height = vpHeight
	m.log.vp.SetYOffset(m.log.vp.YOffset)

	if m.log.plain {
		lines := []string{m.renderLogViewport()}
		if m.log.filtering {
			lines = append(lines, "", m.log.input.View())
		}
		return strings.Join(lines, "\n")
	}

	// Title bar.
	filterInfo := ""
	if m.log.filter != "" {
		filterInfo = fmt.Sprintf("  [grep: %q  %d lines]", m.log.filter, len(m.log.filtered))
	}
	totalInfo := fmt.Sprintf("  %d lines", len(m.log.allLines))
	if m.log.loading {
		totalInfo = "  loading..."
	} else if m.log.notice != "" {
		totalInfo = "  " + m.log.notice
	} else if m.log.err != nil {
		if hasCachedContent {
			totalInfo = "  reload failed; showing cached lines"
		} else {
			totalInfo = "  failed to load log"
		}
	}
	titleLine := m.styles.popupTitle.Render(m.log.title) +
		m.styles.label.Render("  "+fallback(m.log.source, m.log.filePath)) +
		m.styles.value.Render(totalInfo+filterInfo)
	// Ensure the title line never overflows the inner panel width — if it does,
	// lipgloss v1 silently expands the panel box, shifting the right border off screen.
	titleLine = padVisibleWidth(titleLine, vpWidth)

	lines := []string{titleLine}

	if m.log.err != nil && !m.log.loading && !hasCachedContent {
		// Truncate error messages so they don't overflow the panel width.
		lines = append(lines, padVisibleWidth(m.styles.error.Render(ansi.Truncate(m.log.err.Error(), vpWidth, "…")), vpWidth))
	} else {
		lines = append(lines, m.renderLogViewport())
	}

	if m.log.filtering {
		lines = append(lines, "", m.log.input.View())
	}

	inner := strings.Join(lines, "\n")
	// In lipgloss v1, Width() sets content+padding width; borders are added on
	// top.  With Padding(0,1) and NormalBorder the border contributes 2 extra
	// chars (left+right), so the rendered outer width = Width + 2.  We want
	// outer = m.contentWidth() so that the log block fits the body area exactly.
	panel := m.styles.panel.Width(width - 2).Render(inner)
	panelHeight := lipgloss.Height(panel)
	if panelHeight >= height {
		return panel
	}
	padding := make([]string, 0, height-panelHeight)
	for i := 0; i < height-panelHeight; i++ {
		padding = append(padding, strings.Repeat(" ", width))
	}
	return strings.Join(append(padding, strings.Split(panel, "\n")...), "\n")
}

func (m model) renderLogViewport() string {
	w := m.logViewportWidth()
	lines := renderWrappedLogLines(m.log.filtered, w, m.log.wrap)
	top := max(0, min(m.log.vp.YOffset, len(lines)))
	bottom := min(top+m.log.vp.Height, len(lines))

	var visible []string
	if top < bottom {
		visible = lines[top:bottom]
	}

	// Prepend blank lines so content is bottom-aligned within the viewport.
	// This keeps the panel box full-height while the newest lines sit
	// naturally at the bottom, matching tail(1) behaviour.
	for len(visible) < m.log.vp.Height {
		visible = append([]string{""}, visible...)
	}

	// Explicitly pad every line to the viewport width so that lipgloss draws
	// the right border correctly even when lines are shorter than the panel.
	for i, line := range visible {
		padded := padVisibleWidth(line, w)
		if strings.TrimSpace(padded) == "" {
			// Keep blank viewport rows as styled spans so the desktop renderer
			// preserves their full width instead of collapsing them.
			visible[i] = m.styles.status.Render(padded)
		} else {
			visible[i] = padded
		}
	}

	return strings.Join(visible, "\n")
}

// applyLogFilter returns lines that case-insensitively contain filter.
// Empty filter returns all lines.
func applyLogFilter(lines []string, filter string) []string {
	if filter == "" {
		return lines
	}
	lower := strings.ToLower(filter)
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.Contains(strings.ToLower(l), lower) {
			out = append(out, l)
		}
	}
	return out
}
