package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lobis/eos-tui/eos"
)

type ModelOptions struct {
	IdleTimeout        time.Duration
	RefreshInterval    time.Duration
	DisableAutoRefresh bool
}

func NewModel(client *eos.Client, endpoint, rootPath string) tea.Model {
	return NewModelWithOptions(client, endpoint, rootPath, ModelOptions{})
}

func NewModelWithOptions(client *eos.Client, endpoint, rootPath string, opts ModelOptions) tea.Model {
	refreshEvery := opts.RefreshInterval
	if refreshEvery <= 0 {
		refreshEvery = defaultRefreshInterval
	}
	input := textinput.New()
	input.Prompt = "filter> "
	input.CharLimit = 256
	input.Width = 40
	input.Focus()

	popupTable := table.New(
		table.WithColumns([]table.Column{{Title: "value", Width: 40}}),
		table.WithRows(nil),
		table.WithFocused(true),
		table.WithHeight(8),
	)

	state := defaultPersistedUIState()
	activeView := defaultActiveView()
	commandLogVisible := true
	if rootPath == "" {
		state = loadPersistedUIState()
		activeView = state.ActiveView
		commandLogVisible = state.CommandLogVisible
	}
	activeView = normalizePersistedView(activeView)
	initialPath := rootPath
	if initialPath == "" {
		initialPath = state.NamespacePath
	}
	if initialPath == "" {
		initialPath = "/eos"
	}
	now := time.Now()
	ioGeneration := uint64(0)
	if activeView == viewIOShaping {
		ioGeneration = 1
	}
	// Generations are never zero in production. Zero remains available only to
	// legacy unit messages; treating a real first request as a wildcard would
	// allow an old response to win after a scope ABA transition.
	vidGeneration := uint64(1)
	ioPolicyLoading := activeView == viewIOShaping && ioShapingModeHasPolicies(eos.IOShapingApps)
	ioConfigLoading := activeView == viewIOShaping
	commandGeneration := uint64(0)
	if commandLogVisible {
		commandGeneration = 1
	}

	return model{
		client:             client,
		endpoint:           endpoint,
		idleTimeout:        opts.IdleTimeout,
		lastActivity:       now,
		refreshInterval:    refreshEvery,
		autoRefresh:        !opts.DisableAutoRefresh,
		width:              120,
		height:             32,
		activeView:         activeView,
		fstStatsLoading:    true,
		fstsLoading:        true,
		fileSystemsLoading: true,
		mgmsLoading:        true,
		spacesLoading:      true,
		nsStatsLoading:     true,
		inspectorLoading:   true,
		nsLoading:          activeView == viewNamespace,
		spaceStatusLoading: false,
		groupsLoading:      activeView == viewGroups,
		ioShapingLoading:   activeView == viewIOShaping,
		vidLoading:         activeView == viewVID,
		accessLoading:      activeView == viewAccess,
		directory: eos.Directory{
			Path: cleanPath(initialPath),
		},
		nsRequestID:              1,
		nsRequestedPath:          cleanPath(initialPath),
		ioShapingGeneration:      ioGeneration,
		vidGeneration:            vidGeneration,
		nodeStatsGeneration:      1,
		fstsGeneration:           1,
		fileSystemsGeneration:    1,
		mgmsGeneration:           1,
		mgmVersionsGeneration:    1,
		spacesGeneration:         1,
		groupsGeneration:         1,
		accessGeneration:         1,
		namespaceStatsGeneration: 1,
		inspectorGeneration:      1,
		ioShapingPoliciesLoading: ioPolicyLoading,
		ioShapingConfigLoading:   ioConfigLoading,
		commandLogGeneration:     commandGeneration,
		status:                   "Loading EOS state...",
		fstColumnSelected:        int(fstFilterHost),
		fsColumnSelected:         int(fsFilterHost),
		groupsColumnSelected:     int(groupFilterName),
		fstSort:                  sortState{column: int(fstSortNone)},
		fsSort:                   sortState{column: int(fsSortNone)},
		spaceSort:                sortState{column: int(spaceSortNone)},
		groupSort:                sortState{column: int(groupSortNone)},
		fstFilter:                filterState{filters: map[int]string{}},
		fsFilter:                 filterState{filters: map[int]string{}},
		nsFilter:                 filterState{filters: map[int]string{}},
		spaceFilter:              filterState{filters: map[int]string{}},
		groupFilter:              filterState{filters: map[int]string{}},
		accessFilter:             filterState{filters: map[int]string{}},
		statsFilter:              filterState{filters: map[int]string{}},
		popup: filterPopup{
			input: input,
			table: popupTable,
		},
		commandLog: commandPanel{
			active:   commandLogVisible,
			loading:  commandLogVisible,
			inFlight: commandLogVisible,
		},
		splash: startupSplash{
			active: true,
		},
		styles: newStyles(),
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{checkEOSCmd(m.client), loadInfraCmd(m.client, m.infraGenerations()), tickCmd(m.refreshInterval), splashTickCmd()}
	if m.idleTimeout > 0 {
		cmds = append(cmds, idleTickCmd(m.idleTimeout))
	}
	switch m.activeView {
	case viewNamespace:
		cmds = append(cmds, loadDirectoryCmd(m.client, m.nsRequestedPath, m.nsRequestID))
	case viewGroups:
		cmds = append(cmds, loadGroupsCmd(m.client, m.groupsGeneration))
	case viewIOShaping:
		cmds = append(cmds,
			loadIOShapingViewCmd(m.client, m.ioShapingMode, m.ioShapingGeneration),
			loadIOShapingPolicyDataCmd(m.client, m.ioShapingMode, m.ioShapingGeneration),
		)
	case viewVID:
		cmds = append(cmds, loadVIDCmd(m.client, m.vidMode, m.vidGeneration))
	case viewAccess:
		cmds = append(cmds, loadAccessCmd(m.client, m.accessGeneration))
	}
	if m.commandLog.active {
		cmds = append(cmds, loadCommandHistoryCmd(m.client, m.commandLogGeneration), commandLogTickCmd(m.commandLogGeneration))
	}
	return tea.Batch(cmds...)
}

func (m model) infraGenerations() infraGenerations {
	return infraGenerations{
		nodeStats:      m.nodeStatsGeneration,
		fsts:           m.fstsGeneration,
		mgms:           m.mgmsGeneration,
		fileSystems:    m.fileSystemsGeneration,
		spaces:         m.spacesGeneration,
		namespaceStats: m.namespaceStatsGeneration,
		inspector:      m.inspectorGeneration,
	}
}

func bumpGeneration(value *uint64) uint64 {
	*value++
	if *value == 0 {
		*value = 1
	}
	return *value
}

func (m *model) startNodeStatsLoad() tea.Cmd {
	m.fstStatsLoading = true
	return loadNodeStatsCmd(m.client, bumpGeneration(&m.nodeStatsGeneration))
}

func (m *model) startFSTLoad() tea.Cmd {
	m.fstsLoading = true
	return loadFSTsCmd(m.client, bumpGeneration(&m.fstsGeneration))
}

func (m *model) startFileSystemsLoad() tea.Cmd {
	m.fileSystemsLoading = true
	return loadFileSystemsCmd(m.client, bumpGeneration(&m.fileSystemsGeneration))
}

func (m *model) startMGMLoad() tea.Cmd {
	m.mgmsLoading = true
	return loadMGMsCmd(m.client, bumpGeneration(&m.mgmsGeneration))
}

func (m *model) startMGMVersionsReload() tea.Cmd {
	m.mgmVersionsLoading = true
	return reloadMGMVersionsCmd(m.client, bumpGeneration(&m.mgmVersionsGeneration))
}

func (m *model) startSpacesLoad() tea.Cmd {
	m.spacesLoading = true
	return loadSpacesCmd(m.client, bumpGeneration(&m.spacesGeneration))
}

func (m *model) startGroupsLoad() tea.Cmd {
	m.groupsLoading = true
	return loadGroupsCmd(m.client, bumpGeneration(&m.groupsGeneration))
}

func (m *model) startAccessLoad() tea.Cmd {
	m.accessLoading = true
	return loadAccessCmd(m.client, bumpGeneration(&m.accessGeneration))
}

func (m *model) startNamespaceStatsLoad() tea.Cmd {
	m.nsStatsLoading = true
	return loadNamespaceStatsCmd(m.client, bumpGeneration(&m.namespaceStatsGeneration))
}

func (m *model) startInspectorLoad() tea.Cmd {
	m.inspectorLoading = true
	return loadInspectorCmd(m.client, bumpGeneration(&m.inspectorGeneration))
}

func (m model) toggleCommandLog() (tea.Model, tea.Cmd) {
	m.commandLogGeneration++
	m.commandLog.active = !m.commandLog.active
	m.persistUIState()
	if !m.commandLog.active {
		m.commandLog.loading = false
		m.commandLog.inFlight = false
		return m, nil
	}

	m.commandLog.loading = true
	m.commandLog.inFlight = true
	m.commandLog.err = nil
	return m, tea.Batch(
		loadCommandHistoryCmd(m.client, m.commandLogGeneration),
		commandLogTickCmd(m.commandLogGeneration),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
		m.lastActivity = time.Now()
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		if m.log.active && m.log.wrap {
			m.refreshLogViewportContent(true)
		}
		return m, tea.ClearScreen
	case tea.KeyMsg:
		if m.helpActive {
			switch msg.String() {
			case "?", "esc", "q", "ctrl+c", "enter":
				m.helpActive = false
			}
			return m, nil
		}
		if m.blockingOverlayNeedsResize() {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			if msg.String() != "esc" {
				m.status = "Resize the terminal to continue, or press esc to cancel"
				return m, nil
			}
		}
		// Log overlay intercepts all keys when active.
		if m.log.active {
			if !m.log.filtering {
				switch msg.String() {
				case "?":
					m.helpActive = true
					return m, nil
				case "P":
					m.autoRefresh = !m.autoRefresh
					if m.autoRefresh {
						m.status = fmt.Sprintf("Automatic refresh enabled (%s)", m.refreshInterval)
					} else {
						m.status = "Automatic refresh paused"
					}
					return m, nil
				}
			}
			return m.updateLogKeys(msg)
		}
		if m.alert.active {
			if m.alert.fatal {
				return m, tea.Quit
			}
			if msg.String() == "enter" || msg.String() == "esc" {
				m.alert.active = false
			}
			return m, nil
		}
		if m.accessAction.active {
			return m.updateAccessActionKeys(msg)
		}
		if m.nodeStatus.active {
			return m.updateNodeStatusKeys(msg)
		}
		if m.popup.active {
			return m.updatePopup(msg)
		}
		if m.nsAttrEdit.active {
			return m.updateNamespaceAttrEditKeys(msg)
		}
		if m.nsGoTo.active {
			return m.updateNamespaceGoToKeys(msg)
		}
		if m.nsMkdir.active {
			return m.updateNamespaceMkdirKeys(msg)
		}
		if m.ioShapingEdit.active {
			return m.updateIOShapingPolicyEditKeys(msg)
		}
		if m.edit.active {
			return m.updateSpaceStatusEditKeys(msg)
		}
		if m.groupDrain.active {
			return m.updateGroupDrainKeys(msg)
		}
		if m.apollon.active {
			return m.updateApollonDrainKeys(msg)
		}
		if m.qdbCoup.active {
			return m.updateQDBCoupKeys(msg)
		}
		if m.qdbCoupDone.active {
			return m.updateQDBCoupResultKeys(msg)
		}
		if m.fsEdit.active {
			return m.updateFSConfigStatusEditKeys(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "?":
			m.helpActive = true
			return m, nil
		case "P":
			m.autoRefresh = !m.autoRefresh
			if m.autoRefresh {
				m.status = fmt.Sprintf("Automatic refresh enabled (%s)", m.refreshInterval)
			} else {
				m.status = "Automatic refresh paused"
			}
			return m, nil
		case "esc":
			if m.activeView == viewSpaces && m.spaceStatusActive {
				m.spaceStatusActive = false
				m.status = "Returned to spaces list"
				return m, nil
			}
			switch m.activeView {
			case viewFST:
				if len(m.fstFilter.filters) > 0 {
					m.fstFilter.filters = map[int]string{}
					m.status = "Node filters cleared"
				}
			case viewFileSystems:
				if len(m.fsFilter.filters) > 0 {
					m.fsFilter.filters = map[int]string{}
					m.status = "Filesystem filters cleared"
				}
			case viewSpaces:
				if len(m.spaceFilter.filters) > 0 {
					m.spaceFilter.filters = map[int]string{}
					m.spacesSelected = clampIndex(m.spacesSelected, len(m.visibleSpaces()))
					m.status = "Space filters cleared"
				}
			case viewNamespace:
				if len(m.nsFilter.filters) > 0 {
					m.nsFilter.filters = map[int]string{}
					m.nsSelected = clampIndex(m.nsSelected, len(m.visibleNamespaceEntries()))
					m.status = "Namespace filters cleared"
				}
			case viewGroups:
				if len(m.groupFilter.filters) > 0 {
					m.groupFilter.filters = map[int]string{}
					m.groupsSelected = clampIndex(m.groupsSelected, len(m.visibleGroups()))
					m.status = "Group filters cleared"
				}
			case viewNamespaceStats:
				if len(m.statsFilter.filters) > 0 {
					m.statsFilter.filters = map[int]string{}
					m.statsDetailSelected = 0
					m.statsDetailOffsetX = 0
					m.statsDetailOffsetY = 0
					m.status = "Stats detail filter cleared"
				} else if m.statsPaneFocus == statsFocusDetail || m.statsDetailOffsetX > 0 || m.statsDetailOffsetY > 0 {
					m.statsPaneFocus = statsFocusList
					m.statsDetailSelected = 0
					m.statsDetailOffsetX = 0
					m.statsDetailOffsetY = 0
					m.status = "Returned to stats section list"
				}
			case viewAccess:
				if len(m.accessFilter.filters) > 0 {
					m.accessFilter.filters = map[int]string{}
					m.accessSelected = clampIndex(m.accessSelected, len(m.visibleAccessRecords()))
					m.status = "Access filters cleared"
				}
			}
			return m, nil
		case "tab":
			m.activeView = nextOrderedView(m.activeView, 1)
			return m.onViewChanged()
		case "shift+tab":
			m.activeView = nextOrderedView(m.activeView, -1)
			return m.onViewChanged()
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			m.activeView, _ = viewForHotkey(msg.String())
			return m.onViewChanged()
		case "r":
			return m.refreshActiveView()
		case "L":
			return m.toggleCommandLog()
		case "l":
			if _, ok := m.logTargetsForView(); ok {
				return m.openLogOverlay()
			}
		case "s":
			if m.activeView == viewAccess {
				break
			}
			return m.openShell()
		}

		switch m.activeView {
		case viewMGM, viewQDB:
			return m.updateMGMKeys(msg)
		case viewFST:
			return m.updateFSTKeys(msg)
		case viewFileSystems:
			return m.updateFileSystemKeys(msg)
		case viewNamespace:
			return m.updateNamespaceKeys(msg)
		case viewSpaces:
			if m.spaceStatusActive {
				if msg.String() == "enter" {
					return m.startSpaceStatusEdit()
				}
				return m.updateSpaceStatusKeys(msg)
			}
			return m.updateSpacesKeys(msg)
		case viewNamespaceStats:
			return m.updateNamespaceStatsKeys(msg)
		case viewSpaceStatus:
			if msg.String() == "enter" {
				return m.startSpaceStatusEdit()
			}
			return m.updateSpaceStatusKeys(msg)
		case viewIOShaping:
			return m.updateIOShapingKeys(msg)
		case viewGroups:
			return m.updateGroupKeys(msg)
		case viewVID:
			return m.updateVIDKeys(msg)
		case viewAccess:
			return m.updateAccessKeys(msg)
		}
	case mgmsLoadedMsg:
		if msg.generation != 0 && msg.generation != m.mgmsGeneration {
			return m, nil
		}
		selectedHost, keepSelectedHost := m.selectedTopologyHost()
		m.mgmsLoading = false
		m.mgmsErr = msg.err
		if msg.err == nil {
			m.mgms = mergeMGMVersionData(msg.mgms, m.mgms)
			m.mgmSelected = restoreSelection(m.topologySelectableRows(), m.mgmSelected, keepSelectedHost, func(row topologyHostRow) bool {
				return sameTopologyHost(row, selectedHost)
			})
			m.mgmVersionsLoaded = !hasMissingMGMVersions(m.mgms)
			m.markRefreshed("MGM/QDB topology updated", viewMGM, viewQDB)
		}
		versionsDue := m.mgmVersionsUpdated.IsZero() || time.Since(m.mgmVersionsUpdated) >= mgmVersionRefreshInterval
		if msg.err == nil && !m.mgmVersionsLoading && versionsDue {
			probeTargets := mgmVersionProbeTargets(m.mgms)
			if len(probeTargets) > 0 {
				m.mgmVersionsLoaded = false
				m.mgmVersionsLoading = true
				generation := bumpGeneration(&m.mgmVersionsGeneration)
				return m, loadMGMVersionsCmd(m.client, probeTargets, generation)
			}
		}
		return m, nil

	case mgmVersionsLoadedMsg:
		if msg.generation != 0 && msg.generation != m.mgmVersionsGeneration {
			return m, nil
		}
		m.mgmVersionsLoading = false
		m.mgmVersionsErr = msg.err
		m.mgmVersionsUpdated = time.Now()
		m.mgms = applyMGMVersions(m.mgms, msg.mgmVersions, msg.qdbVersions)
		m.mgmVersionsLoaded = !hasMissingMGMVersions(m.mgms)
		if msg.err != nil {
			m.setStatusForViews(fmt.Sprintf("Loaded MGM/QDB topology with partial versions: %v", msg.err), viewMGM, viewQDB)
		} else if len(msg.mgmVersions) > 0 || len(msg.qdbVersions) > 0 {
			m.markRefreshed("MGM/QDB versions updated", viewMGM, viewQDB)
		}
		return m, nil

	case infraLoadedMsg:
		selectedNode, keepSelectedNode := m.selectedNode()
		selectedFS, keepSelectedFS := m.selectedFileSystem()
		selectedHost, keepSelectedHost := m.selectedTopologyHost()
		m.fstStatsLoading = false
		m.fstsLoading = false
		m.mgmsLoading = false
		m.fileSystemsLoading = false
		if msg.eosVersion != "" {
			m.eosVersion = msg.eosVersion
		}
		// Apply per-component results independently so a failure in one
		// component does not hide data from the others.
		m.nodeStatsErr = msg.statsErr
		if msg.statsErr == nil {
			m.nodeStats = msg.stats
		}
		m.fstsErr = msg.fstsErr
		if msg.fstsErr == nil {
			m.fsts = msg.fsts
			m.fstSelected = restoreSelection(m.visibleFSTs(), m.fstSelected, keepSelectedNode, func(node eos.FstRecord) bool {
				return node.Host == selectedNode.Host && node.Port == selectedNode.Port
			})
		}
		m.mgmsErr = msg.mgmsErr
		if msg.mgmsErr == nil {
			m.mgms = msg.mgms
			m.mgmSelected = restoreSelection(m.topologySelectableRows(), m.mgmSelected, keepSelectedHost, func(row topologyHostRow) bool {
				return sameTopologyHost(row, selectedHost)
			})
		}
		m.fileSystemsErr = msg.fsErr
		if msg.fsErr == nil {
			m.fileSystems = msg.fs
			m.fsSelected = restoreSelection(m.visibleFileSystems(), m.fsSelected, keepSelectedFS, func(fs eos.FileSystemRecord) bool {
				return fs.ID == selectedFS.ID
			})
		}
		// Legacy single-error path (early-return failures).
		if msg.err != nil {
			if m.nodeStatsErr == nil {
				m.nodeStatsErr = msg.err
			}
			if m.fstsErr == nil {
				m.fstsErr = msg.err
			}
			if m.mgmsErr == nil {
				m.mgmsErr = msg.err
			}
			if m.fileSystemsErr == nil {
				m.fileSystemsErr = msg.err
			}
			m.setStatusForViews(fmt.Sprintf("Infrastructure refresh failed: %v", msg.err), viewNamespaceStats, viewFST, viewFileSystems, viewMGM, viewQDB)
		} else {
			m.markRefreshed(fmt.Sprintf("Connected to %s", m.endpoint), viewNamespaceStats, viewFST, viewFileSystems, viewMGM, viewQDB)
		}
	case eosVersionLoadedMsg:
		if msg.version != "" {
			m.eosVersion = msg.version
		}
	case nodeStatsLoadedMsg:
		if msg.generation != 0 && msg.generation != m.nodeStatsGeneration {
			return m, nil
		}
		m.fstStatsLoading = false
		m.nodeStatsErr = msg.err
		if msg.err != nil {
			m.setStatusForViews(fmt.Sprintf("Cluster summary refresh failed: %v", msg.err), viewNamespaceStats)
		} else {
			m.nodeStats = msg.stats
			m.nodeStats.State = m.computeClusterHealth()
			m.markRefreshed("Cluster summary updated", viewNamespaceStats)
		}
	case fstsLoadedMsg:
		if msg.generation != 0 && msg.generation != m.fstsGeneration {
			return m, nil
		}
		selectedNode, keepSelectedNode := m.selectedNode()
		m.fstsLoading = false
		m.fstsErr = msg.err
		if msg.err != nil {
			m.setStatusForViews(fmt.Sprintf("Node list refresh failed: %v", msg.err), viewFST, viewNamespaceStats)
		} else {
			m.fsts = msg.fsts
			m.fstSelected = restoreSelection(m.visibleFSTs(), m.fstSelected, keepSelectedNode, func(node eos.FstRecord) bool {
				return node.Host == selectedNode.Host && node.Port == selectedNode.Port
			})
			m.markRefreshed("FST nodes updated", viewFST, viewNamespaceStats)
		}
		m.nodeStats.State = m.computeClusterHealth()
	case nodeStatusResultMsg:
		if msg.err != nil {
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("node set failed for %s: %v", msg.hostPort, msg.err),
			}
			return m, nil
		}
		m.status = fmt.Sprintf("Node %s set %s", msg.hostPort, msg.status)
		m.fstsErr = nil
		m.fileSystemsErr = nil
		return m, tea.Batch(m.startFSTLoad(), m.startFileSystemsLoad())
	case fileSystemsLoadedMsg:
		if msg.generation != 0 && msg.generation != m.fileSystemsGeneration {
			return m, nil
		}
		selectedFS, keepSelectedFS := m.selectedFileSystem()
		m.fileSystemsLoading = false
		m.fileSystemsErr = msg.err
		if msg.err != nil {
			m.setStatusForViews(fmt.Sprintf("Filesystem refresh failed: %v", msg.err), viewFileSystems, viewNamespaceStats)
		} else {
			m.fileSystems = msg.fs
			m.fsSelected = restoreSelection(m.visibleFileSystems(), m.fsSelected, keepSelectedFS, func(fs eos.FileSystemRecord) bool {
				return fs.ID == selectedFS.ID
			})
			m.markRefreshed("Filesystems updated", viewFileSystems, viewNamespaceStats)
		}
		m.nodeStats.State = m.computeClusterHealth()
	case spacesLoadedMsg:
		if msg.generation != 0 && msg.generation != m.spacesGeneration {
			return m, nil
		}
		selectedSpace, keepSelectedSpace := m.selectedSpace()
		m.spacesLoading = false
		m.spacesErr = msg.err
		if msg.err != nil {
			m.setStatusForViews(fmt.Sprintf("Spaces refresh failed: %v", msg.err), viewSpaces)
		} else {
			m.spaces = msg.spaces
			m.spacesSelected = restoreSelection(m.visibleSpaces(), m.spacesSelected, keepSelectedSpace, func(space eos.SpaceRecord) bool {
				return space.Name == selectedSpace.Name
			})
			m.markRefreshed("Spaces updated", viewSpaces)
		}
	case groupsLoadedMsg:
		if msg.generation != 0 && msg.generation != m.groupsGeneration {
			return m, nil
		}
		selectedGroup, keepSelectedGroup := m.selectedGroup()
		m.groupsLoading = false
		m.groupsErr = msg.err
		if msg.err != nil {
			m.setStatusForViews(fmt.Sprintf("Groups refresh failed: %v", msg.err), viewGroups)
		} else {
			m.groups = msg.groups
			m.groupsSelected = restoreSelection(m.visibleGroups(), m.groupsSelected, keepSelectedGroup, func(group eos.GroupRecord) bool {
				return group.Name == selectedGroup.Name
			})
			m.markRefreshed("Groups updated", viewGroups)
		}
	case vidLoadedMsg:
		if msg.mode != m.vidMode || msg.generation != m.vidGeneration {
			return m, nil
		}
		selectedVID, keepSelectedVID := m.selectedVIDRecord()
		m.vidLoading = false
		m.vidErr = msg.err
		if msg.err != nil {
			m.setStatusForViews(fmt.Sprintf("VID refresh failed: %v", msg.err), viewVID)
		} else {
			m.vidRecords = msg.records
			m.vidSelected = restoreSelection(m.vidRecords, m.vidSelected, keepSelectedVID, func(record eos.VIDRecord) bool {
				return record.Key == selectedVID.Key && record.Value == selectedVID.Value
			})
			m.markRefreshed(fmt.Sprintf("Loaded VID mappings via eos vid ls %s", strings.TrimSpace(msg.mode.flag())), viewVID)
		}
	case accessLoadedMsg:
		if msg.generation != 0 && msg.generation != m.accessGeneration {
			return m, nil
		}
		selectedAccess, keepSelectedAccess := m.selectedAccessRecord()
		m.accessLoading = false
		m.accessErr = msg.err
		if msg.err != nil {
			m.setStatusForViews(fmt.Sprintf("Access refresh failed: %v", msg.err), viewAccess)
		} else {
			m.accessRecords = msg.records
			m.accessSelected = restoreSelection(m.visibleAccessRecords(), m.accessSelected, keepSelectedAccess, func(record eos.AccessRecord) bool {
				return record.RawKey == selectedAccess.RawKey && record.Value == selectedAccess.Value
			})
			m.markRefreshed("Loaded access rules via eos access ls -m", viewAccess)
		}
	case accessActionResultMsg:
		if msg.err != nil {
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("access action failed: %v", msg.err),
			}
			return m, nil
		}
		m.status = fmt.Sprintf("Applied access action: %s", msg.target)
		m.accessErr = nil
		return m, m.startAccessLoad()
	case namespaceStatsLoadedMsg:
		if msg.generation != 0 && msg.generation != m.namespaceStatsGeneration {
			return m, nil
		}
		m.nsStatsLoading = false
		m.nsStatsErr = msg.err
		if msg.err != nil {
			m.setStatusForViews(fmt.Sprintf("Namespace stats refresh failed: %v", msg.err), viewNamespaceStats)
		} else {
			m.namespaceStats = msg.stats
			m.markRefreshed("Namespace statistics updated", viewNamespaceStats)
		}
	case inspectorLoadedMsg:
		if msg.generation != 0 && msg.generation != m.inspectorGeneration {
			return m, nil
		}
		m.inspectorLoading = false
		m.inspectorErr = msg.err
		m.inspectorUpdated = time.Now()
		if msg.err != nil {
			if inspectorErrorSummary(msg.err) == "disabled" {
				m.setStatusForViews("Inspector is disabled; core statistics remain available", viewNamespaceStats)
			} else {
				m.setStatusForViews(fmt.Sprintf("Inspector refresh failed: %v", msg.err), viewNamespaceStats)
			}
		} else {
			m.inspectorStats = msg.stats
			m.markRefreshed("Inspector statistics updated", viewNamespaceStats)
		}
	case directoryLoadedMsg:
		if msg.requestID != 0 && (msg.requestID != m.nsRequestID || cleanPath(msg.path) != m.nsRequestedPath) {
			return m, nil
		}
		selectedEntry, keepSelectedEntry := m.selectedNamespaceEntry()
		m.nsLoading = false
		m.nsErr = msg.err
		if msg.err != nil {
			m.setStatusForViews(fmt.Sprintf("Namespace refresh failed: %v", msg.err), viewNamespace)
		} else {
			m.nsLoaded = true
			m.directory = msg.directory
			m.nsRequestedPath = cleanPath(msg.directory.Path)
			m.nsSelected = restoreSelection(m.visibleNamespaceEntries(), m.nsSelected, keepSelectedEntry, func(entry eos.Entry) bool {
				return entry.Path == selectedEntry.Path
			})
			m = m.rememberNamespaceDetailContent()
			m.markRefreshed(fmt.Sprintf("Browsing namespace %s", m.directory.Path), viewNamespace)
			m.persistUIState()
			return m.startNamespaceAttrLoad(true)
		}
	case namespaceAttrsLoadedMsg:
		if msg.path != m.nsAttrsTargetPath || (msg.requestID != 0 && msg.requestID != m.nsAttrsRequestID) {
			return m, nil
		}
		m.nsAttrsLoading = false
		m.nsAttrsLoaded = true
		m.nsAttrsErr = msg.err
		if msg.err == nil {
			m.nsAttrs = msg.attrs
		}
		m = m.rememberNamespaceDetailContent()
	case namespaceAttrSetResultMsg:
		if msg.err != nil {
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("attr set failed: %v", msg.err),
			}
			return m, nil
		}
		if msg.recursive {
			m.status = fmt.Sprintf("Updated attributes recursively on %s", msg.path)
		} else {
			m.status = fmt.Sprintf("Updated attributes on %s", msg.path)
		}
		return m.startNamespaceAttrLoad(true)
	case namespaceMkdirResultMsg:
		if msg.err != nil {
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("mkdir failed: %v", msg.err),
			}
			return m, nil
		}
		m.nsFilter.filters = map[int]string{}
		m.nsSelected = 0
		m.nsLoading = true
		m.status = fmt.Sprintf("Created directory %s", msg.path)
		return m.requestDirectory(m.directory.Path)
	case spaceStatusLoadedMsg:
		if msg.space != m.spaceStatusTarget || (msg.requestID != 0 && msg.requestID != m.spaceStatusRequestID) {
			return m, nil
		}
		selectedStatus, keepSelectedStatus := m.selectedSpaceStatusRecord()
		m.spaceStatusLoading = false
		m.spaceStatusErr = msg.err
		if msg.err != nil {
			m.setStatusForViews(fmt.Sprintf("Space %s status refresh failed: %v", msg.space, msg.err), viewSpaces, viewSpaceStatus)
		} else {
			m.spaceStatus = msg.records
			m.spaceStatusSelected = restoreSelection(m.spaceStatus, m.spaceStatusSelected, keepSelectedStatus, func(record eos.SpaceStatusRecord) bool {
				return record.Key == selectedStatus.Key
			})
			m.markRefreshed(fmt.Sprintf("Loaded space status for %s", msg.space), viewSpaces)
		}
	case spaceConfigResultMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Space config failed: %v", msg.err)
		} else {
			m.status = fmt.Sprintf("Space %s configuration updated successfully", msg.space)
			if msg.space != m.spaceStatusTarget {
				// The operator has moved to another space while this write was
				// running. Keep the success notification, but never switch the
				// underlying editor/list back to the old context.
				return m, nil
			}
			return m.requestSpaceStatus(msg.space)
		}
	case groupSetResultMsg:
		if msg.batch {
			if len(msg.failed) > 0 {
				m.alert = errorAlert{
					active:  true,
					message: fmt.Sprintf("group set partially failed (%d/%d failed)\n\n%s", len(msg.failed), msg.count, strings.Join(msg.failed, "\n")),
				}
				return m, m.startGroupsLoad()
			}
			m.status = fmt.Sprintf("Set %d groups to %s", msg.count, msg.status)
			return m, m.startGroupsLoad()
		}
		if msg.err != nil {
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("group set failed: %v", msg.err),
			}
			return m, nil
		}
		m.status = fmt.Sprintf("Group %s set to %s", msg.group, msg.status)
		return m, m.startGroupsLoad()
	case fsConfigStatusResultMsg:
		if msg.err != nil {
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("fs config failed: %v", msg.err),
			}
		} else {
			m.status = fmt.Sprintf("Filesystem %d configstatus updated to %s", msg.fsID, msg.value)
			return m, m.startFileSystemsLoad()
		}
	case fsConfigStatusBatchResultMsg:
		if len(msg.failed) > 0 {
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("filesystem configstatus partially failed (%d/%d failed)\n\n%s", len(msg.failed), msg.attempted, strings.Join(msg.failed, "\n")),
			}
			return m, m.startFileSystemsLoad()
		}
		m.status = fmt.Sprintf("Updated configstatus=%s on %d filesystems", msg.value, msg.attempted)
		return m, m.startFileSystemsLoad()
	case apollonDrainResultMsg:
		if msg.err != nil {
			detail := fmt.Sprintf("Apollon drain failed for filesystem %d on %s: %v", msg.fsID, msg.instance, msg.err)
			if msg.output != "" {
				detail += "\n\n" + msg.output
			}
			m.alert = errorAlert{
				active:  true,
				message: detail,
			}
			return m, nil
		}
		m.status = fmt.Sprintf("Apollon drain started for filesystem %d on %s", msg.fsID, msg.instance)
		return m, m.startFileSystemsLoad()
	case qdbCoupResultMsg:
		m.qdbCoupDone = qdbCoupResultPopup{
			active: true,
			host:   msg.host,
			output: msg.output,
			err:    msg.err,
		}
		if msg.err != nil {
			m.status = fmt.Sprintf("QDB coup failed on %s: %v", msg.host, msg.err)
			return m, nil
		}
		m.status = fmt.Sprintf("QDB raft coup attempted on %s", msg.host)
		if msg.output != "" {
			m.status = fmt.Sprintf("QDB raft coup attempted on %s: %s", msg.host, msg.output)
		}
		m.mgmsLoading = true
		m.mgmVersionsLoading = true
		mgmsGeneration := bumpGeneration(&m.mgmsGeneration)
		versionsGeneration := bumpGeneration(&m.mgmVersionsGeneration)
		return m, tea.Batch(
			delayedLoadMGMsCmd(m.client, qdbCoupRefreshDelay, mgmsGeneration),
			delayedReloadMGMVersionsCmd(m.client, qdbCoupRefreshDelay, versionsGeneration),
		)
	case ioShapingLoadedMsg:
		if msg.mode != m.ioShapingMode || (msg.generation != 0 && msg.generation != m.ioShapingGeneration) {
			return m, nil
		}
		selectedRow, keepSelectedRow := m.selectedIOShapingRow()
		m.ioShapingLoading = false
		if msg.err != nil {
			m.ioShapingErr = msg.err
		} else {
			m.ioShaping = msg.records
			m.ioShapingErr = nil
			m.ioShapingSelected = restoreSelection(m.ioShapingMergedRows(), m.ioShapingSelected, keepSelectedRow, func(row ioShapingMergedRow) bool {
				return row.id == selectedRow.id
			})
			m.markRefreshed("IO traffic updated", viewIOShaping)
		}
	case ioShapingPressureLoadedMsg:
		if msg.mode != m.ioShapingMode || (msg.generation != 0 && msg.generation != m.ioShapingGeneration) {
			return m, nil
		}
		pressureRows := m.sortedIOShapingPressure()
		var selectedPressure eos.IOShapingPressureRecord
		keepSelectedPressure := m.ioShapingSelected >= 0 && m.ioShapingSelected < len(pressureRows)
		if keepSelectedPressure {
			selectedPressure = pressureRows[m.ioShapingSelected]
		}
		m.ioShapingLoading = false
		if msg.err != nil {
			m.ioShapingErr = msg.err
		} else {
			m.ioShapingPressure = msg.records
			m.ioShapingErr = nil
			m.ioShapingSelected = restoreSelection(m.sortedIOShapingPressure(), m.ioShapingSelected, keepSelectedPressure, func(record eos.IOShapingPressureRecord) bool {
				return record.App == selectedPressure.App && record.NodeID == selectedPressure.NodeID
			})
			m.markRefreshed("IO pressure updated", viewIOShaping)
		}
	case ioShapingPoliciesLoadedMsg:
		if msg.generation != 0 && msg.generation != m.ioShapingGeneration {
			return m, nil
		}
		selectedRow, keepSelectedRow := m.selectedIOShapingRow()
		m.ioShapingPoliciesLoading = false
		m.ioShapingPoliciesErr = msg.err
		if msg.err == nil {
			m.ioShapingPolicies = msg.records
			m.ioShapingSelected = restoreSelection(m.ioShapingMergedRows(), m.ioShapingSelected, keepSelectedRow, func(row ioShapingMergedRow) bool {
				return row.id == selectedRow.id
			})
		}
	case ioShapingConfigLoadedMsg:
		if msg.generation != 0 && msg.generation != m.ioShapingGeneration {
			return m, nil
		}
		m.ioShapingConfigLoading = false
		m.ioShapingConfigErr = msg.err
		if msg.err == nil {
			m.ioShapingConfig = msg.config
			m.ioShapingConfigLoaded = true
		}
	case ioShapingPolicyResultMsg:
		if msg.err != nil {
			m.ioShapingLoading = false
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("io shaping policy %s failed: %v", msg.op, msg.err),
			}
			return m, nil
		}
		if msg.op == "deleted" {
			m.status = fmt.Sprintf("Deleted IO shaping policy for %s", msg.id)
		} else {
			m.status = fmt.Sprintf("Updated IO shaping policy for %s", msg.id)
		}
		m.ioShapingLoading = true
		m.ioShapingGeneration++
		m.markIOShapingPolicyLoading()
		return m, tea.Batch(
			loadIOShapingViewCmd(m.client, m.ioShapingMode, m.ioShapingGeneration),
			loadIOShapingPolicyDataCmd(m.client, m.ioShapingMode, m.ioShapingGeneration),
		)
	case ioShapingLimitsToggleResultMsg:
		if msg.err != nil {
			m.ioShapingConfigLoading = true
			m.ioShapingGeneration++
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("io shaping controller limits toggle failed: %v", msg.err),
			}
			return m, loadIOShapingConfigCmd(m.client, m.ioShapingGeneration)
		}
		m.ioShapingConfig.LimitsEnabled = msg.enabled
		m.ioShapingConfigLoaded = true
		if msg.enabled {
			m.status = "Enabled IO shaping controller limits"
		} else {
			m.status = "Disabled IO shaping controller limits"
		}
		m.ioShapingLoading = true
		m.ioShapingGeneration++
		m.markIOShapingPolicyLoading()
		return m, tea.Batch(
			loadIOShapingViewCmd(m.client, m.ioShapingMode, m.ioShapingGeneration),
			loadIOShapingPolicyDataCmd(m.client, m.ioShapingMode, m.ioShapingGeneration),
		)
	case eosCheckResultMsg:
		if msg.err != nil {
			hint := "Make sure EOS is installed and available in PATH."
			if m.client != nil && m.client.OriginalSSHTarget() != "" {
				hint = fmt.Sprintf(
					"Could not reach EOS via SSH target %q.\nCheck that the host is reachable and EOS is running.",
					m.client.OriginalSSHTarget(),
				)
			} else {
				hint += "\nUse --ssh <target> to connect to a remote EOS cluster."
			}
			m.alert = errorAlert{
				active:  true,
				fatal:   true,
				message: fmt.Sprintf("EOS is not available: %v\n\n%s", msg.err, hint),
			}
		}
	case logLoadedMsg:
		if !m.log.active || (msg.generation != 0 && msg.generation != m.logGeneration) {
			return m, nil
		}
		if msg.filePath != "" && msg.filePath != m.logSourceLabel() {
			return m, nil
		}
		m.log.loading = false
		m.log.inFlight = false
		m.log.err = msg.err
		m.log.notice = msg.notice
		if msg.err == nil {
			wasAtBottom := m.log.vp.AtBottom()
			prevOffset := m.log.vp.YOffset
			m.log.allLines = msg.lines
			m.log.filtered = applyLogFilter(msg.lines, m.log.filter)
			m.refreshLogViewportContent(false)
			if wasAtBottom {
				m.log.vp.GotoBottom()
			} else {
				maxOffset := max(0, m.log.vp.TotalLineCount()-m.log.vp.Height)
				m.log.vp.SetYOffset(min(prevOffset, maxOffset))
			}
		}
	case logTickMsg:
		if msg.generation != m.logGeneration {
			return m, nil
		}
		if m.log.active && m.log.tailing {
			if m.log.inFlight {
				return m, logTickCmd(m.logGeneration)
			}
			m.log.inFlight = true
			return m, tea.Batch(
				loadLogCmd(m.client, m.currentLogTarget(), m.logGeneration),
				logTickCmd(m.logGeneration),
			)
		}
	case commandHistoryLoadedMsg:
		if msg.generation != 0 && msg.generation != m.commandLogGeneration {
			return m, nil
		}
		m.commandLog.loading = false
		m.commandLog.inFlight = false
		m.commandLog.filePath = msg.filePath
		m.commandLog.err = msg.err
		if msg.err == nil {
			m.commandLog.lines = msg.lines
		}
	case commandLogTickMsg:
		if msg.generation != 0 && msg.generation != m.commandLogGeneration {
			return m, nil
		}
		if m.commandLog.active {
			if m.commandLog.inFlight {
				return m, commandLogTickCmd(m.commandLogGeneration)
			}
			m.commandLog.inFlight = true
			return m, tea.Batch(
				loadCommandHistoryCmd(m.client, m.commandLogGeneration),
				commandLogTickCmd(m.commandLogGeneration),
			)
		}
	case shellExitedMsg:
		m.lastActivity = time.Now()
		if msg.err != nil {
			m.status = fmt.Sprintf("shell exited: %v", msg.err)
		} else {
			m.status = "Shell closed"
		}
		return m, tea.ClearScreen
	case splashTickMsg:
		if m.splash.active {
			if !m.startupLoading() {
				m.splash.active = false
				return m, nil
			}
			m.splash.frame++
			return m, splashTickCmd()
		}
	case tickMsg:
		now := time.Time(msg)
		next := tickCmd(m.refreshInterval)
		if !m.autoRefresh {
			return m, next
		}
		m, refresh := m.autoRefreshActiveView(now)
		if refresh == nil {
			return m, next
		}
		return m, tea.Batch(next, refresh)
	case idleTickMsg:
		now := time.Time(msg)
		if m.idleTimeout > 0 && !m.lastActivity.IsZero() && !now.Before(m.lastActivity.Add(m.idleTimeout)) {
			m.status = fmt.Sprintf("Idle for %s; exiting", m.idleTimeout)
			return m, tea.Quit
		}
		return m, idleTickCmd(m.idleTimeout)
	}

	return m, nil
}

