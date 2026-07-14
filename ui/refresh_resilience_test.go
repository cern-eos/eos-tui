package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lobis/eos-tui/eos"
)

func TestAutoRefreshCanBePausedAndResumed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModelWithOptions(nil, "test", "/", ModelOptions{RefreshInterval: 12 * time.Second}).(model)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	m = updated.(model)
	if m.autoRefresh || !strings.Contains(m.status, "paused") {
		t.Fatalf("expected automatic refresh to pause, state=%v status=%q", m.autoRefresh, m.status)
	}
	if footer := ansi.Strip(m.renderFooter()); !strings.Contains(footer, "P resume") {
		t.Fatalf("paused footer should advertise resume: %q", footer)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	m = updated.(model)
	if !m.autoRefresh || !strings.Contains(m.status, "12s") {
		t.Fatalf("expected automatic refresh to resume at configured interval, state=%v status=%q", m.autoRefresh, m.status)
	}
}

func TestBackgroundRefreshDoesNotOverwriteActiveViewStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.activeView = viewFST
	m.status = "Viewing 2 FST"

	updated, _ := m.Update(namespaceStatsLoadedMsg{stats: eos.NamespaceStats{TotalFiles: 42}})
	m = updated.(model)
	if m.status != "Viewing 2 FST" {
		t.Fatalf("background Stats completion overwrote FST status: %q", m.status)
	}

	updated, _ = m.Update(fstsLoadedMsg{fsts: []eos.FstRecord{{Host: "fst01", Port: 1095}}})
	m = updated.(model)
	if m.status != "FST nodes updated" {
		t.Fatalf("active FST completion did not update status: %q", m.status)
	}
}

func TestInspectorFailuresAreRateLimited(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.activeView = viewNamespaceStats
	m.fstsLoading = false
	m.fileSystemsLoading = false
	m.fstStatsLoading = false
	m.nsStatsLoading = false

	updated, _ := m.Update(inspectorLoadedMsg{err: errors.New("inspector disabled")})
	m = updated.(model)
	lastAttempt := m.inspectorUpdated
	if lastAttempt.IsZero() {
		t.Fatalf("failed inspector attempt should be timestamped")
	}

	m, _ = m.autoRefreshActiveView(lastAttempt.Add(5 * time.Second))
	if m.inspectorLoading {
		t.Fatalf("inspector failure was retried before backoff elapsed")
	}
	m.fstsLoading = false
	m.fileSystemsLoading = false
	m.fstStatsLoading = false
	m.nsStatsLoading = false
	m, _ = m.autoRefreshActiveView(lastAttempt.Add(inspectorFailureRetryInterval))
	if !m.inspectorLoading {
		t.Fatalf("inspector was not retried after backoff elapsed")
	}
}

func TestAutoRefreshIsScopedToActiveViewAndSingleFlight(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.activeView = viewFST
	m.fstsLoading = false
	m.fileSystemsLoading = false
	m.spacesLoading = false
	m.nsStatsLoading = false
	m.fstStatsLoading = false

	refreshed, cmd := m.autoRefreshActiveView(time.Now())
	if cmd == nil || !refreshed.fstsLoading {
		t.Fatalf("expected active FST view to start one refresh")
	}
	if refreshed.fileSystemsLoading || refreshed.spacesLoading || refreshed.nsStatsLoading || refreshed.fstStatsLoading {
		t.Fatalf("inactive views were unexpectedly refreshed: %+v", refreshed)
	}

	_, cmd = refreshed.autoRefreshActiveView(time.Now())
	if cmd != nil {
		t.Fatalf("expected in-flight FST refresh to suppress overlap")
	}
}

func TestDirectoryRefreshIgnoresStaleResponse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/eos/current").(model)
	m.nsRequestID = 9
	m.nsRequestedPath = "/eos/current"
	m.nsLoading = true
	m.directory = eos.Directory{Path: "/eos/original"}

	updated, _ := m.Update(directoryLoadedMsg{
		requestID: 8,
		path:      "/eos/old",
		directory: eos.Directory{Path: "/eos/old"},
	})
	m = updated.(model)
	if m.directory.Path != "/eos/original" || !m.nsLoading {
		t.Fatalf("stale namespace result changed current request: path=%q loading=%v", m.directory.Path, m.nsLoading)
	}
}

