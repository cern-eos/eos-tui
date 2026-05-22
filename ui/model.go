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
	IdleTimeout time.Duration
}

func NewModel(client *eos.Client, endpoint, rootPath string) tea.Model {
	return NewModelWithOptions(client, endpoint, rootPath, ModelOptions{})
}

func NewModelWithOptions(client *eos.Client, endpoint, rootPath string, opts ModelOptions) tea.Model {
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

	return model{
		client:             client,
		endpoint:           endpoint,
		idleTimeout:        opts.IdleTimeout,
		lastActivity:       now,
		width:              120,
		height:             32,
		activeView:         activeView,
		fstStatsLoading:    true,
		fstsLoading:        true,
		fileSystemsLoading: true,
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
		status:               "Loading EOS state...",
		fstColumnSelected:    int(fstFilterHost),
		fsColumnSelected:     int(fsFilterHost),
		groupsColumnSelected: int(groupFilterName),
		fstSort:              sortState{column: int(fstSortNone)},
		fsSort:               sortState{column: int(fsSortNone)},
		spaceSort:            sortState{column: int(spaceSortNone)},
		groupSort:            sortState{column: int(groupSortNone)},
		fstFilter:            filterState{filters: map[int]string{}},
		fsFilter:             filterState{filters: map[int]string{}},
		nsFilter:             filterState{filters: map[int]string{}},
		spaceFilter:          filterState{filters: map[int]string{}},
		groupFilter:          filterState{filters: map[int]string{}},
		accessFilter:         filterState{filters: map[int]string{}},
		statsFilter:          filterState{filters: map[int]string{}},
		popup: filterPopup{
			input: input,
			table: popupTable,
		},
		commandLog: commandPanel{
			active:  commandLogVisible,
			loading: commandLogVisible,
		},
		splash: startupSplash{
			active: true,
		},
		styles: newStyles(),
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{checkEOSCmd(m.client), loadInfraCmd(m.client), tickCmd(), splashTickCmd()}
	switch m.activeView {
	case viewNamespace:
		cmds = append(cmds, loadDirectoryCmd(m.client, m.directory.Path))
	case viewGroups:
		cmds = append(cmds, loadGroupsCmd(m.client))
	case viewIOShaping:
		cmds = append(cmds, loadIOShapingViewCmd(m.client, m.ioShapingMode), loadIOShapingPolicyDataCmd(m.client, m.ioShapingMode), ioShapingTickCmd(), ioShapingPolicyTickCmd())
	case viewVID:
		cmds = append(cmds, loadVIDCmd(m.client, m.vidMode))
	case viewAccess:
		cmds = append(cmds, loadAccessCmd(m.client))
	}
	if m.commandLog.active {
		cmds = append(cmds, loadCommandHistoryCmd(m.client), commandLogTickCmd())
	}
	return tea.Batch(cmds...)
}

func (m model) toggleCommandLog() (tea.Model, tea.Cmd) {
	m.commandLog.active = !m.commandLog.active
	m.persistUIState()
	if !m.commandLog.active {
		m.commandLog.loading = false
		return m, nil
	}

	m.commandLog.loading = true
	m.commandLog.err = nil
	return m, tea.Batch(loadCommandHistoryCmd(m.client), commandLogTickCmd())
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
		// Log overlay intercepts all keys when active.
		if m.log.active {
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
			return m.openLogOverlay()
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
		m.mgmsLoading = false
		m.mgms = mergeMGMVersionData(msg.mgms, m.mgms)
		m.mgmsErr = msg.err
		m.mgmSelected = clampIndex(m.mgmSelected, len(m.topologySelectableRows()))
		if msg.err == nil && !m.mgmVersionsLoading {
			probeTargets := mgmVersionProbeTargets(m.mgms)
			if len(probeTargets) > 0 {
				m.mgmVersionsLoaded = false
				m.mgmVersionsLoading = true
				return m, loadMGMVersionsCmd(m.client, probeTargets)
			}
		}
		if msg.err == nil {
			m.mgmVersionsLoaded = !hasMissingMGMVersions(m.mgms)
		}
		return m, nil

	case mgmVersionsLoadedMsg:
		m.mgmVersionsLoading = false
		m.mgmVersionsErr = msg.err
		m.mgms = applyMGMVersions(m.mgms, msg.mgmVersions, msg.qdbVersions)
		m.mgmVersionsLoaded = !hasMissingMGMVersions(m.mgms)
		if msg.err != nil {
			m.status = fmt.Sprintf("Loaded MGM/QDB topology with partial versions: %v", msg.err)
		} else if len(msg.mgmVersions) > 0 || len(msg.qdbVersions) > 0 {
			m.status = "Loaded MGM/QDB versions"
		}
		return m, nil

	case infraLoadedMsg:
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
			m.fstSelected = clampIndex(m.fstSelected, len(m.visibleFSTs()))
		}
		m.mgmsErr = msg.mgmsErr
		if msg.mgmsErr == nil {
			m.mgms = msg.mgms
			m.mgmSelected = clampIndex(m.mgmSelected, len(m.topologySelectableRows()))
		}
		m.fileSystemsErr = msg.fsErr
		if msg.fsErr == nil {
			m.fileSystems = msg.fs
			m.fsSelected = clampIndex(m.fsSelected, len(m.visibleFileSystems()))
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
			m.status = fmt.Sprintf("Infrastructure refresh failed: %v", msg.err)
		} else {
			m.status = fmt.Sprintf("Connected to %s", m.endpoint)
		}
	case eosVersionLoadedMsg:
		if msg.version != "" {
			m.eosVersion = msg.version
		}
	case nodeStatsLoadedMsg:
		m.fstStatsLoading = false
		m.nodeStatsErr = msg.err
		if msg.err != nil {
			m.status = fmt.Sprintf("Cluster summary refresh failed: %v", msg.err)
		} else {
			m.nodeStats = msg.stats
			m.nodeStats.State = m.computeClusterHealth()
			m.status = fmt.Sprintf("Connected to %s", m.endpoint)
		}
	case fstsLoadedMsg:
		m.fstsLoading = false
		m.fstsErr = msg.err
		if msg.err != nil {
			m.status = fmt.Sprintf("Node list refresh failed: %v", msg.err)
		} else {
			m.fsts = msg.fsts
			m.fstSelected = clampIndex(m.fstSelected, len(m.visibleFSTs()))
			m.status = fmt.Sprintf("Connected to %s", m.endpoint)
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
		m.fstsLoading = true
		m.fstsErr = nil
		return m, tea.Batch(loadFSTsCmd(m.client), loadFileSystemsCmd(m.client))
	case fileSystemsLoadedMsg:
		m.fileSystemsLoading = false
		m.fileSystemsErr = msg.err
		if msg.err != nil {
			m.status = fmt.Sprintf("Filesystem refresh failed: %v", msg.err)
		} else {
			m.fileSystems = msg.fs
			m.fsSelected = clampIndex(m.fsSelected, len(m.visibleFileSystems()))
			m.status = fmt.Sprintf("Connected to %s", m.endpoint)
		}
		m.nodeStats.State = m.computeClusterHealth()
	case spacesLoadedMsg:
		m.spacesLoading = false
		m.spacesErr = msg.err
		if msg.err != nil {
			m.status = fmt.Sprintf("Spaces refresh failed: %v", msg.err)
		} else {
			m.spaces = msg.spaces
			m.spacesSelected = clampIndex(m.spacesSelected, len(m.visibleSpaces()))
		}
	case groupsLoadedMsg:
		m.groupsLoading = false
		m.groupsErr = msg.err
		if msg.err != nil {
			m.status = fmt.Sprintf("Groups refresh failed: %v", msg.err)
		} else {
			m.groups = msg.groups
			m.groupsSelected = clampIndex(m.groupsSelected, len(m.visibleGroups()))
			m.status = fmt.Sprintf("Connected to %s", m.endpoint)
		}
	case vidLoadedMsg:
		if msg.mode != m.vidMode {
			return m, nil
		}
		m.vidLoading = false
		m.vidErr = msg.err
		if msg.err != nil {
			m.status = fmt.Sprintf("VID refresh failed: %v", msg.err)
		} else {
			m.vidRecords = msg.records
			m.vidSelected = clampIndex(m.vidSelected, len(m.vidRecords))
			m.status = fmt.Sprintf("Loaded VID mappings via eos vid ls %s", strings.TrimSpace(msg.mode.flag()))
		}
	case accessLoadedMsg:
		m.accessLoading = false
		m.accessErr = msg.err
		if msg.err != nil {
			m.status = fmt.Sprintf("Access refresh failed: %v", msg.err)
		} else {
			m.accessRecords = msg.records
			m.accessSelected = clampIndex(m.accessSelected, len(m.visibleAccessRecords()))
			m.status = "Loaded access rules via eos access ls -m"
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
		m.accessLoading = true
		m.accessErr = nil
		return m, loadAccessCmd(m.client)
	case namespaceStatsLoadedMsg:
		m.nsStatsLoading = false
		m.nsStatsErr = msg.err
		if msg.err != nil {
			m.status = fmt.Sprintf("Namespace stats refresh failed: %v", msg.err)
		} else {
			m.namespaceStats = msg.stats
			m.status = fmt.Sprintf("Connected to %s", m.endpoint)
		}
	case inspectorLoadedMsg:
		m.inspectorLoading = false
		m.inspectorErr = msg.err
		if msg.err != nil {
			m.status = fmt.Sprintf("Inspector refresh failed: %v", msg.err)
		} else {
			m.inspectorStats = msg.stats
			m.status = fmt.Sprintf("Connected to %s", m.endpoint)
		}
	case directoryLoadedMsg:
		m.nsLoading = false
		m.nsErr = msg.err
		if msg.err != nil {
			m.status = fmt.Sprintf("Namespace refresh failed: %v", msg.err)
		} else {
			m.nsLoaded = true
			m.directory = msg.directory
			if m.nsSelected >= len(m.directory.Entries) {
				m.nsSelected = max(0, len(m.directory.Entries)-1)
			}
			m = m.rememberNamespaceDetailContent()
			m.status = fmt.Sprintf("Browsing namespace %s", m.directory.Path)
			m.persistUIState()
			return m.startNamespaceAttrLoad(true)
		}
	case namespaceAttrsLoadedMsg:
		if msg.path != m.nsAttrsTargetPath {
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
		m.nsAttrEdit.active = false
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
		m.nsMkdir.active = false
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
		return m, loadDirectoryCmd(m.client, m.directory.Path)
	case spaceStatusLoadedMsg:
		if msg.space != m.spaceStatusTarget {
			return m, nil
		}
		m.spaceStatusLoading = false
		m.spaceStatusErr = msg.err
		if msg.err != nil {
			m.status = fmt.Sprintf("Space %s status refresh failed: %v", msg.space, msg.err)
		} else {
			m.spaceStatus = msg.records
			m.spaceStatusSelected = clampIndex(m.spaceStatusSelected, len(m.spaceStatus))
			m.status = fmt.Sprintf("Loaded space status for %s", msg.space)
		}
	case spaceConfigResultMsg:
		m.edit.active = false
		if msg.err != nil {
			m.status = m.styles.error.Render(fmt.Sprintf("Space config failed: %v", msg.err))
		} else {
			m.status = fmt.Sprintf("Space %s configuration updated successfully", msg.space)
			return m, loadSpaceStatusCmd(m.client, msg.space)
		}
	case groupSetResultMsg:
		m.groupDrain.active = false
		if msg.batch {
			if len(msg.failed) > 0 {
				m.alert = errorAlert{
					active:  true,
					message: fmt.Sprintf("group set partially failed (%d/%d failed)\n\n%s", len(msg.failed), msg.count, strings.Join(msg.failed, "\n")),
				}
				return m, loadGroupsCmd(m.client)
			}
			m.status = fmt.Sprintf("Set %d groups to %s", msg.count, msg.status)
			return m, loadGroupsCmd(m.client)
		}
		if msg.err != nil {
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("group set failed: %v", msg.err),
			}
			return m, nil
		}
		m.status = fmt.Sprintf("Group %s set to %s", msg.group, msg.status)
		return m, loadGroupsCmd(m.client)
	case fsConfigStatusResultMsg:
		m.fsEdit.active = false
		if msg.err != nil {
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("fs config failed: %v", msg.err),
			}
		} else {
			m.status = fmt.Sprintf("Filesystem %d configstatus updated", m.fsEdit.fsID)
			return m, loadFileSystemsCmd(m.client)
		}
	case fsConfigStatusBatchResultMsg:
		m.fsEdit.active = false
		if len(msg.failed) > 0 {
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("filesystem configstatus partially failed (%d/%d failed)\n\n%s", len(msg.failed), msg.attempted, strings.Join(msg.failed, "\n")),
			}
			return m, loadFileSystemsCmd(m.client)
		}
		m.status = fmt.Sprintf("Updated configstatus=%s on %d filesystems", msg.value, msg.attempted)
		return m, loadFileSystemsCmd(m.client)
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
		return m, loadFileSystemsCmd(m.client)
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
		return m, tea.Batch(
			delayedLoadMGMsCmd(m.client, qdbCoupRefreshDelay),
			delayedReloadMGMVersionsCmd(m.client, qdbCoupRefreshDelay),
		)
	case ioShapingLoadedMsg:
		if msg.mode != m.ioShapingMode {
			return m, nil
		}
		m.ioShapingLoading = false
		if msg.err != nil {
			m.ioShapingErr = msg.err
		} else {
			m.ioShaping = msg.records
			m.ioShapingErr = nil
			m.ioShapingSelected = clampIndex(m.ioShapingSelected, len(m.ioShapingMergedRows()))
		}
	case ioShapingPressureLoadedMsg:
		if msg.mode != m.ioShapingMode {
			return m, nil
		}
		m.ioShapingLoading = false
		if msg.err != nil {
			m.ioShapingErr = msg.err
		} else {
			m.ioShapingPressure = msg.records
			m.ioShapingErr = nil
			m.ioShapingSelected = clampIndex(m.ioShapingSelected, len(m.ioShapingPressure))
		}
	case ioShapingPoliciesLoadedMsg:
		if msg.err == nil {
			m.ioShapingPolicies = msg.records
			m.ioShapingSelected = clampIndex(m.ioShapingSelected, len(m.ioShapingMergedRows()))
		}
	case ioShapingConfigLoadedMsg:
		m.ioShapingConfigErr = msg.err
		if msg.err == nil {
			m.ioShapingConfig = msg.config
			m.ioShapingConfigLoaded = true
		}
	case ioShapingPolicyResultMsg:
		if msg.err != nil {
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
		return m, tea.Batch(loadIOShapingViewCmd(m.client, m.ioShapingMode), loadIOShapingPolicyDataCmd(m.client, m.ioShapingMode))
	case ioShapingLimitsToggleResultMsg:
		if msg.err != nil {
			m.alert = errorAlert{
				active:  true,
				message: fmt.Sprintf("io shaping controller limits toggle failed: %v", msg.err),
			}
			return m, loadIOShapingConfigCmd(m.client)
		}
		m.ioShapingConfig.LimitsEnabled = msg.enabled
		m.ioShapingConfigLoaded = true
		if msg.enabled {
			m.status = "Enabled IO shaping controller limits"
		} else {
			m.status = "Disabled IO shaping controller limits"
		}
		return m, tea.Batch(loadIOShapingViewCmd(m.client, m.ioShapingMode), loadIOShapingPolicyDataCmd(m.client, m.ioShapingMode))
	case ioShapingTickMsg:
		if m.activeView == viewIOShaping && !m.ioShapingLoading {
			m.ioShapingLoading = true
			return m, tea.Batch(loadIOShapingViewCmd(m.client, m.ioShapingMode), ioShapingTickCmd())
		} else if m.activeView == viewIOShaping {
			return m, ioShapingTickCmd()
		}
	case ioShapingPolicyTickMsg:
		if m.activeView == viewIOShaping {
			return m, tea.Batch(loadIOShapingPolicyDataCmd(m.client, m.ioShapingMode), ioShapingPolicyTickCmd())
		}
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
		if m.log.active && msg.filePath != "" && msg.filePath != m.logSourceLabel() {
			return m, nil
		}
		m.log.loading = false
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
		if m.log.active && m.log.tailing {
			return m, tea.Batch(loadLogCmd(m.client, m.currentLogTarget()), logTickCmd())
		}
	case commandHistoryLoadedMsg:
		m.commandLog.loading = false
		m.commandLog.filePath = msg.filePath
		m.commandLog.err = msg.err
		if msg.err == nil {
			m.commandLog.lines = msg.lines
		}
	case commandLogTickMsg:
		if m.commandLog.active {
			return m, tea.Batch(loadCommandHistoryCmd(m.client), commandLogTickCmd())
		}
	case shellExitedMsg:
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
		if m.idleTimeout > 0 && !m.lastActivity.IsZero() && !now.Before(m.lastActivity.Add(m.idleTimeout)) {
			m.status = fmt.Sprintf("Idle for %s; exiting", m.idleTimeout)
			return m, tea.Quit
		}
		return m, tea.Batch(tickCmd(), loadInfraCmd(m.client))
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
		return m.styles.app.Render(header + "\n" + body + "\n" + footer)
	}

	bodyHeight, commandHeight := m.splitMainAndCommandHeights(availableHeight)
	bodyTotalHeight := middleHeight
	if commandHeight > 0 {
		bodyTotalHeight = middleHeight - commandHeight
	}

	body := m.renderBody(bodyHeight)
	if m.popup.active {
		body = m.renderBodyWithPopup(body, bodyTotalHeight)
	} else if m.accessAction.active {
		body = m.renderOverlay(body, m.renderAccessActionPopup(), bodyTotalHeight)
	} else if m.nodeStatus.active {
		body = m.renderOverlay(body, m.renderNodeStatusConfirmPopup(), bodyTotalHeight)
	} else if m.edit.active {
		body = m.renderBodyWithEditPopup(body, bodyTotalHeight)
	} else if m.nsAttrEdit.active {
		body = m.renderOverlay(body, m.renderNamespaceAttrEditPopup(), bodyTotalHeight)
	} else if m.nsGoTo.active {
		body = m.renderOverlay(body, m.renderNamespaceGoToPopup(), bodyTotalHeight)
	} else if m.nsMkdir.active {
		body = m.renderOverlay(body, m.renderNamespaceMkdirPopup(), bodyTotalHeight)
	} else if m.ioShapingEdit.active {
		body = m.renderOverlay(body, m.renderIOShapingPolicyEditPopup(), bodyTotalHeight)
	} else if m.groupDrain.active {
		body = m.renderOverlay(body, m.renderGroupDrainConfirmPopup(), bodyTotalHeight)
	} else if m.apollon.active {
		body = m.renderOverlay(body, m.renderApollonDrainConfirmPopup(), bodyTotalHeight)
	} else if m.qdbCoup.active {
		body = m.renderOverlay(body, m.renderQDBCoupConfirmPopup(), bodyTotalHeight)
	} else if m.qdbCoupDone.active {
		body = m.renderOverlay(body, m.renderQDBCoupResultPopup(), bodyTotalHeight)
	} else if m.fsEdit.active {
		body = m.renderOverlay(body, m.renderFSConfigStatusEditPopup(), bodyTotalHeight)
	} else if m.alert.active {
		body = m.renderOverlay(body, m.renderErrorAlert(), bodyTotalHeight)
	}

	body = m.normalizeRenderedBlock(body, bodyTotalHeight)
	middle := body
	if commandHeight > 0 {
		commandPanel := m.normalizeRenderedBlock(m.renderCommandPanel(commandHeight), commandHeight)
		middle = body + "\n" + commandPanel
	}
	return m.styles.app.Render(header + "\n" + middle + "\n" + footer)
}

func (m model) startupLoading() bool {
	switch m.activeView {
	case viewMGM, viewQDB:
		return len(m.mgms) == 0 && (m.fstStatsLoading || m.fstsLoading || m.fileSystemsLoading)
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

func (m model) renderBodyWithPopup(body string, height int) string {
	return m.renderOverlay(body, m.renderFilterPopup(), height)
}

func (m model) renderBodyWithEditPopup(body string, height int) string {
	var popup string
	if m.edit.stage == editStageInput {
		popup = m.renderSpaceStatusEditPopup()
	} else if m.edit.stage == editStageConfirm {
		popup = m.renderSpaceStatusConfirmPopup()
	}
	return m.renderOverlay(body, popup, height)
}

func (m model) onViewChanged() (tea.Model, tea.Cmd) {
	m.persistUIState()
	switch m.activeView {
	case viewNamespace:
		return m.maybeLoadNamespace()
	case viewSpaces:
		if !m.spacesLoading && len(m.spaces) == 0 && m.spacesErr == nil {
			m.spacesLoading = true
			m.spacesErr = nil
			return m, loadSpacesCmd(m.client)
		}
		if m.spaceStatusActive {
			return m.maybeLoadSpaceStatus(m.spaceStatusTarget)
		}
		return m, nil
	case viewGroups:
		if !m.groupsLoading && len(m.groups) == 0 && m.groupsErr == nil {
			m.groupsLoading = true
			m.groupsErr = nil
			return m, loadGroupsCmd(m.client)
		}
		return m, nil
	case viewVID:
		if !m.vidLoading && len(m.vidRecords) == 0 && m.vidErr == nil {
			m.vidLoading = true
			m.vidErr = nil
			return m, loadVIDCmd(m.client, m.vidMode)
		}
		return m, nil
	case viewAccess:
		if !m.accessLoading && len(m.accessRecords) == 0 && m.accessErr == nil {
			m.accessLoading = true
			m.accessErr = nil
			return m, loadAccessCmd(m.client)
		}
		return m, nil
	case viewNamespaceStats:
		cmds := make([]tea.Cmd, 0, 2)
		if !m.nsStatsLoading && m.namespaceStats == (eos.NamespaceStats{}) && m.nsStatsErr == nil {
			m.nsStatsLoading = true
			m.nsStatsErr = nil
			cmds = append(cmds, loadNamespaceStatsCmd(m.client))
		}
		if !m.inspectorLoading && !hasInspectorStatsData(m.inspectorStats) && m.inspectorErr == nil {
			m.inspectorLoading = true
			m.inspectorErr = nil
			cmds = append(cmds, loadInspectorCmd(m.client))
		}
		if !m.fstStatsLoading && m.nodeStats == (eos.NodeStats{}) && m.nodeStatsErr == nil {
			m.fstStatsLoading = true
			m.nodeStatsErr = nil
			cmds = append(cmds, loadNodeStatsCmd(m.client))
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
		return m, tea.Batch(loadIOShapingViewCmd(m.client, m.ioShapingMode), ioShapingTickCmd(), loadIOShapingPolicyDataCmd(m.client, m.ioShapingMode), ioShapingPolicyTickCmd())
	default:
		return m, nil
	}
}

func (m model) refreshActiveView() (tea.Model, tea.Cmd) {
	switch m.activeView {
	case viewNamespace:
		m.nsLoaded = false
		m.nsLoading = true
		m.nsErr = nil
		m.status = fmt.Sprintf("Refreshing namespace %s...", m.directory.Path)
		return m, loadDirectoryCmd(m.client, m.directory.Path)
	case viewSpaces:
		if m.spaceStatusActive {
			m.spaceStatusLoading = true
			m.spaceStatusErr = nil
			m.status = fmt.Sprintf("Refreshing space status for %s...", m.spaceStatusTarget)
			return m, loadSpaceStatusCmd(m.client, m.spaceStatusTarget)
		}
		m.spacesLoading = true
		m.spacesErr = nil
		m.status = "Refreshing spaces..."
		return m, loadSpacesCmd(m.client)
	case viewGroups:
		m.groupsLoading = true
		m.groupsErr = nil
		m.status = "Refreshing groups..."
		return m, loadGroupsCmd(m.client)
	case viewNamespaceStats:
		m.nsStatsLoading = true
		m.fstStatsLoading = true
		m.inspectorLoading = true
		m.nsStatsErr = nil
		m.nodeStatsErr = nil
		m.inspectorErr = nil
		m.status = "Refreshing general stats..."
		return m, tea.Batch(loadNamespaceStatsCmd(m.client), loadNodeStatsCmd(m.client), loadInspectorCmd(m.client))
	case viewSpaceStatus:
		m.spaceStatusLoading = true
		m.spaceStatusErr = nil
		m.status = fmt.Sprintf("Refreshing space status for %s...", m.currentSpaceStatusName())
		return m, loadSpaceStatusCmd(m.client, m.currentSpaceStatusName())
	case viewIOShaping:
		m.ioShapingLoading = true
		m.ioShapingErr = nil
		m.status = "Refreshing IO shaping..."
		return m, tea.Batch(loadIOShapingViewCmd(m.client, m.ioShapingMode), loadIOShapingPolicyDataCmd(m.client, m.ioShapingMode))
	case viewVID:
		m.vidLoading = true
		m.vidErr = nil
		m.status = fmt.Sprintf("Refreshing VID scope %s...", m.vidMode.label())
		return m, loadVIDCmd(m.client, m.vidMode)
	case viewAccess:
		m.accessLoading = true
		m.accessErr = nil
		m.status = "Refreshing access rules..."
		return m, loadAccessCmd(m.client)
	case viewMGM, viewQDB:
		if m.mgmsLoading || m.mgmVersionsLoading {
			m.status = "MGM/QDB refresh already in progress..."
			return m, nil
		}
		m.mgmsLoading = true
		m.mgmsErr = nil
		m.mgmVersionsLoading = true
		m.mgmVersionsErr = nil
		m.status = "Refreshing MGM/QDB topology and versions..."
		return m, tea.Batch(loadMGMsCmd(m.client), reloadMGMVersionsCmd(m.client))
	default:
		m.fstStatsLoading = true
		m.fstsLoading = true
		m.mgmsLoading = true
		m.fileSystemsLoading = true
		m.spacesLoading = true
		m.nsStatsLoading = true
		m.nodeStatsErr = nil
		m.fstsErr = nil
		m.mgmsErr = nil
		m.fileSystemsErr = nil
		m.spacesErr = nil
		m.nsStatsErr = nil
		m.status = "Refreshing..."
		return m, loadInfraCmd(m.client)
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

func (m model) maybeLoadNamespace() (tea.Model, tea.Cmd) {
	if m.nsLoading {
		return m, nil
	}
	if m.nsLoaded {
		return m.startNamespaceAttrLoad(false)
	}

	m.nsLoading = true
	m.nsErr = nil
	m.status = fmt.Sprintf("Loading namespace %s...", m.directory.Path)
	return m, loadDirectoryCmd(m.client, m.directory.Path)
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
	m.nsAttrsLoading = true
	m.nsAttrsLoaded = false
	m.nsAttrsErr = nil
	m.nsAttrs = nil
	return m, loadNamespaceAttrsCmd(m.client, path)
}

func (m model) maybeLoadSpaceStatus(space string) (tea.Model, tea.Cmd) {
	if space == "" || m.client == nil {
		return m, nil
	}
	if !m.spaceStatusLoading && m.spaceStatusErr == nil && m.spaceStatusTarget == space && len(m.spaceStatus) > 0 {
		return m, nil
	}

	m.spaceStatusTarget = space
	m.spaceStatusLoading = true
	m.spaceStatusErr = nil
	m.status = fmt.Sprintf("Loading space status for %s...", space)
	return m, loadSpaceStatusCmd(m.client, space)
}

func (m model) computeClusterHealth() string {
	fsts := m.fsts
	fss := m.fileSystems
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
