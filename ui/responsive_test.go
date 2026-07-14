package ui

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lobis/eos-tui/eos"
)

func assertResponsiveLinesFit(t *testing.T, rendered string, width int) {
	t.Helper()
	for _, line := range strings.Split(rendered, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("rendered line width %d exceeds available width %d: %q", got, width, ansi.Strip(line))
		}
	}
}

func assertNormalPanelBordersPaired(t *testing.T, rendered string) {
	t.Helper()
	plain := ansi.Strip(rendered)
	for _, pair := range [][2]string{{"┌", "└"}, {"┐", "┘"}} {
		opening := strings.Count(plain, pair[0])
		closing := strings.Count(plain, pair[1])
		if opening == 0 || opening != closing {
			t.Fatalf("unpaired panel borders %q=%d %q=%d:\n%s", pair[0], opening, pair[1], closing, plain)
		}
	}
}

func TestResponsiveChromeFitsAndPreservesActiveTab(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	for _, width := range []int{16, 32, 40} {
		for _, tab := range orderedViewTabs {
			t.Run(tab.label+"/width-"+strconv.Itoa(width), func(t *testing.T) {
				m := NewModel(nil, "ssh gateway.example.invalid -> root@mgm.example.invalid", "/").(model)
				m.width = width
				m.activeView = tab.view
				m.status = "Ready"

				header := m.renderHeader()
				footer := m.renderFooter()
				assertResponsiveLinesFit(t, header, m.contentWidth())
				assertResponsiveLinesFit(t, footer, m.contentWidth())

				// Every current tab label, including its active-style padding, can fit
				// in the smallest content width. The compact header should therefore
				// keep the selected view identifiable even if it drops other chrome.
				if lipgloss.Width(m.styles.tabActive.Render(tab.label)) <= m.contentWidth() &&
					!strings.Contains(ansi.Strip(header), tab.label) {
					t.Fatalf("compact header lost active tab %q at terminal width %d: %q", tab.label, width, ansi.Strip(header))
				}
			})
		}
	}
}

func TestCompactStartupSplashFitsAtSixteenColumns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 16
	m.height = 20
	m.splash.active = true
	m.fstStatsLoading = true
	m.nodeStats = eos.NodeStats{}

	view := m.View()
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "EOS TUI") || !strings.Contains(plain, "╭") || !strings.Contains(plain, "╯") {
		t.Fatalf("compact startup splash lost identity or borders:\n%s", plain)
	}
	if strings.Contains(plain, "████") {
		t.Fatalf("16-column startup used clipped full ASCII art:\n%s", plain)
	}
	if got := lineCount(view); got != m.height {
		t.Fatalf("compact splash rendered %d lines, want %d", got, m.height)
	}
	assertResponsiveLinesFit(t, view, m.width)
}

func TestVeryNarrowViewPrioritizesDataOverCommandHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 16
	m.height = 20
	m.activeView = viewFileSystems
	m.splash.active = false
	m.commandLog.active = true
	m.fileSystemsLoading = false
	m.fileSystems = []eos.FileSystemRecord{{
		ID:           1,
		Host:         "an-intentionally-long-fst-hostname.example.invalid",
		Path:         "/an/intentionally/long/filesystem/mount/path",
		SchedGroup:   "an-intentionally-long-scheduling-group",
		ConfigStatus: "rw",
		Health:       "a deliberately long health description",
	}}

	view := m.View()
	plain := ansi.Strip(view)
	if strings.Contains(plain, "Recent") {
		t.Fatalf("very narrow view sacrificed active data rows to command history:\n%s", plain)
	}
	if !strings.Contains(plain, "1") {
		t.Fatalf("very narrow view lost active filesystem data:\n%s", plain)
	}
	if got := lineCount(view); got != m.height {
		t.Fatalf("very narrow view rendered %d lines, want %d", got, m.height)
	}
	assertResponsiveLinesFit(t, view, m.width)
	assertNormalPanelBordersPaired(t, view)
}