func TestRefreshPreservesSelectedNodeAcrossReordering(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.fsts = []eos.FstRecord{
		{Type: "fst", Host: "fst-a", Port: 1095},
		{Type: "fst", Host: "fst-b", Port: 1095},
	}
	m.fstSelected = 1

	updated, _ := m.Update(fstsLoadedMsg{fsts: []eos.FstRecord{
		{Type: "fst", Host: "fst-b", Port: 1095},
		{Type: "fst", Host: "fst-a", Port: 1095},
	}})
	m = updated.(model)
	selected, ok := m.selectedNode()
	if !ok || selected.Host != "fst-b" || m.fstSelected != 0 {
		t.Fatalf("selection drifted after refresh: index=%d selected=%+v", m.fstSelected, selected)
	}
}

func TestFailedMGMRefreshKeepsCachedTopology(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.mgms = []eos.MgmRecord{{Host: "mgm-cache", Port: 1094, Role: "leader"}}
	m.mgmSelected = 0

	updated, _ := m.Update(mgmsLoadedMsg{err: errors.New("temporary timeout")})
	m = updated.(model)
	selected, ok := m.selectedTopologyHost()
	if !ok || selected.host != "mgm-cache" || len(m.mgms) != 1 {
		t.Fatalf("failed refresh discarded cached topology: %+v", m.mgms)
	}
}

func TestCachedRowsStayVisibleDuringRefreshAndTransientFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.fsts = []eos.FstRecord{{Type: "fst", Host: "fst-cache", Port: 1095}}
	m.fstsLoading = true
	out := ansi.Strip(m.renderFSTView(20))
	if !strings.Contains(out, "fst-cache") || strings.Contains(out, "Loading node list") {
		t.Fatalf("cached row disappeared while refreshing:\n%s", out)
	}

	m.fstsLoading = false
	m.fstsErr = errors.New("temporary timeout")
	out = ansi.Strip(m.renderFSTView(20))
	if !strings.Contains(out, "fst-cache") {
		t.Fatalf("cached row disappeared after transient failure:\n%s", out)
	}
}

func TestContextualHelpFitsNarrowTerminalAndWorksOverLogs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "ssh test", "/").(model)
	m.splash.active = false
	m.commandLog.active = false
	m.width = 60
	m.height = 24
	m.log.active = true
	m.log.title = "Test Log"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(model)
	if !m.helpActive {
		t.Fatalf("expected help to open over log viewer")
	}
	view := m.View()
	if !strings.Contains(ansi.Strip(view), "Keyboard Help") {
		t.Fatalf("help overlay is missing from rendered view:\n%s", view)
	}
	if got := lineCount(view); got != m.height {
		t.Fatalf("narrow help view height = %d, want %d", got, m.height)
	}
	for _, line := range strings.Split(view, "\n") {
		if width := lipgloss.Width(line); width > m.width {
			t.Fatalf("help line width %d exceeds terminal width %d: %q", width, m.width, ansi.Strip(line))
		}
	}
}

func TestVimRightKeyIsNotStolenFromViewsWithoutLogs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.activeView = viewGroups
	m.groups = []eos.GroupRecord{{Name: "default.0"}}
	m.groupsColumnSelected = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(model)
	if m.log.active || m.groupsColumnSelected != 1 {
		t.Fatalf("l should move the group column, log=%v column=%d", m.log.active, m.groupsColumnSelected)
	}
}

