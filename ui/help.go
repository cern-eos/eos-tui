package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderHelpOverlay() string {
	// Lipgloss Width excludes the border but includes horizontal padding.
	// Reserve the border cells so the popup never relies on clipping to fit.
	width := min(82, max(1, m.contentWidth()-2))
	lineWidth := max(1, width-4)
	line := func(value string) string {
		return lipgloss.NewStyle().Width(lineWidth).Render(value)
	}
	title := truncate(fmt.Sprintf("Keyboard Help — %s", m.activeViewLabel()), max(1, lineWidth-2))
	lines := []string{
		m.styles.popupTitle.Render(title),
		m.renderSectionTitle("Global", lineWidth),
		line("tab/shift+tab views  •  0-9 jump  •  q quit"),
		line("↑↓/j/k move  •  g/G first/last  •  ctrl+u/d page"),
		line("←→ pane/column  •  / filter  •  S sort"),
		line("r refresh  •  P auto  •  L commands  •  ? help"),
		m.renderSectionTitle("Current view actions", lineWidth),
		line(m.activeViewHelp()),
		m.styles.status.Render("? / esc / enter close"),
	}

	return m.styles.panel.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 2).
		Width(width).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (m model) renderCompactHelpOverlay() string {
	return m.renderModal([]string{
		m.styles.popupTitle.Render("Help"),
		m.activeViewLabel(),
		"",
		"Resize",
		m.styles.status.Render("Esc"),
	}, lipgloss.Color("62"), 0)
}

func (m model) activeViewLabel() string {
	for _, tab := range orderedViewTabs {
		if tab.view == m.activeView {
			return tab.label
		}
	}
	return "Unknown"
}

func (m model) activeViewHelp() string {
	switch m.activeView {
	case viewMGM, viewQDB:
		return "c coup (QDB only)  •  l logs  •  s shell"
	case viewFST:
		return "o toggle node on/off  •  l logs  •  s shell"
	case viewFileSystems:
		return "enter set configstatus  •  A bulk set  •  x Apollon drain"
	case viewNamespace:
		return "→ open  •  ←/backspace parent  •  : goto  •  m mkdir  •  enter/a attributes"
	case viewSpaces:
		return "enter open status  •  enter edit selected status value"
	case viewNamespaceStats:
		return "←→ switch list/detail  •  ←→ select detail column"
	case viewIOShaping:
		return "a/u/g/n/p mode  •  N new policy  •  enter edit  •  d delete  •  m limits"
	case viewGroups:
		return "enter set status  •  A set all visible groups"
	case viewVID:
		return "←→ change VID scope"
	case viewAccess:
		return "enter action  •  s set stall  •  c clear focused filter"
	default:
		return "No additional actions for this view."
	}
}
