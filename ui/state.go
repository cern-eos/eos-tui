package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const persistedUIStateFile = "ui-state.json"

type persistedUIState struct {
	NamespacePath     string `json:"namespace_path"`
	ActiveView        viewID `json:"active_view"`
	CommandLogVisible bool   `json:"command_log_visible"`
}

func defaultPersistedUIState() persistedUIState {
	return persistedUIState{
		ActiveView:        defaultActiveView(),
		CommandLogVisible: true,
	}
}

func normalizePersistedView(view viewID) viewID {
	switch view {
	case viewSpaceStatus:
		return viewSpaces
	case viewQDB:
		return viewMGM
	default:
		return view
	}
}

func loadPersistedUIState() persistedUIState {
	home, err := persistedUIStateHomeDir()
	if err != nil {
		return defaultPersistedUIState()
	}

	dir := filepath.Join(home, ".eos-tui")
	dirInfo, err := os.Lstat(dir)
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
		return defaultPersistedUIState()
	}
	path := filepath.Join(dir, persistedUIStateFile)
	fileInfo, err := os.Lstat(path)
	if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 {
		return defaultPersistedUIState()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultPersistedUIState()
	}

	var state persistedUIState
	if err := json.Unmarshal(data, &state); err != nil {
		return defaultPersistedUIState()
	}

	state.NamespacePath = cleanPath(state.NamespacePath)
	state.ActiveView = normalizePersistedView(state.ActiveView)
	if state.ActiveView < 0 || state.ActiveView >= viewCount {
		state.ActiveView = defaultActiveView()
	}
	return state
}

func savePersistedUIState(state persistedUIState) {
	home, err := persistedUIStateHomeDir()
	if err != nil {
		return
	}

	dir := filepath.Join(home, ".eos-tui")
	if err := ensurePrivateUIStateDir(dir); err != nil {
		return
	}

	state.NamespacePath = cleanPath(state.NamespacePath)
	state.ActiveView = normalizePersistedView(state.ActiveView)
	if state.ActiveView < 0 || state.ActiveView >= viewCount {
		state.ActiveView = defaultActiveView()
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}

	tmp, err := os.CreateTemp(dir, persistedUIStateFile+".*.tmp")
	if err != nil {
		return
	}

	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}

	finalPath := filepath.Join(dir, persistedUIStateFile)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return
	}
	cleanup = false
}

func ensurePrivateUIStateDir(dir string) error {
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return os.ErrInvalid
	}
	return os.Chmod(dir, 0700)
}

func (m model) persistedUIState() persistedUIState {
	activeView := m.activeView
	activeView = normalizePersistedView(activeView)
	return persistedUIState{
		NamespacePath:     m.directory.Path,
		ActiveView:        activeView,
		CommandLogVisible: m.commandLog.active,
	}
}

func (m model) persistUIState() {
	savePersistedUIState(m.persistedUIState())
}

func persistedUIStateHomeDir() (string, error) {
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	return os.UserHomeDir()
}