func TestIOAutoRefreshRespectsGlobalPause(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModelWithOptions(nil, "test", "/", ModelOptions{
		RefreshInterval:    37 * time.Second,
		DisableAutoRefresh: true,
	}).(model)
	m.activeView = viewIOShaping
	m.ioShapingLoading = false
	m.ioShapingPoliciesLoading = false
	m.ioShapingConfigLoading = false

	updated, next := m.Update(tickMsg(time.Now()))
	m = updated.(model)
	if next == nil {
		t.Fatalf("paused refresh should still schedule the next configured tick")
	}
	if m.ioShapingLoading || m.ioShapingPoliciesLoading || m.ioShapingConfigLoading {
		t.Fatalf("paused global refresh started IO work: traffic=%v policies=%v config=%v",
			m.ioShapingLoading, m.ioShapingPoliciesLoading, m.ioShapingConfigLoading)
	}
	if m.refreshInterval != 37*time.Second {
		t.Fatalf("refresh interval = %s, want 37s", m.refreshInterval)
	}

	m.autoRefresh = true
	updated, cmd := m.Update(tickMsg(time.Now()))
	m = updated.(model)
	if cmd == nil {
		t.Fatalf("enabled IO refresh should schedule its next tick and reload")
	}
	if !m.ioShapingLoading || !m.ioShapingPoliciesLoading || !m.ioShapingConfigLoading {
		t.Fatalf("enabled IO refresh did not mark all app-mode requests in flight: traffic=%v policies=%v config=%v",
			m.ioShapingLoading, m.ioShapingPoliciesLoading, m.ioShapingConfigLoading)
	}
}

func TestNamespaceNavigationLoadingBlocksCachedRowActions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tests := []struct {
		name string
		key  tea.KeyMsg
	}{
		{name: "enter directory", key: tea.KeyMsg{Type: tea.KeyRight}},
		{name: "edit attributes", key: tea.KeyMsg{Type: tea.KeyEnter}},
		{name: "create directory", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}},
		{name: "go to path", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}},
		{name: "move selection", key: tea.KeyMsg{Type: tea.KeyDown}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(nil, "test", "/eos/old").(model)
			m.directory = eos.Directory{
				Path: "/eos/old",
				Entries: []eos.Entry{
					{Kind: eos.EntryKindContainer, Name: "cached", Path: "/eos/old/cached"},
					{Kind: eos.EntryKindFile, Name: "also-cached", Path: "/eos/old/also-cached"},
				},
			}
			m.nsLoaded = true
			m.nsLoading = true
			m.nsRequestID = 9
			m.nsRequestedPath = "/eos/new"
			m.nsSelected = 0

			updated, cmd := m.updateNamespaceKeys(tt.key)
			m = updated.(model)
			if cmd != nil {
				t.Fatalf("stale cached-row action scheduled a command while navigation was loading")
			}
			if m.nsRequestID != 9 || m.nsRequestedPath != "/eos/new" || m.directory.Path != "/eos/old" {
				t.Fatalf("stale cached-row action changed navigation state: request=%d requested=%q shown=%q",
					m.nsRequestID, m.nsRequestedPath, m.directory.Path)
			}
			if m.nsSelected != 0 || m.nsAttrEdit.active || m.nsMkdir.active || m.nsGoTo.active {
				t.Fatalf("stale cached-row action mutated selection or opened an editor: selected=%d attr=%v mkdir=%v goto=%v",
					m.nsSelected, m.nsAttrEdit.active, m.nsMkdir.active, m.nsGoTo.active)
			}
			if !strings.Contains(m.status, "wait for the directory request") {
				t.Fatalf("blocked action should explain why it was ignored, status=%q", m.status)
			}
		})
	}
}

