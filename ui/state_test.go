package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lobis/eos-tui/eos"
)

func TestNewModelRestoresPersistedUIStateWhenRootPathEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	stateDir := filepath.Join(home, ".eos-tui")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}

	state := persistedUIState{
		NamespacePath:     "/eos/dev",
		ActiveView:        viewGroups,
		CommandLogVisible: true,
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, persistedUIStateFile), data, 0644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	m := NewModel(nil, "local eos cli", "").(model)
	if m.directory.Path != "/eos/dev" {
		t.Fatalf("expected restored namespace path, got %q", m.directory.Path)
	}
	if m.activeView != viewGroups {
		t.Fatalf("expected restored active view %d, got %d", viewGroups, m.activeView)
	}
	if !m.commandLog.active {
		t.Fatalf("expected restored command panel visibility")
	}
}

func TestNewModelDefaultsToEosPathWhenNoPersistedStateExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	m := NewModel(nil, "local eos cli", "").(model)
	if m.directory.Path != "/eos" {
		t.Fatalf("expected default namespace path /eos, got %q", m.directory.Path)
	}
	if !m.commandLog.active {
		t.Fatalf("expected command panel to start open by default")
	}
}

func TestNewModelIgnoresPersistedStateWhenRootPathProvided(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	savePersistedUIState(persistedUIState{
		NamespacePath:     "/persisted",
		ActiveView:        viewGroups,
		CommandLogVisible: true,
	})

	m := NewModel(nil, "local eos cli", "/explicit").(model)
	if m.directory.Path != "/explicit" {
		t.Fatalf("expected explicit path to win, got %q", m.directory.Path)
	}
	if m.activeView != viewNamespaceStats {
		t.Fatalf("expected default active view when explicit path is provided, got %d", m.activeView)
	}
	if !m.commandLog.active {
		t.Fatalf("expected command panel to start open by default when explicit path is provided")
	}
}

func TestPersistedStateUpdatesOnDirectoryLoadViewChangeAndCommandPanelToggle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	m := NewModel(nil, "local eos cli", "/").(model)

	updated, _ := m.Update(directoryLoadedMsg{directory: eos.Directory{Path: "/eos/dev"}})
	m = updated.(model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	m = updated.(model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	m = updated.(model)

	state := loadPersistedUIState()
	if state.NamespacePath != "/eos/dev" {
		t.Fatalf("expected persisted namespace path /eos/dev, got %q", state.NamespacePath)
	}
	if state.ActiveView != viewGroups {
		t.Fatalf("expected persisted active view %d, got %d", viewGroups, state.ActiveView)
	}
	if state.CommandLogVisible {
		t.Fatalf("expected persisted command panel visibility to reflect the toggled closed state")
	}
}

func TestLoadPersistedUIStateMigratesDeprecatedSpaceStatusView(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	savePersistedUIState(persistedUIState{
		NamespacePath:     "/eos/dev",
		ActiveView:        viewSpaceStatus,
		CommandLogVisible: true,
	})

	state := loadPersistedUIState()
	if state.ActiveView != viewSpaces {
		t.Fatalf("expected deprecated space status view to migrate to spaces, got %d", state.ActiveView)
	}
}

func TestLoadPersistedUIStateMigratesDeprecatedQDBView(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	savePersistedUIState(persistedUIState{
		NamespacePath:     "/eos/dev",
		ActiveView:        viewQDB,
		CommandLogVisible: true,
	})

	state := loadPersistedUIState()
	if state.ActiveView != viewMGM {
		t.Fatalf("expected deprecated qdb view to migrate to mgm, got %d", state.ActiveView)
	}
}

func TestLoadPersistedUIStateIgnoresCorruptFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	stateDir := filepath.Join(home, ".eos-tui")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, persistedUIStateFile), []byte("{bad json"), 0644); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	state := loadPersistedUIState()
	if state.NamespacePath != "" || state.ActiveView != viewNamespaceStats || !state.CommandLogVisible {
		t.Fatalf("expected zero-value state for corrupt file, got %+v", state)
	}
}

func TestSavePersistedUIStateDoesNotLeaveTempFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	savePersistedUIState(persistedUIState{
		NamespacePath:     "/eos/dev",
		ActiveView:        viewGroups,
		CommandLogVisible: true,
	})

	stateDir := filepath.Join(home, ".eos-tui")
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}

	foundState := false
	for _, entry := range entries {
		if entry.Name() == persistedUIStateFile {
			foundState = true
		}
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("expected no leftover temp files, found %q", entry.Name())
		}
	}
	if !foundState {
		t.Fatalf("expected persisted state file to exist")
	}
}

func TestSavePersistedUIStateUsesPrivatePermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	savePersistedUIState(persistedUIState{ActiveView: viewNamespaceStats})
	dir := filepath.Join(home, ".eos-tui")
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat state directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("state directory mode = %o, want 700", got)
	}
	fileInfo, err := os.Stat(filepath.Join(dir, persistedUIStateFile))
	if err != nil {
		t.Fatalf("stat state file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("state file mode = %o, want 600", got)
	}
}

func TestPersistedUIStateRejectsSymlinkedStateDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	external := filepath.Join(t.TempDir(), "external")
	if err := os.Mkdir(external, 0755); err != nil {
		t.Fatalf("mkdir external state dir: %v", err)
	}
	malicious := persistedUIState{NamespacePath: "/unexpected", ActiveView: viewGroups, CommandLogVisible: false}
	data, err := json.Marshal(malicious)
	if err != nil {
		t.Fatalf("marshal external state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(external, persistedUIStateFile), data, 0644); err != nil {
		t.Fatalf("write external state: %v", err)
	}
	if err := os.Symlink(external, filepath.Join(home, ".eos-tui")); err != nil {
		t.Fatalf("symlink state dir: %v", err)
	}

	if got := loadPersistedUIState(); got != defaultPersistedUIState() {
		t.Fatalf("loaded state through symlink: %+v", got)
	}
	savePersistedUIState(persistedUIState{NamespacePath: "/eos/safe", ActiveView: viewVID})
	info, err := os.Stat(external)
	if err != nil {
		t.Fatalf("stat external state dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0755 {
		t.Fatalf("external state directory permissions changed to %04o", got)
	}
	externalData, err := os.ReadFile(filepath.Join(external, persistedUIStateFile))
	if err != nil {
		t.Fatalf("read external state: %v", err)
	}
	if string(externalData) != string(data) {
		t.Fatalf("save followed state-directory symlink: got %q want %q", externalData, data)
	}
}