func TestPanelNormalizationClampsPhysicalRowsBeforeBordering(t *testing.T) {
	m := model{styles: newStyles()}
	lines := []string{
		m.renderSectionTitle("An intentionally overlong section title", 8),
		m.styles.error.Render("first error line\nsecond error line"),
	}
	rendered := normalizePanelLines(lines, 8, 2)
	rows := strings.Split(rendered, "\n")
	if len(rows) != 2 {
		t.Fatalf("normalized panel has %d physical rows, want 2:\n%s", len(rows), ansi.Strip(rendered))
	}
	for _, row := range rows {
		if got := lipgloss.Width(row); got != 8 {
			t.Fatalf("normalized row width = %d, want 8: %q", got, ansi.Strip(row))
		}
	}
	if got := lipgloss.Width(m.renderSectionTitle("Selected Filesystem", 8)); got > 8 {
		t.Fatalf("section title width = %d, want <= 8", got)
	}
}

func TestCompactHelpFallbackFitsAtSixteenByTwenty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 16
	m.height = 20
	m.activeView = viewNamespace
	m.splash.active = false
	m.helpActive = true

	view := m.View()
	plain := ansi.Strip(view)
	for _, needle := range []string{"Help", "4", "Resize", "Esc", "╭", "╯"} {
		if !strings.Contains(plain, needle) {
			t.Fatalf("16x20 compact help clipped %q:\n%s", needle, plain)
		}
	}
	if strings.Contains(plain, "Current view actions") {
		t.Fatalf("oversized full help was rendered at 16x20:\n%s", plain)
	}
	if got := lineCount(view); got != m.height {
		t.Fatalf("16x20 compact help rendered %d lines, want %d", got, m.height)
	}
	assertResponsiveLinesFit(t, view, m.width)
}

func TestContextualHelpAtSixtyByTwentyKeepsActionsAndCloseGuidance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "ssh test", "/eos/test").(model)
	m.width = 60
	m.height = 20
	m.activeView = viewNamespace
	m.splash.active = false
	m.commandLog.active = false
	m.nsLoading = false
	m.nsLoaded = true
	m.directory = eos.Directory{Path: "/eos/test"}
	m.helpActive = true

	view := m.View()
	plain := ansi.Strip(view)
	for _, needle := range []string{
		"Keyboard Help",
		"4 Namespace",
		"Current view actions",
		"open",
		"mkdir",
		"attributes",
		"? / esc / enter close",
	} {
		if !strings.Contains(plain, needle) {
			t.Fatalf("60x20 contextual help clipped %q:\n%s", needle, plain)
		}
	}
	if got := lineCount(view); got != m.height {
		t.Fatalf("60x20 help rendered %d lines, want %d", got, m.height)
	}
	assertResponsiveLinesFit(t, view, m.width)
}

func TestContextualHelpGlobalShortcutsStayOnOneLineAtSixtyColumns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "ssh test", "/").(model)
	m.width = 60
	m.height = 20
	m.activeView = viewNamespaceStats
	m.splash.active = false
	m.commandLog.active = false
	m.helpActive = true

	plain := ansi.Strip(m.renderHelpOverlay())
	if !strings.Contains(plain, "r refresh  •  P auto  •  L commands  •  ? help") {
		t.Fatalf("global refresh shortcuts wrapped or disappeared at 60 columns:\n%s", plain)
	}
}

func TestBlockingPopupUsesCommandPanelRowsAtSixtyByTwenty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 60
	m.height = 20
	m.activeView = viewFileSystems
	m.splash.active = false
	m.commandLog.active = true
	m.fileSystemsLoading = false
	m.fileSystems = []eos.FileSystemRecord{{ID: 1, Path: "/data/fst.1", ConfigStatus: "rw"}}
	m.fsEdit = fsConfigStatusEdit{
		active:  true,
		fsID:    1,
		fsPath:  "/data/fst.1",
		current: "rw",
	}

	view := m.View()
	plain := ansi.Strip(view)
	for _, needle := range []string{"Set configstatus", "enter apply", "╭", "╰"} {
		if !strings.Contains(plain, needle) {
			t.Fatalf("60x20 filesystem popup clipped %q:\n%s", needle, plain)
		}
	}
	if strings.Contains(plain, "Recent commands") {
		t.Fatalf("command panel remained visible behind blocking popup:\n%s", plain)
	}
	if got := lineCount(view); got != m.height {
		t.Fatalf("60x20 popup rendered %d lines, want %d", got, m.height)
	}
	assertResponsiveLinesFit(t, view, m.width)
}