func TestIOAndVIDModeChangesClearStaleRows(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.ioShapingMode = eos.IOShapingApps
	m.ioShapingGeneration = 4
	m.ioShaping = []eos.IOShapingRecord{{}}
	m.ioShapingPressure = []eos.IOShapingPressureRecord{{}}
	m.ioShapingPolicies = []eos.IOShapingPolicyRecord{{}}
	m.ioShapingLoading = false
	m.ioShapingPoliciesLoading = false
	m.ioShapingConfigLoading = false

	updated, cmd := m.changeIOShapingMode(eos.IOShapingUsers)
	m = updated.(model)
	if cmd == nil {
		t.Fatalf("IO mode change should load the new scope")
	}
	if len(m.ioShaping) != 0 || len(m.ioShapingPressure) != 0 || len(m.ioShapingPolicies) != 0 {
		t.Fatalf("IO mode change retained stale rows: traffic=%d pressure=%d policies=%d",
			len(m.ioShaping), len(m.ioShapingPressure), len(m.ioShapingPolicies))
	}
	if m.ioShapingGeneration != 5 || !m.ioShapingLoading {
		t.Fatalf("IO mode change did not start a new generation: generation=%d loading=%v",
			m.ioShapingGeneration, m.ioShapingLoading)
	}

	m.vidMode = vidListDefault
	m.vidGeneration = 10
	m.vidRecords = []eos.VIDRecord{{Key: "stale-default", Value: "1"}}
	updated, cmd = m.changeVIDMode(vidListUsers)
	m = updated.(model)
	if cmd == nil || m.vidGeneration != 11 || len(m.vidRecords) != 0 {
		t.Fatalf("VID mode change did not clear and reload: generation=%d records=%v cmdNil=%v",
			m.vidGeneration, m.vidRecords, cmd == nil)
	}

	// Return to the original scope before its original request completes. The
	// mode alone now matches, so only the generation can reject this ABA result.
	m.vidRecords = []eos.VIDRecord{{Key: "stale-users", Value: "2"}}
	updated, _ = m.changeVIDMode(vidListDefault)
	m = updated.(model)
	if m.vidGeneration != 12 || len(m.vidRecords) != 0 {
		t.Fatalf("second VID mode change did not clear rows: generation=%d records=%v", m.vidGeneration, m.vidRecords)
	}
	updated, _ = m.Update(vidLoadedMsg{
		mode:       vidListDefault,
		generation: 10,
		records:    []eos.VIDRecord{{Key: "late-aba", Value: "wrong"}},
	})
	m = updated.(model)
	if !m.vidLoading || len(m.vidRecords) != 0 {
		t.Fatalf("stale ABA VID response was applied: loading=%v records=%v", m.vidLoading, m.vidRecords)
	}

	updated, _ = m.Update(vidLoadedMsg{
		mode:       vidListDefault,
		generation: 12,
		records:    []eos.VIDRecord{{Key: "current", Value: "ok"}},
	})
	m = updated.(model)
	if m.vidLoading || len(m.vidRecords) != 1 || m.vidRecords[0].Key != "current" {
		t.Fatalf("current VID response was not applied: loading=%v records=%v", m.vidLoading, m.vidRecords)
	}
}

func TestNamespaceAttrsRequestIDIgnoresSamePathABAResponse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.nsAttrsTargetPath = "/eos/same"
	m.nsAttrsRequestID = 4
	m.nsAttrsLoading = true
	m.nsAttrsLoaded = false
	m.nsAttrs = []eos.NamespaceAttr{{Key: "cached", Value: "keep"}}

	updated, _ := m.Update(namespaceAttrsLoadedMsg{
		path:      "/eos/same",
		requestID: 3,
		attrs:     []eos.NamespaceAttr{{Key: "late", Value: "wrong"}},
	})
	m = updated.(model)
	if !m.nsAttrsLoading || m.nsAttrsLoaded || len(m.nsAttrs) != 1 || m.nsAttrs[0].Key != "cached" {
		t.Fatalf("same-path stale attrs response changed current request: loading=%v loaded=%v attrs=%v",
			m.nsAttrsLoading, m.nsAttrsLoaded, m.nsAttrs)
	}

	updated, _ = m.Update(namespaceAttrsLoadedMsg{
		path:      "/eos/same",
		requestID: 4,
		attrs:     []eos.NamespaceAttr{{Key: "current", Value: "ok"}},
	})
	m = updated.(model)
	if m.nsAttrsLoading || !m.nsAttrsLoaded || len(m.nsAttrs) != 1 || m.nsAttrs[0].Key != "current" {
		t.Fatalf("current attrs response was not applied: loading=%v loaded=%v attrs=%v",
			m.nsAttrsLoading, m.nsAttrsLoaded, m.nsAttrs)
	}
}

func TestSpaceStatusRequestIDIgnoresSameTargetABAResponse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.spaceStatusTarget = "default"
	m.spaceStatusRequestID = 8
	m.spaceStatusLoading = true
	m.spaceStatus = []eos.SpaceStatusRecord{{Key: "cached", Value: "keep"}}

	updated, _ := m.Update(spaceStatusLoadedMsg{
		space:     "default",
		requestID: 7,
		records:   []eos.SpaceStatusRecord{{Key: "late", Value: "wrong"}},
	})
	m = updated.(model)
	if !m.spaceStatusLoading || len(m.spaceStatus) != 1 || m.spaceStatus[0].Key != "cached" {
		t.Fatalf("same-target stale space response changed current request: loading=%v records=%v",
			m.spaceStatusLoading, m.spaceStatus)
	}

	updated, _ = m.Update(spaceStatusLoadedMsg{
		space:     "default",
		requestID: 8,
		records:   []eos.SpaceStatusRecord{{Key: "current", Value: "ok"}},
	})
	m = updated.(model)
	if m.spaceStatusLoading || len(m.spaceStatus) != 1 || m.spaceStatus[0].Key != "current" {
		t.Fatalf("current space response was not applied: loading=%v records=%v", m.spaceStatusLoading, m.spaceStatus)
	}
}

