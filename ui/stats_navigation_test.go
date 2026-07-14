package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lobis/eos-tui/eos"
)

func TestStatsDetailNavigationUsesVisibleFilteredTableRows(t *testing.T) {
	m := NewModel(nil, "local eos cli", "/").(model)
	m.activeView = viewNamespaceStats
	m.splash.active = false
	m.inspectorLoading = false
	m.statsSectionSelected = 5 // Inspector Users
	m.statsPaneFocus = statsFocusDetail

	for i := 0; i < 8; i++ {
		prefix := "drop"
		if i%2 == 0 {
			prefix = "keep"
		}
		m.inspectorStats.UserCosts = append(m.inspectorStats.UserCosts, eos.InspectorCostRecord{
			Name: fmt.Sprintf("%s-user-%02d", prefix, i),
			ID:   uint64(i),
		})
	}
	m.inspectorStats.TopUserCost = m.inspectorStats.UserCosts[0]

	if got := m.statsDetailLineCount(m.statsSections()); got != 8 {
		t.Fatalf("expected all 8 visible table rows to be navigable, got %d", got)
	}

	for range 20 {
		updated, _ := m.updateNamespaceStatsKeys(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(model)
	}
	if got := m.statsDetailSelected; got != 7 {
		t.Fatalf("expected repeated down navigation to reach final table row 7, got %d", got)
	}

	m.statsFilter.filters = map[int]string{0: "keep"}
	updated, _ := m.updateNamespaceStatsKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = updated.(model)

	if got := m.statsDetailLineCount(m.statsSections()); got != 4 {
		t.Fatalf("expected navigation count to use 4 filtered rows, got %d", got)
	}
	if got := m.statsDetailSelected; got != 3 {
		t.Fatalf("expected G to select final filtered row 3, got %d", got)
	}
}