func TestFilterPopupFitsAtSixtyByTwentyWithCommandPanelEnabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 60
	m.height = 20
	m.activeView = viewFileSystems
	m.splash.active = false
	m.commandLog.active = true
	m.fileSystemsLoading = false
	m.fileSystems = []eos.FileSystemRecord{{ID: 1, Path: "/data/fst.1", ConfigStatus: "rw"}}
	m.openFilterPopup()

	popup := m.renderFilterPopup()
	popupLines := strings.Split(ansi.Strip(popup), "\n")
	if !strings.Contains(popupLines[len(popupLines)-1], "└") || !strings.Contains(popupLines[len(popupLines)-1], "┘") {
		t.Fatalf("filter popup lost its bottom border:\n%s", ansi.Strip(popup))
	}
	assertResponsiveLinesFit(t, popup, m.contentWidth())

	view := m.View()
	plain := ansi.Strip(view)
	for _, needle := range []string{"Filter", "Enter apply selected value", "└", "┘"} {
		if !strings.Contains(plain, needle) {
			t.Fatalf("60x20 filter popup clipped %q:\n%s", needle, plain)
		}
	}
	if strings.Contains(plain, "Recent commands") {
		t.Fatalf("command panel remained visible behind filter popup:\n%s", plain)
	}
	assertResponsiveLinesFit(t, view, m.width)
}

func TestFilterPopupKeepsBordersAtSixteenColumns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 16
	m.height = 20
	m.activeView = viewFileSystems
	m.fileSystems = []eos.FileSystemRecord{{ID: 1, Host: "fst01", ConfigStatus: "rw"}}
	m.openFilterPopup()

	popup := m.renderFilterPopup()
	lines := strings.Split(ansi.Strip(popup), "\n")
	if !strings.Contains(lines[0], "┌") || !strings.Contains(lines[0], "┐") ||
		!strings.Contains(lines[len(lines)-1], "└") || !strings.Contains(lines[len(lines)-1], "┘") {
		t.Fatalf("16-column filter popup lost a border:\n%s", ansi.Strip(popup))
	}
	assertResponsiveLinesFit(t, popup, m.contentWidth())
}

func TestCommandPanelKeepsBothBordersAtSixteenColumns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 16
	m.commandLog.lines = []string{"eos -j node ls"}

	panel := m.renderCommandPanel(6)
	lines := strings.Split(ansi.Strip(panel), "\n")
	if !strings.Contains(lines[0], "┌") || !strings.Contains(lines[0], "┐") ||
		!strings.Contains(lines[len(lines)-1], "└") || !strings.Contains(lines[len(lines)-1], "┘") {
		t.Fatalf("16-column command panel lost a border:\n%s", ansi.Strip(panel))
	}
	assertResponsiveLinesFit(t, panel, m.contentWidth())
}

func TestMutationModalsLayOutBeforeClippingAtSixteenColumns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 16
	m.nodeStatus = nodeStatusConfirm{host: "fst.example.invalid", port: 1095, current: "on", target: "off", command: "eos node config fst.example.invalid:1095 status=off"}
	m.qdbCoup = qdbCoupConfirm{host: "qdb.example.invalid", command: "redis-cli -p 7777 raft-attempt-coup"}
	m.nsAttrEdit = namespaceAttrEdit{targetPath: "/eos/very/long/path", attrs: []eos.NamespaceAttr{{Key: "sys.acl", Value: "u:1000:rwx"}}}

	for name, popup := range map[string]string{
		"node":      m.renderNodeStatusConfirmPopup(),
		"qdb":       m.renderQDBCoupConfirmPopup(),
		"attribute": m.renderNamespaceAttrEditPopup(),
	} {
		t.Run(name, func(t *testing.T) {
			lines := strings.Split(ansi.Strip(popup), "\n")
			if !strings.Contains(lines[0], "╭") || !strings.Contains(lines[0], "╮") ||
				!strings.Contains(lines[len(lines)-1], "╰") || !strings.Contains(lines[len(lines)-1], "╯") {
				t.Fatalf("16-column %s popup lost a border:\n%s", name, ansi.Strip(popup))
			}
			assertResponsiveLinesFit(t, popup, m.contentWidth())
		})
	}
}