func TestLogFilterAcceptsGlobalQuestionAndPauseKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	input := textinput.New()
	input.Focus()
	m := NewModel(nil, "test", "/").(model)
	m.autoRefresh = true
	m.log = logOverlay{
		active:    true,
		filtering: true,
		allLines:  []string{"?P", "other"},
		filtered:  []string{"?P", "other"},
		input:     input,
		vp:        viewport.New(80, 10),
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	m = updated.(model)

	if m.helpActive {
		t.Fatalf("? opened global help instead of entering the active log filter")
	}
	if !m.autoRefresh {
		t.Fatalf("P toggled global refresh instead of entering the active log filter")
	}
	if got := m.log.input.Value(); got != "?P" {
		t.Fatalf("log filter input = %q, want ?P", got)
	}
}

func TestLateSpaceConfigResultDoesNotSwitchCurrentSpace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.spaceStatusTarget = "space-b"
	m.spaceStatusRequestID = 9
	m.spaceStatus = []eos.SpaceStatusRecord{{Key: "space", Value: "space-b"}}
	m.edit = spaceStatusEdit{active: true, space: "space-b"}

	updated, cmd := m.Update(spaceConfigResultMsg{space: "space-a"})
	m = updated.(model)
	if cmd != nil {
		t.Fatal("late result for an old space scheduled a context-changing refresh")
	}
	if m.spaceStatusTarget != "space-b" || m.spaceStatusRequestID != 9 {
		t.Fatalf("late result switched space context: target=%q request=%d", m.spaceStatusTarget, m.spaceStatusRequestID)
	}
	if !m.edit.active || m.edit.space != "space-b" {
		t.Fatalf("late result disturbed the newer editor: %+v", m.edit)
	}
}

func TestMutationRefreshGenerationsRejectOlderInflightResponses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	t.Run("filesystem", func(t *testing.T) {
		m := NewModel(nil, "test", "/").(model)
		m.fileSystemsGeneration = 5
		m.fileSystemsLoading = true
		m.fileSystems = []eos.FileSystemRecord{{ID: 1, ConfigStatus: "rw"}}

		updated, cmd := m.Update(fsConfigStatusResultMsg{fsID: 1, value: "drain"})
		m = updated.(model)
		if cmd == nil || m.fileSystemsGeneration != 6 || !m.fileSystemsLoading {
			t.Fatalf("post-write filesystem refresh not started as a new generation: generation=%d loading=%v cmdNil=%v", m.fileSystemsGeneration, m.fileSystemsLoading, cmd == nil)
		}

		updated, _ = m.Update(fileSystemsLoadedMsg{generation: 5, fs: []eos.FileSystemRecord{{ID: 1, ConfigStatus: "rw"}}})
		m = updated.(model)
		if !m.fileSystemsLoading || m.fileSystems[0].ConfigStatus != "rw" {
			t.Fatalf("stale filesystem response changed current request state: loading=%v rows=%v", m.fileSystemsLoading, m.fileSystems)
		}

		updated, _ = m.Update(fileSystemsLoadedMsg{generation: 6, fs: []eos.FileSystemRecord{{ID: 1, ConfigStatus: "drain"}}})
		m = updated.(model)
		if m.fileSystemsLoading || m.fileSystems[0].ConfigStatus != "drain" {
			t.Fatalf("current filesystem response was not applied: loading=%v rows=%v", m.fileSystemsLoading, m.fileSystems)
		}
	})

	t.Run("node", func(t *testing.T) {
		m := NewModel(nil, "test", "/").(model)
		m.fstsGeneration = 7
		m.fileSystemsGeneration = 11
		m.fstsLoading = true
		m.fileSystemsLoading = true
		m.fsts = []eos.FstRecord{{Host: "fst01", Port: 1095, Status: "online"}}

		updated, cmd := m.Update(nodeStatusResultMsg{hostPort: "fst01:1095", status: "off"})
		m = updated.(model)
		if cmd == nil || m.fstsGeneration != 8 || m.fileSystemsGeneration != 12 {
			t.Fatalf("node mutation did not supersede both dependent refreshes: fst=%d fs=%d", m.fstsGeneration, m.fileSystemsGeneration)
		}
		updated, _ = m.Update(fstsLoadedMsg{generation: 7, fsts: []eos.FstRecord{{Host: "stale", Port: 1095}}})
		m = updated.(model)
		if !m.fstsLoading || m.fsts[0].Host != "fst01" {
			t.Fatalf("stale node response was applied: loading=%v rows=%v", m.fstsLoading, m.fsts)
		}
	})

	t.Run("group and access", func(t *testing.T) {
		m := NewModel(nil, "test", "/").(model)
		m.groupsGeneration = 3
		m.accessGeneration = 4
		m.groupsLoading = true
		m.accessLoading = true

		updated, groupCmd := m.Update(groupSetResultMsg{group: "default.0", status: "drain"})
		m = updated.(model)
		updated, accessCmd := m.Update(accessActionResultMsg{target: "ban user 1000"})
		m = updated.(model)
		if groupCmd == nil || accessCmd == nil || m.groupsGeneration != 4 || m.accessGeneration != 5 {
			t.Fatalf("post-write list generations not advanced: group=%d access=%d", m.groupsGeneration, m.accessGeneration)
		}
	})
}