func (m model) View() string {
	if m.shouldShowStartupSplash() {
		splash := m.normalizeRenderedBlock(m.renderStartupSplash(m.height), m.height)
		return m.styles.app.Render(splash)
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	middleHeight := max(0, m.height-lipgloss.Height(header)-lipgloss.Height(footer))
	availableHeight := max(4, middleHeight-2)

	if m.log.active {
		body := m.renderLogOverlay(middleHeight)
		if m.log.plain {
			body = m.normalizeRenderedBlock(body, middleHeight)
		}
		if m.helpActive {
			body = m.renderOverlay(body, m.renderHelpOverlay(), middleHeight)
		}
		return m.styles.app.Render(header + "\n" + body + "\n" + footer)
	}
	if m.helpActive {
		body := m.renderBody(availableHeight)
		help := m.renderHelpOverlay()
		if m.overlayNeedsResize(help) {
			help = m.renderCompactHelpOverlay()
		}
		body = m.renderOverlay(body, help, middleHeight)
		body = m.normalizeRenderedBlock(body, middleHeight)
		return m.styles.app.Render(header + "\n" + body + "\n" + footer)
	}

	bodyHeight, commandHeight := m.splitMainAndCommandHeights(availableHeight)
	// Blocking overlays need the full middle area. Keeping the command history
	// visible underneath a modal steals enough rows to clip choices and the
	// confirmation guidance on common 60x20 terminals.
	if m.blockingOverlayActive() {
		bodyHeight = availableHeight
		commandHeight = 0
	}
	bodyTotalHeight := middleHeight
	if commandHeight > 0 {
		bodyTotalHeight = middleHeight - commandHeight
	}

	body := m.renderBody(bodyHeight)
	if popup, ok := m.activeBlockingOverlay(); ok {
		if m.overlayNeedsResize(popup) {
			popup = m.renderResizeRequiredPopup()
		}
		body = m.renderOverlay(body, popup, bodyTotalHeight)
	}

	body = m.normalizeRenderedBlock(body, bodyTotalHeight)
	middle := body
	if commandHeight > 0 {
		commandPanel := m.normalizeRenderedBlock(m.renderCommandPanel(commandHeight), commandHeight)
		middle = body + "\n" + commandPanel
	}
	return m.styles.app.Render(header + "\n" + middle + "\n" + footer)
}

func (m model) blockingOverlayActive() bool {
	return m.popup.active ||
		m.accessAction.active ||
		m.nodeStatus.active ||
		m.edit.active ||
		m.nsAttrEdit.active ||
		m.nsGoTo.active ||
		m.nsMkdir.active ||
		m.ioShapingEdit.active ||
		m.groupDrain.active ||
		m.apollon.active ||
		m.qdbCoup.active ||
		m.qdbCoupDone.active ||
		m.fsEdit.active ||
		m.alert.active
}

func (m model) activeBlockingOverlay() (string, bool) {
	switch {
	case m.popup.active:
		return m.renderFilterPopup(), true
	case m.accessAction.active:
		return m.renderAccessActionPopup(), true
	case m.nodeStatus.active:
		return m.renderNodeStatusConfirmPopup(), true
	case m.edit.active:
		if m.edit.stage == editStageInput {
			return m.renderSpaceStatusEditPopup(), true
		}
		if m.edit.stage == editStageConfirm {
			return m.renderSpaceStatusConfirmPopup(), true
		}
		return "", false
	case m.nsAttrEdit.active:
		return m.renderNamespaceAttrEditPopup(), true
	case m.nsGoTo.active:
		return m.renderNamespaceGoToPopup(), true
	case m.nsMkdir.active:
		return m.renderNamespaceMkdirPopup(), true
	case m.ioShapingEdit.active:
		return m.renderIOShapingPolicyEditPopup(), true
	case m.groupDrain.active:
		return m.renderGroupDrainConfirmPopup(), true
	case m.apollon.active:
		return m.renderApollonDrainConfirmPopup(), true
	case m.qdbCoup.active:
		return m.renderQDBCoupConfirmPopup(), true
	case m.qdbCoupDone.active:
		return m.renderQDBCoupResultPopup(), true
	case m.fsEdit.active:
		return m.renderFSConfigStatusEditPopup(), true
	case m.alert.active:
		return m.renderErrorAlert(), true
	default:
		return "", false
	}
}

func (m model) overlayNeedsResize(popup string) bool {
	middleHeight := max(0, m.height-lipgloss.Height(m.renderHeader())-lipgloss.Height(m.renderFooter()))
	return lipgloss.Width(popup) > m.contentWidth() || lipgloss.Height(popup) > middleHeight
}

func (m model) blockingOverlayNeedsResize() bool {
	popup, ok := m.activeBlockingOverlay()
	return ok && m.overlayNeedsResize(popup)
}

func (m model) renderResizeRequiredPopup() string {
	return m.renderModal([]string{
		m.styles.popupTitle.Render("Resize"),
		"",
		"Small",
		"",
		m.styles.status.Render("Esc"),
	}, lipgloss.Color("203"), 0)
}

func (m model) startupLoading() bool {
	switch m.activeView {
	case viewMGM, viewQDB:
		return len(m.mgms) == 0 && m.mgmsLoading
	case viewFST:
		return len(m.fsts) == 0 && m.fstsLoading
	case viewFileSystems:
		return len(m.fileSystems) == 0 && m.fileSystemsLoading
	case viewNamespace:
		return !m.nsLoaded && m.nsLoading
	case viewSpaces:
		if m.spaceStatusActive {
			return m.spaceStatusLoading && len(m.spaceStatus) == 0
		}
		return len(m.spaces) == 0 && m.spacesLoading
	case viewNamespaceStats:
		return m.namespaceStats == (eos.NamespaceStats{}) && m.nsStatsLoading
	case viewSpaceStatus:
		return len(m.spaceStatus) == 0 && m.spaceStatusLoading
	case viewIOShaping:
		return len(m.ioShapingMergedRows()) == 0 && m.ioShapingLoading
	case viewGroups:
		return len(m.groups) == 0 && m.groupsLoading
	case viewVID:
		return len(m.vidRecords) == 0 && m.vidLoading
	case viewAccess:
		return len(m.accessRecords) == 0 && m.accessLoading
	default:
		return false
	}
}

func (m model) shouldShowStartupSplash() bool {
	return m.splash.active && m.startupLoading()
}

func (m model) onViewChanged() (tea.Model, tea.Cmd) {
	// Invalidate timer chains and in-flight IO responses whenever the selected
	// view changes. Re-entering the same view deliberately starts one fresh
	// polling owner rather than adding another recurring timer.
	m.ioShapingGeneration++
	m.ioShapingPoliciesLoading = false
	m.ioShapingConfigLoading = false
	m.persistUIState()
	m.status = fmt.Sprintf("Viewing %s", m.activeViewLabel())
	switch m.activeView {
	case viewNamespace:
		return m.maybeLoadNamespace()
	case viewSpaces:
		if !m.spacesLoading && len(m.spaces) == 0 && m.spacesErr == nil {
			m.spacesErr = nil
			return m, m.startSpacesLoad()
		}
		if m.spaceStatusActive {
			return m.maybeLoadSpaceStatus(m.spaceStatusTarget)
		}
		return m, nil
	case viewGroups:
		if !m.groupsLoading && len(m.groups) == 0 && m.groupsErr == nil {
			m.groupsErr = nil
			return m, m.startGroupsLoad()
		}
		return m, nil
	case viewVID:
		if !m.vidLoading && len(m.vidRecords) == 0 && m.vidErr == nil {
			m.vidLoading = true
			m.vidErr = nil
			return m, loadVIDCmd(m.client, m.vidMode, m.vidGeneration)
		}
		return m, nil
	case viewAccess:
		if !m.accessLoading && len(m.accessRecords) == 0 && m.accessErr == nil {
			m.accessErr = nil
			return m, m.startAccessLoad()
		}
		return m, nil
	case viewNamespaceStats:
		cmds := make([]tea.Cmd, 0, 2)
		if !m.nsStatsLoading && m.namespaceStats == (eos.NamespaceStats{}) && m.nsStatsErr == nil {
			m.nsStatsErr = nil
			cmds = append(cmds, m.startNamespaceStatsLoad())
		}
		if !m.inspectorLoading && !hasInspectorStatsData(m.inspectorStats) && m.inspectorErr == nil {
			m.inspectorErr = nil
			cmds = append(cmds, m.startInspectorLoad())
		}
		if !m.fstStatsLoading && m.nodeStats == (eos.NodeStats{}) && m.nodeStatsErr == nil {
			m.nodeStatsErr = nil
			cmds = append(cmds, m.startNodeStatsLoad())
		}
		if len(cmds) == 0 {
			return m, nil
		}
		return m, tea.Batch(cmds...)
	case viewSpaceStatus:
		return m.maybeLoadSpaceStatus(m.currentSpaceStatusName())
	case viewIOShaping:
		m.ioShapingLoading = true
		m.ioShapingErr = nil
		m.markIOShapingPolicyLoading()
		return m, tea.Batch(
			loadIOShapingViewCmd(m.client, m.ioShapingMode, m.ioShapingGeneration),
			loadIOShapingPolicyDataCmd(m.client, m.ioShapingMode, m.ioShapingGeneration),
		)
	default:
		return m, nil
	}
}

func (m model) refreshActiveView() (tea.Model, tea.Cmd) {
	switch m.activeView {
	case viewNamespace:
		if m.nsLoading {
			m.status = "Namespace refresh already in progress"
			return m, nil
		}
		m.status = fmt.Sprintf("Refreshing namespace %s...", m.directory.Path)
		return m.requestDirectory(m.directory.Path)
	case viewSpaces:
		if m.spaceStatusActive {
			if m.spaceStatusLoading {
				m.status = "Space status refresh already in progress"
				return m, nil
			}
			m.spaceStatusLoading = true
			m.spaceStatusErr = nil
			m.status = fmt.Sprintf("Refreshing space status for %s...", m.spaceStatusTarget)
			return m.requestSpaceStatus(m.spaceStatusTarget)
		}
		if m.spacesLoading {
			m.status = "Spaces refresh already in progress"
			return m, nil
		}
		m.spacesErr = nil
		m.status = "Refreshing spaces..."
		return m, m.startSpacesLoad()
	case viewGroups:
		if m.groupsLoading {
			m.status = "Groups refresh already in progress"
			return m, nil
		}
		m.groupsErr = nil
		m.status = "Refreshing groups..."
		return m, m.startGroupsLoad()
	case viewNamespaceStats:
		if m.nsStatsLoading || m.fstStatsLoading || m.fstsLoading || m.fileSystemsLoading {
			m.status = "General stats refresh already in progress"
			return m, nil
		}
		m.nsStatsErr = nil
		m.nodeStatsErr = nil
		m.status = "Refreshing general stats..."
		cmds := []tea.Cmd{
			m.startNamespaceStatsLoad(),
			m.startNodeStatsLoad(),
			m.startFSTLoad(),
			m.startFileSystemsLoad(),
		}
		if !m.inspectorLoading {
			m.inspectorErr = nil
			cmds = append(cmds, m.startInspectorLoad())
		}
		return m, tea.Batch(cmds...)
	case viewSpaceStatus:
		m.spaceStatusLoading = true
		m.spaceStatusErr = nil
		m.status = fmt.Sprintf("Refreshing space status for %s...", m.currentSpaceStatusName())
		return m.requestSpaceStatus(m.currentSpaceStatusName())
	case viewIOShaping:
		if m.ioShapingLoading || m.ioShapingPoliciesLoading || m.ioShapingConfigLoading {
			m.status = "IO shaping refresh already in progress"
			return m, nil
		}
		m.ioShapingLoading = true
		m.ioShapingErr = nil
		m.markIOShapingPolicyLoading()
		m.status = "Refreshing IO shaping..."
		return m, tea.Batch(
			loadIOShapingViewCmd(m.client, m.ioShapingMode, m.ioShapingGeneration),
			loadIOShapingPolicyDataCmd(m.client, m.ioShapingMode, m.ioShapingGeneration),
		)
	case viewVID:
		if m.vidLoading {
			m.status = "VID refresh already in progress"
			return m, nil
		}
		m.vidLoading = true
		m.vidErr = nil
		m.status = fmt.Sprintf("Refreshing VID scope %s...", m.vidMode.label())
		return m, loadVIDCmd(m.client, m.vidMode, m.vidGeneration)
	case viewAccess:
		if m.accessLoading {
			m.status = "Access refresh already in progress"
			return m, nil
		}
		m.accessErr = nil
		m.status = "Refreshing access rules..."
		return m, m.startAccessLoad()
	case viewMGM, viewQDB:
		if m.mgmsLoading || m.mgmVersionsLoading {
			m.status = "MGM/QDB refresh already in progress..."
			return m, nil
		}
		m.mgmsErr = nil
		m.mgmVersionsErr = nil
		m.status = "Refreshing MGM/QDB topology and versions..."
		return m, tea.Batch(m.startMGMLoad(), m.startMGMVersionsReload())
	case viewFST:
		if m.fstsLoading {
			m.status = "Node refresh already in progress"
			return m, nil
		}
		m.fstsErr = nil
		m.status = "Refreshing FST nodes..."
		return m, m.startFSTLoad()
	case viewFileSystems:
		if m.fileSystemsLoading {
			m.status = "Filesystem refresh already in progress"
			return m, nil
		}
		m.fileSystemsErr = nil
		m.status = "Refreshing filesystems..."
		return m, m.startFileSystemsLoad()
	default:
		m.nodeStatsErr = nil
		m.fstsErr = nil
		m.mgmsErr = nil
		m.fileSystemsErr = nil
		m.spacesErr = nil
		m.nsStatsErr = nil
		m.status = "Refreshing..."
		return m, tea.Batch(
			m.startNodeStatsLoad(),
			m.startFSTLoad(),
			m.startMGMLoad(),
			m.startFileSystemsLoad(),
			m.startSpacesLoad(),
			m.startNamespaceStatsLoad(),
			m.startInspectorLoad(),
		)
	}
}

func (m *model) markIOShapingPolicyLoading() {
	m.ioShapingPoliciesErr = nil
	m.ioShapingConfigErr = nil
	m.ioShapingConfigLoading = true
	m.ioShapingPoliciesLoading = ioShapingModeHasPolicies(m.ioShapingMode)
}

// autoRefreshActiveView refreshes only the data the operator is currently
// looking at. Loading flags provide single-flight protection, so a slow EOS or
// SSH request cannot cause overlapping refresh waves.
func (m model) autoRefreshActiveView(now time.Time) (model, tea.Cmd) {
	switch m.activeView {
	case viewNamespaceStats:
		if m.nsStatsLoading || m.fstStatsLoading || m.fstsLoading || m.fileSystemsLoading {
			return m, nil
		}
		cmds := []tea.Cmd{
			m.startNamespaceStatsLoad(),
			m.startNodeStatsLoad(),
			m.startFSTLoad(),
			m.startFileSystemsLoad(),
		}
		if !m.inspectorLoading && m.inspectorAutoRefreshDue(now) {
			cmds = append(cmds, m.startInspectorLoad())
		}
		return m, tea.Batch(cmds...)
	case viewFST:
		if m.fstsLoading {
			return m, nil
		}
		return m, m.startFSTLoad()
	case viewFileSystems:
		if m.fileSystemsLoading {
			return m, nil
		}
		return m, m.startFileSystemsLoad()
	case viewSpaces:
		if m.spaceStatusActive {
			if m.spaceStatusLoading || m.spaceStatusTarget == "" {
				return m, nil
			}
			m.spaceStatusLoading = true
			return m.requestSpaceStatus(m.spaceStatusTarget)
		}
		if m.spacesLoading {
			return m, nil
		}
		return m, m.startSpacesLoad()
	case viewGroups:
		if m.groupsLoading {
			return m, nil
		}
		return m, m.startGroupsLoad()
	case viewIOShaping:
		if m.ioShapingLoading || m.ioShapingPoliciesLoading || m.ioShapingConfigLoading {
			return m, nil
		}
		m.ioShapingLoading = true
		m.markIOShapingPolicyLoading()
		return m, tea.Batch(
			loadIOShapingViewCmd(m.client, m.ioShapingMode, m.ioShapingGeneration),
			loadIOShapingPolicyDataCmd(m.client, m.ioShapingMode, m.ioShapingGeneration),
		)
	case viewMGM, viewQDB:
		if m.mgmsLoading {
			return m, nil
		}
		cmds := []tea.Cmd{m.startMGMLoad()}
		if !m.mgmVersionsLoading && (m.mgmVersionsUpdated.IsZero() || now.Sub(m.mgmVersionsUpdated) >= mgmVersionRefreshInterval) {
			cmds = append(cmds, m.startMGMVersionsReload())
		}
		return m, tea.Batch(cmds...)
	case viewVID:
		if m.vidLoading {
			return m, nil
		}
		m.vidLoading = true
		return m, loadVIDCmd(m.client, m.vidMode, m.vidGeneration)
	case viewAccess:
		if m.accessLoading {
			return m, nil
		}
		return m, m.startAccessLoad()
	default:
		return m, nil
	}
}

func mergeMGMVersionData(next, current []eos.MgmRecord) []eos.MgmRecord {
	if len(next) == 0 || len(current) == 0 {
		return next
	}
	return applyMGMVersions(next, existingMGMVersions(current), existingQDBVersions(current))
}

func mgmVersionProbeTargets(records []eos.MgmRecord) []eos.MgmRecord {
	targets := make([]eos.MgmRecord, 0, len(records))
	for _, record := range records {
		var target eos.MgmRecord
		if record.Host != "" && record.EOSVersion == "" {
			target.Host = record.Host
		}
		if record.QDBHost != "" && record.QDBVersion == "" {
			target.QDBHost = record.QDBHost
		}
		if target.Host != "" || target.QDBHost != "" {
			targets = append(targets, target)
		}
	}
	return targets
}

func hasMissingMGMVersions(records []eos.MgmRecord) bool {
	return len(mgmVersionProbeTargets(records)) > 0
}

func existingMGMVersions(records []eos.MgmRecord) map[string]string {
	versions := make(map[string]string, len(records))
	for _, record := range records {
		if record.Host != "" && record.EOSVersion != "" {
			versions[record.Host] = record.EOSVersion
		}
	}
	return versions
}

func existingQDBVersions(records []eos.MgmRecord) map[string]string {
	versions := make(map[string]string, len(records))
	for _, record := range records {
		if record.QDBHost != "" && record.QDBVersion != "" {
			versions[record.QDBHost] = record.QDBVersion
		}
	}
	return versions
}

func applyMGMVersions(records []eos.MgmRecord, mgmVersions, qdbVersions map[string]string) []eos.MgmRecord {
	if len(records) == 0 {
		return records
	}
	out := append([]eos.MgmRecord(nil), records...)
	for i := range out {
		if version := mgmVersions[out[i].Host]; version != "" {
			out[i].EOSVersion = version
		}
		if version := qdbVersions[out[i].QDBHost]; version != "" {
			out[i].QDBVersion = version
		}
	}
	return out
}

func hasInspectorStatsData(stats eos.InspectorStats) bool {
	return stats.AvgFileSize > 0 ||
		stats.HardlinkCount > 0 ||
		stats.HardlinkVolume > 0 ||
		stats.SymlinkCount > 0 ||
		stats.LayoutCount > 0 ||
		stats.TopLayout.Layout != "" ||
		stats.TopUserCost.Name != "" ||
		stats.TopGroupCost.Name != "" ||
		len(stats.Layouts) > 0 ||
		len(stats.UserCosts) > 0 ||
		len(stats.GroupCosts) > 0 ||
		len(stats.AccessFiles) > 0 ||
		len(stats.AccessVolume) > 0 ||
		len(stats.BirthFiles) > 0 ||
		len(stats.BirthVolume) > 0
}

func (m model) inspectorAutoRefreshDue(now time.Time) bool {
	if m.inspectorUpdated.IsZero() {
		return true
	}
	interval := inspectorRefreshInterval
	if m.inspectorErr != nil {
		interval = inspectorFailureRetryInterval
	}
	return now.Sub(m.inspectorUpdated) >= interval
}

func (m model) maybeLoadNamespace() (tea.Model, tea.Cmd) {
	if m.nsLoading {
		return m, nil
	}
	if m.nsLoaded {
		return m.startNamespaceAttrLoad(false)
	}

	m.status = fmt.Sprintf("Loading namespace %s...", m.directory.Path)
	return m.requestDirectory(m.directory.Path)
}

func (m model) requestDirectory(path string) (tea.Model, tea.Cmd) {
	path = cleanPath(path)
	m.nsRequestID++
	m.nsRequestedPath = path
	m.nsLoading = true
	m.nsErr = nil
	return m, loadDirectoryCmd(m.client, path, m.nsRequestID)
}

func (m model) namespaceNavigationLoading() bool {
	return m.nsLoading && cleanPath(m.nsRequestedPath) != cleanPath(m.directory.Path)
}

func (m model) currentNamespaceAttrTargetPath() string {
	if selected, ok := m.selectedNamespaceEntry(); ok && selected.Path != "" {
		return selected.Path
	}
	if m.directory.Self.Path != "" {
		return m.directory.Self.Path
	}
	if m.directory.Path != "" {
		return m.directory.Path
	}
	return "/"
}

func (m model) startNamespaceAttrLoad(force bool) (tea.Model, tea.Cmd) {
	path := m.currentNamespaceAttrTargetPath()
	if path == "" || m.client == nil {
		return m, nil
	}
	if !force && m.nsAttrsTargetPath == path && (m.nsAttrsLoading || m.nsAttrsLoaded) {
		return m, nil
	}

	m.nsAttrsTargetPath = path
	m.nsAttrsRequestID++
	m.nsAttrsLoading = true
	m.nsAttrsLoaded = false
	m.nsAttrsErr = nil
	m.nsAttrs = nil
	return m, loadNamespaceAttrsCmd(m.client, path, m.nsAttrsRequestID)
}

func (m model) maybeLoadSpaceStatus(space string) (tea.Model, tea.Cmd) {
	if space == "" || m.client == nil {
		return m, nil
	}
	if !m.spaceStatusLoading && m.spaceStatusErr == nil && m.spaceStatusTarget == space && len(m.spaceStatus) > 0 {
		return m, nil
	}

	if m.spaceStatusTarget != space {
		m.spaceStatus = nil
		m.spaceStatusSelected = 0
	}
	m.spaceStatusTarget = space
	m.status = fmt.Sprintf("Loading space status for %s...", space)
	return m.requestSpaceStatus(space)
}

func (m model) requestSpaceStatus(space string) (model, tea.Cmd) {
	m.spaceStatusRequestID++
	m.spaceStatusTarget = space
	m.spaceStatusLoading = true
	m.spaceStatusErr = nil
	return m, loadSpaceStatusCmd(m.client, space, m.spaceStatusRequestID)
}

func (m *model) markRefreshed(status string, views ...viewID) {
	now := time.Now()
	for _, view := range views {
		if view >= 0 && int(view) < len(m.lastRefreshAt) {
			m.lastRefreshAt[view] = now
		}
	}
	m.setStatusForViews(status, views...)
}

func (m *model) setStatusForViews(status string, views ...viewID) {
	if status == "" {
		return
	}
	for _, view := range views {
		if m.activeView == view {
			m.status = status
			return
		}
	}
}

func (m model) activeViewLastRefresh() time.Time {
	if m.activeView >= 0 && int(m.activeView) < len(m.lastRefreshAt) {
		return m.lastRefreshAt[m.activeView]
	}
	return time.Time{}
}

func (m model) computeClusterHealth() string {
	fsts := m.fsts
	fss := m.fileSystems
	if m.fstsErr != nil || m.fileSystemsErr != nil {
		return "UNKNOWN"
	}
	if (m.fstsLoading && len(fsts) == 0) || (m.fileSystemsLoading && len(fss) == 0) {
		return "CHECKING"
	}
	if len(fsts) == 0 && len(fss) == 0 {
		return "-"
	}
	for _, node := range fsts {
		if strings.ToLower(node.Status) != "online" {
			return "WARN"
		}
	}
	for _, fs := range fss {
		if strings.ToLower(fs.Boot) != "booted" {
			return "WARN"
		}
	}
	return "OK"
}