func TestOversizedMutationModalShowsSafeResizeStateAtSixteenByTwenty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 16
	m.height = 20
	m.activeView = viewFST
	m.splash.active = false
	m.commandLog.active = true
	m.fstsLoading = false
	m.nodeStatus = nodeStatusConfirm{
		active:  true,
		host:    "fst.example.invalid",
		port:    1095,
		current: "on",
		target:  "off",
		command: "eos node config fst.example.invalid:1095 status=off",
	}

	view := m.View()
	plain := ansi.Strip(view)
	for _, needle := range []string{"Resize", "Small", "Esc", "╰", "╯"} {
		if !strings.Contains(plain, needle) {
			t.Fatalf("16x20 resize state clipped %q:\n%s", needle, plain)
		}
	}
	if strings.Contains(plain, "Recent commands") {
		t.Fatalf("command panel remained visible behind resize state:\n%s", plain)
	}
	if got := lineCount(view); got != m.height {
		t.Fatalf("16x20 resize state rendered %d lines, want %d", got, m.height)
	}
	assertResponsiveLinesFit(t, view, m.width)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd != nil || !m.nodeStatus.active || !strings.Contains(strings.ToLower(m.status), "resize") {
		t.Fatalf("hidden enter action was not blocked: active=%v cmdNil=%v status=%q", m.nodeStatus.active, cmd == nil, m.status)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.nodeStatus.active {
		t.Fatal("escape did not cancel oversized mutation modal")
	}
}

func TestFooterUsesActiveViewFreshness(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 120
	m.status = "Ready"
	m.fstsLoading = false
	m.groupsLoading = false
	now := time.Now()
	m.lastRefreshAt[viewFST] = now.Add(-2 * time.Minute)
	m.lastRefreshAt[viewGroups] = now

	m.activeView = viewFST
	fstFooter := ansi.Strip(m.renderFooter())
	if !strings.Contains(fstFooter, "updated 2m ago") {
		t.Fatalf("FST footer did not use FST freshness: %q", fstFooter)
	}

	m.activeView = viewGroups
	groupsFooter := ansi.Strip(m.renderFooter())
	if !strings.Contains(groupsFooter, "updated now") {
		t.Fatalf("Groups footer did not use Groups freshness: %q", groupsFooter)
	}
	if strings.Contains(groupsFooter, "updated 2m ago") {
		t.Fatalf("Groups footer leaked FST freshness: %q", groupsFooter)
	}
}

func TestFooterPrioritizesOnlyTheActiveViewError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 160
	m.activeView = viewFST
	m.fstsLoading = false
	m.status = "Last operation completed"
	m.fileSystemsErr = errors.New("unrelated filesystem failure")

	footer := ansi.Strip(m.renderFooter())
	if !strings.Contains(footer, "Last operation completed") {
		t.Fatalf("inactive-view error replaced current status: %q", footer)
	}
	if strings.Contains(footer, "unrelated filesystem failure") {
		t.Fatalf("inactive filesystem error leaked into FST footer: %q", footer)
	}

	m.fstsErr = errors.New("node list unavailable")
	footer = ansi.Strip(m.renderFooter())
	for _, needle := range []string{"FST refresh failed", "node list unavailable"} {
		if !strings.Contains(footer, needle) {
			t.Fatalf("active-view error footer missing %q: %q", needle, footer)
		}
	}
	if strings.Contains(footer, "Last operation completed") {
		t.Fatalf("generic status obscured active-view error: %q", footer)
	}
}

func TestNewModelStartsMGMTopologyInLoadingState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	if !m.mgmsLoading {
		t.Fatal("new model should mark the initial MGM topology request as loading")
	}
}

func TestCachedStatsRemainVisibleDuringRefresh(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 120
	m.height = 30
	m.activeView = viewNamespaceStats
	m.splash.active = false
	m.commandLog.active = false
	m.fstStatsLoading = true
	m.nsStatsLoading = true
	m.inspectorLoading = true
	m.nodeStats = eos.NodeStats{State: "CACHED-OK", FileCount: 4242, DirCount: 42}
	m.namespaceStats = eos.NamespaceStats{MasterHost: "cached-master", TotalFiles: 5151, TotalDirectories: 51}
	m.inspectorStats = eos.InspectorStats{AvgFileSize: 4096, LayoutCount: 7}

	plain := ansi.Strip(m.renderNamespaceStatsView(22))
	for _, needle := range []string{"CACHED-OK", "4242", "cached-master", "5151", "7 layouts"} {
		if !strings.Contains(plain, needle) {
			t.Fatalf("cached statistics disappeared during refresh; missing %q:\n%s", needle, plain)
		}
	}
	for _, stalePlaceholder := range []string{
		"Loading cluster summary",
		"Loading namespace statistics",
		"Loading inspector statistics",
	} {
		if strings.Contains(plain, stalePlaceholder) {
			t.Fatalf("refresh replaced cached statistics with %q:\n%s", stalePlaceholder, plain)
		}
	}
}
