package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lobis/eos-tui/eos"
)

func TestNamespaceStatsNarrowLayoutUsesOneFocusedPane(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 60
	m.height = 20
	m.activeView = viewNamespaceStats
	m.commandLog.active = false
	m.splash.active = false
	m.fstStatsLoading = false
	m.nsStatsLoading = false
	m.inspectorLoading = false
	m.nodeStats = eos.NodeStats{State: "OK", FileCount: 42}
	m.namespaceStats = eos.NamespaceStats{MasterHost: "mgm01", TotalFiles: 42}

	list := m.renderNamespaceStatsView(9)
	if strings.Count(ansi.Strip(list), "General Statistics") != 1 || strings.Count(list, "┌") != 1 {
		t.Fatalf("narrow list view should render one complete pane:\n%s", list)
	}
	for _, line := range strings.Split(list, "\n") {
		if got := lipgloss.Width(line); got > m.contentWidth() {
			t.Fatalf("narrow list line width %d exceeds %d: %q", got, m.contentWidth(), ansi.Strip(line))
		}
	}
	listLines := strings.Split(list, "\n")
	if len(listLines) != 11 || !strings.Contains(listLines[len(listLines)-1], "└") {
		t.Fatalf("narrow scrollable pane lost its bottom border (lines=%d):\n%s", len(listLines), list)
	}

	m.statsPaneFocus = statsFocusDetail
	detail := m.renderNamespaceStatsView(9)
	if !strings.Contains(ansi.Strip(detail), "Health OK") || strings.Count(detail, "┌") != 1 {
		t.Fatalf("narrow detail view should render the selected section in one pane:\n%s", detail)
	}
}

func TestNamespaceStatsNarrowLayoutCanOpenAndCloseNonTableDetails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 60
	m.height = 20
	m.activeView = viewNamespaceStats
	m.commandLog.active = false
	m.splash.active = false
	m.fstStatsLoading = false
	m.nsStatsLoading = false
	m.inspectorLoading = false
	m.nodeStats = eos.NodeStats{State: "OK", FileCount: 42}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(model)
	if m.statsPaneFocus != statsFocusDetail {
		t.Fatal("right arrow did not retain detail focus for a non-table stats section")
	}
	if detail := ansi.Strip(m.renderNamespaceStatsView(9)); !strings.Contains(detail, "Health OK") {
		t.Fatalf("right arrow did not reveal non-table details:\n%s", detail)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(model)
	if m.statsPaneFocus != statsFocusList {
		t.Fatal("left arrow did not return from non-table details to the stats list")
	}
}

func TestInspectorDisabledUsesLongBackoffAndNoActiveError(t *testing.T) {
	m := NewModel(nil, "test", "/").(model)
	m.activeView = viewNamespaceStats
	m.inspectorErr = errors.New("inspector disabled")
	fixedRefreshTime := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	m.inspectorUpdated = fixedRefreshTime

	if m.inspectorAutoRefreshDue(fixedRefreshTime.Add(inspectorRefreshInterval)) {
		t.Fatal("disabled inspector was retried at the normal success interval")
	}
	if !m.inspectorAutoRefreshDue(fixedRefreshTime.Add(inspectorFailureRetryInterval)) {
		t.Fatal("disabled inspector was not retried after the failure backoff")
	}
	if got := m.activeViewErrorStatus(); strings.Contains(strings.ToLower(got), "inspector") {
		t.Fatalf("disabled optional inspector surfaced as active failure: %q", got)
	}
}