func TestFirstVIDRequestNeverUsesWildcardGeneration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	if m.vidGeneration == 0 {
		t.Fatal("new model initialized VID request generation as wildcard zero")
	}
	m.activeView = viewVID
	m.vidLoading = true

	updated, _ := m.Update(vidLoadedMsg{
		mode:       m.vidMode,
		generation: 0,
		records:    []eos.VIDRecord{{Key: "late", Value: "wrong"}},
	})
	m = updated.(model)
	if !m.vidLoading || len(m.vidRecords) != 0 {
		t.Fatalf("wildcard VID response was accepted: loading=%v records=%v", m.vidLoading, m.vidRecords)
	}
}

func TestTurningLogTailOffDoesNotWedgeManualRefresh(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.logGeneration = 6
	m.log = logOverlay{
		active:   true,
		tailing:  true,
		inFlight: true,
		filePath: "/var/log/eos/mgm/xrdlog.mgm",
		source:   "/var/log/eos/mgm/xrdlog.mgm",
		input:    textinput.New(),
		vp:       viewport.New(80, 10),
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = updated.(model)
	if cmd != nil || m.log.tailing || m.log.inFlight {
		t.Fatalf("tail-off should stop the stream and clear in-flight state: cmdNil=%v tailing=%v inFlight=%v",
			cmd == nil, m.log.tailing, m.log.inFlight)
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(model)
	if cmd == nil || !m.log.inFlight {
		t.Fatalf("manual refresh remained wedged after tail-off: cmdNil=%v inFlight=%v", cmd == nil, m.log.inFlight)
	}
}

func TestStatsCachedContentRemainsVisibleWhileRefreshing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(nil, "test", "/").(model)
	m.width = 140
	m.height = 32
	m.splash.active = false
	m.activeView = viewNamespaceStats
	m.fstStatsLoading = true
	m.nsStatsLoading = true
	m.nodeStats = eos.NodeStats{State: "CACHED-OK", FileCount: 424242, DirCount: 31337}
	m.namespaceStats = eos.NamespaceStats{
		MasterHost:       "cached-master:1094",
		TotalFiles:       424242,
		TotalDirectories: 31337,
	}

	view := ansi.Strip(m.renderNamespaceStatsView(24))
	for _, needle := range []string{"CACHED-OK", "cached-master:1094", "424242"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("cached stats %q disappeared during refresh:\n%s", needle, view)
		}
	}
	for _, staleLoading := range []string{"Loading cluster summary...", "Loading namespace statistics..."} {
		if strings.Contains(view, staleLoading) {
			t.Fatalf("cached stats were replaced by %q during refresh:\n%s", staleLoading, view)
		}
	}
}
