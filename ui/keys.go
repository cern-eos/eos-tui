package ui

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lobis/eos-tui/eos"
)

func (m model) updateFSTKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	fsts := m.visibleFSTs()
	half := max(1, m.height/6)
	switch msg.String() {
	case "/":
		m.fstFilter.column = m.fstColumnSelected
		m.openFilterPopup()
		return m, nil
	case "left":
		m.fstColumnSelected = max(0, m.fstColumnSelected-1)
		m.status = fmt.Sprintf("Selected node column: %s", m.fstSelectedColumnLabel())
	case "right":
		m.fstColumnSelected = min(nodeColumnCount()-1, m.fstColumnSelected+1)
		m.status = fmt.Sprintf("Selected node column: %s", m.fstSelectedColumnLabel())
	case "S":
		m.fstSort = m.nextNodeSortState()
		m.fstSelected = clampIndex(0, len(m.visibleFSTs()))
		m.status = fmt.Sprintf("Node sort: %s", m.fstSortStateLabel())
	case "o":
		return m.startNodeStatusToggleConfirm()
	case "c":
		delete(m.fstFilter.filters, m.fstColumnSelected)
		m.fstFilter.column = m.fstColumnSelected
		m.status = fmt.Sprintf("Cleared node filter on %s", m.fstSelectedColumnLabel())
	case "up", "k":
		if m.fstSelected > 0 {
			m.fstSelected--
		}
	case "down", "j":
		if m.fstSelected < len(fsts)-1 {
			m.fstSelected++
		}
	case "ctrl+u":
		m.fstSelected = max(0, m.fstSelected-half)
	case "ctrl+d":
		m.fstSelected = min(len(fsts)-1, m.fstSelected+half)
	case "g":
		m.fstSelected = 0
	case "G":
		m.fstSelected = max(0, len(fsts)-1)
	}

	return m, nil
}

func (m model) updateNodeStatusKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.nodeStatus.active = false
		return m, nil
	case "g":
		m.nodeStatus.button = buttonCancel
	case "G":
		m.nodeStatus.button = buttonContinue
	case "left", "right", "tab", "shift+tab":
		if m.nodeStatus.button == buttonCancel {
			m.nodeStatus.button = buttonContinue
		} else {
			m.nodeStatus.button = buttonCancel
		}
	case "enter":
		if m.nodeStatus.button == buttonCancel {
			m.nodeStatus.active = false
			return m, nil
		}
		host := m.nodeStatus.host
		port := m.nodeStatus.port
		target := m.nodeStatus.target
		m.nodeStatus.active = false
		m.status = fmt.Sprintf("Setting node %s:%d %s...", host, port, target)
		return m, runNodeStatusCmd(m.client, host, port, target)
	}
	return m, nil
}

func (m model) updateMGMKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.topologySelectableRows())
	half := max(1, m.height/6)
	switch msg.String() {
	case "up", "k":
		if m.mgmSelected > 0 {
			m.mgmSelected--
		}
	case "down", "j":
		if m.mgmSelected < n-1 {
			m.mgmSelected++
		}
	case "ctrl+u":
		m.mgmSelected = max(0, m.mgmSelected-half)
	case "ctrl+d":
		m.mgmSelected = min(n-1, m.mgmSelected+half)
	case "g":
		m.mgmSelected = 0
	case "G":
		m.mgmSelected = max(0, n-1)
	case "c":
		return m.startQDBCoupConfirm()
	}
	return m, nil
}

func (m model) updateFileSystemKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	fileSystems := m.visibleFileSystems()
	half := max(1, m.height/6)
	switch msg.String() {
	case "/":
		m.fsFilter.column = m.fsColumnSelected
		m.openFilterPopup()
		return m, nil
	case "enter":
		return m.openFSConfigStatusEdit()
	case "A":
		return m.openFSConfigStatusEditAll()
	case "x":
		return m.startApollonDrainConfirm()
	case "left":
		m.fsColumnSelected = max(0, m.fsColumnSelected-1)
		m.status = fmt.Sprintf("Selected filesystem column: %s", m.fsSelectedColumnLabel())
	case "right":
		m.fsColumnSelected = min(fsColumnCount()-1, m.fsColumnSelected+1)
		m.status = fmt.Sprintf("Selected filesystem column: %s", m.fsSelectedColumnLabel())
	case "S":
		m.fsSort = m.nextFileSystemSortState()
		m.fsSelected = clampIndex(0, len(m.visibleFileSystems()))
		m.status = fmt.Sprintf("Filesystem sort: %s", m.fsSortStateLabel())
	case "c":
		delete(m.fsFilter.filters, m.fsColumnSelected)
		m.fsFilter.column = m.fsColumnSelected
		m.status = fmt.Sprintf("Cleared filesystem filter on %s", m.fsSelectedColumnLabel())
	case "up", "k":
		if m.fsSelected > 0 {
			m.fsSelected--
		}
	case "down", "j":
		if m.fsSelected < len(fileSystems)-1 {
			m.fsSelected++
		}
	case "ctrl+u":
		m.fsSelected = max(0, m.fsSelected-half)
	case "ctrl+d":
		m.fsSelected = min(len(fileSystems)-1, m.fsSelected+half)
	case "g":
		m.fsSelected = 0
	case "G":
		m.fsSelected = max(0, len(fileSystems)-1)
	}

	return m, nil
}

func (m model) updateNamespaceKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.namespaceNavigationLoading() {
		m.status = fmt.Sprintf("Opening %s; wait for the directory request to finish", m.nsRequestedPath)
		return m, nil
	}
	half := max(1, m.height/6)
	entries := m.visibleNamespaceEntries()
	selectionChanged := false
	switch msg.String() {
	case "/":
		m.openFilterPopup()
		return m, nil
	case "up", "k":
		if m.nsSelected > 0 {
			m.nsSelected--
			selectionChanged = true
		}
	case "down", "j":
		if m.nsSelected < len(entries)-1 {
			m.nsSelected++
			selectionChanged = true
		}
	case "ctrl+u":
		m.nsSelected = max(0, m.nsSelected-half)
		selectionChanged = true
	case "ctrl+d":
		m.nsSelected = min(len(entries)-1, m.nsSelected+half)
		selectionChanged = true
	case "G":
		m.nsSelected = max(0, len(entries)-1)
		selectionChanged = true
	case "g":
		m.nsSelected = 0
		selectionChanged = true
	case "backspace", "left":
		parent := parentPath(m.directory.Path)
		if parent != m.directory.Path {
			m.nsFilter.filters = map[int]string{}
			m.nsSelected = 0
			m.status = fmt.Sprintf("Opening %s...", parent)
			return m.requestDirectory(parent)
		}
	case "enter", "a":
		return m.startNamespaceAttrEdit()
	case "m":
		return m.startNamespaceMkdir()
	case ":":
		return m.startNamespaceGoTo()
	case "right":
		entry, ok := m.selectedNamespaceEntry()
		if ok && entry.Kind == eos.EntryKindContainer {
			m.nsFilter.filters = map[int]string{}
			m.nsSelected = 0
			m.status = fmt.Sprintf("Opening %s...", entry.Path)
			return m.requestDirectory(entry.Path)
		}
	}

	if selectionChanged {
		m = m.rememberNamespaceDetailContent()
		return m.startNamespaceAttrLoad(false)
	}

	return m, nil
}

func (m model) updateNamespaceStatsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sections := m.statsSections()
	half := max(1, m.height/6)
	prevSelected := m.statsSectionSelected
	hasTable := m.statsCurrentSectionHasTable(sections)
	maxDetailSelected := max(0, m.statsDetailLineCount(sections)-1)
	switch msg.String() {
	case "/":
		if m.statsPaneFocus == statsFocusDetail && hasTable {
			m.openFilterPopup()
			return m, nil
		}
		m.status = "No table column selected for filtering"
	case "c":
		if m.statsPaneFocus == statsFocusDetail && hasTable {
			delete(m.statsFilter.filters, m.statsDetailColumnSelected)
			m.statsFilter.column = m.statsDetailColumnSelected
			m.statsDetailSelected = 0
			m.statsDetailOffsetX = 0
			m.status = fmt.Sprintf("Cleared stats filter on %s", m.statsFilterColumnLabel(m.statsDetailColumnSelected))
		} else if len(m.statsFilter.filters) > 0 {
			m.statsFilter.filters = map[int]string{}
			m.statsDetailSelected = 0
			m.statsDetailOffsetX = 0
			m.status = "Stats detail filters cleared"
		}
	case "left", "h":
		if m.statsPaneFocus == statsFocusDetail {
			if hasTable && m.statsDetailColumnSelected > 0 {
				m.statsDetailColumnSelected--
				m.status = fmt.Sprintf("Selected stats column: %s", m.statsFilterColumnLabel(m.statsDetailColumnSelected))
			} else {
				m.statsPaneFocus = statsFocusList
				m.status = "Focused stats section list"
			}
		}
	case "right", "l":
		if m.statsPaneFocus == statsFocusList {
			m.statsPaneFocus = statsFocusDetail
			if hasTable {
				m.statsDetailSelected = min(m.statsDetailSelected, maxDetailSelected)
				m.statsDetailColumnSelected = min(m.statsDetailColumnSelected, len(sections[m.statsSectionSelected].table.columns)-1)
				m.status = fmt.Sprintf("Focused stats details (%s)", m.statsFilterColumnLabel(m.statsDetailColumnSelected))
			} else {
				m.status = "Focused stats details"
			}
		} else if hasTable && m.statsDetailColumnSelected < len(sections[m.statsSectionSelected].table.columns)-1 {
			m.statsDetailColumnSelected++
			m.status = fmt.Sprintf("Selected stats column: %s", m.statsFilterColumnLabel(m.statsDetailColumnSelected))
		}
	case "up", "k":
		if m.statsPaneFocus == statsFocusDetail {
			m.statsDetailSelected = max(0, m.statsDetailSelected-1)
		} else {
			m.statsSectionSelected = max(0, m.statsSectionSelected-1)
		}
	case "down", "j":
		if m.statsPaneFocus == statsFocusDetail {
			m.statsDetailSelected = min(maxDetailSelected, m.statsDetailSelected+1)
		} else {
			m.statsSectionSelected = min(len(sections)-1, m.statsSectionSelected+1)
		}
	case "ctrl+u":
		if m.statsPaneFocus == statsFocusDetail {
			m.statsDetailSelected = max(0, m.statsDetailSelected-half)
		} else {
			m.statsSectionSelected = max(0, m.statsSectionSelected-half)
		}
	case "ctrl+d":
		if m.statsPaneFocus == statsFocusDetail {
			m.statsDetailSelected = min(maxDetailSelected, m.statsDetailSelected+half)
		} else {
			m.statsSectionSelected = min(len(sections)-1, m.statsSectionSelected+half)
		}
	case "g":
		if m.statsPaneFocus == statsFocusDetail {
			m.statsDetailSelected = 0
		} else {
			m.statsSectionSelected = 0
		}
	case "G":
		if m.statsPaneFocus == statsFocusDetail {
			m.statsDetailSelected = maxDetailSelected
		} else {
			m.statsSectionSelected = max(0, len(sections)-1)
		}
	}
	m.statsSectionSelected = clampIndex(m.statsSectionSelected, len(sections))
	if m.statsSectionSelected != prevSelected {
		m.statsPaneFocus = statsFocusList
		m.statsDetailSelected = 0
		m.statsDetailColumnSelected = 0
		m.statsDetailOffsetX = 0
		m.statsFilter.filters = map[int]string{}
	}
	if hasTable && len(sections) > 0 && sections[m.statsSectionSelected].table != nil {
		m.statsDetailColumnSelected = min(max(0, m.statsDetailColumnSelected), len(sections[m.statsSectionSelected].table.columns)-1)
	} else {
		m.statsDetailColumnSelected = 0
	}
	m.statsDetailSelected = min(max(0, m.statsDetailSelected), max(0, m.statsDetailLineCount(sections)-1))
	if len(sections) > 0 {
		m.statsDetailOffsetX = m.statsAdjustedOffsetX(sections[m.statsSectionSelected], m.currentStatsDetailContentWidth(sections))
	}
	return m, nil
}

func (m model) startNamespaceAttrEdit() (tea.Model, tea.Cmd) {
	targetPath := m.currentNamespaceAttrTargetPath()
	if m.nsAttrsLoading && m.nsAttrsTargetPath == targetPath {
		m.status = "Attributes are still loading..."
		return m, nil
	}
	if m.nsAttrsErr != nil && m.nsAttrsTargetPath == targetPath {
		m.status = "Cannot edit attributes until they load successfully"
		return m, nil
	}
	if !m.nsAttrsLoaded || m.nsAttrsTargetPath != targetPath || len(m.nsAttrs) == 0 {
		m.status = "No attributes available to edit"
		return m, nil
	}

	input := textinput.New()
	input.Prompt = "value> "
	input.CharLimit = 4096
	input.Width = 48

	m.nsAttrEdit = namespaceAttrEdit{
		active:     true,
		stage:      attrEditStageSelect,
		targetPath: targetPath,
		attrs:      append([]eos.NamespaceAttr(nil), m.nsAttrs...),
		selected:   0,
		recursive:  false,
		input:      input,
	}
	return m, nil
}

func (m model) updateNamespaceAttrEditKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.nsAttrEdit.stage {
	case attrEditStageSelect:
		switch msg.String() {
		case "esc":
			m.nsAttrEdit.active = false
			return m, nil
		case "r":
			m.nsAttrEdit.recursive = !m.nsAttrEdit.recursive
		case "g":
			m.nsAttrEdit.selected = 0
		case "G":
			m.nsAttrEdit.selected = max(0, len(m.nsAttrEdit.attrs)-1)
		case "up", "k":
			if m.nsAttrEdit.selected > 0 {
				m.nsAttrEdit.selected--
			}
		case "down", "j":
			if m.nsAttrEdit.selected < len(m.nsAttrEdit.attrs)-1 {
				m.nsAttrEdit.selected++
			}
		case "enter":
			attr := m.nsAttrEdit.attrs[m.nsAttrEdit.selected]
			m.nsAttrEdit.stage = attrEditStageInput
			m.nsAttrEdit.input.SetValue(attr.Value)
			return m, m.nsAttrEdit.input.Focus()
		}
		return m, nil
	case attrEditStageInput:
		switch msg.String() {
		case "esc":
			m.nsAttrEdit.active = false
			return m, nil
		case "enter":
			attr := m.nsAttrEdit.attrs[m.nsAttrEdit.selected]
			m.nsAttrEdit.active = false
			return m, runNamespaceAttrSetCmd(m.client, m.nsAttrEdit.targetPath, attr.Key, m.nsAttrEdit.input.Value(), m.nsAttrEdit.recursive)
		}

		var cmd tea.Cmd
		m.nsAttrEdit.input, cmd = m.nsAttrEdit.input.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m model) startNamespaceGoTo() (tea.Model, tea.Cmd) {
	input := textinput.New()
	input.Prompt = "path> "
	input.CharLimit = 4096
	input.Width = 60
	input.SetValue(m.directory.Path)
	input.CursorEnd()

	m.nsGoTo = namespaceGoTo{
		active: true,
		input:  input,
	}
	return m, m.nsGoTo.input.Focus()
}

func (m model) updateNamespaceGoToKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.nsGoTo.active = false
		return m, nil
	case "enter":
		target := resolveNamespacePath(m.directory.Path, m.nsGoTo.input.Value())
		m.nsGoTo.active = false
		if target == m.directory.Path {
			m.status = fmt.Sprintf("Already at %s", target)
			return m, nil
		}
		m.nsFilter.filters = map[int]string{}
		m.nsSelected = 0
		m.status = fmt.Sprintf("Opening %s...", target)
		return m.requestDirectory(target)
	}

	var cmd tea.Cmd
	m.nsGoTo.input, cmd = m.nsGoTo.input.Update(msg)
	return m, cmd
}

func (m model) startNamespaceMkdir() (tea.Model, tea.Cmd) {
	input := textinput.New()
	input.Prompt = "name> "
	input.CharLimit = 4096
	input.Width = 48

	m.nsMkdir = namespaceMkdir{
		active: true,
		input:  input,
	}
	return m, m.nsMkdir.input.Focus()
}

func (m model) updateNamespaceMkdirKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.nsMkdir.active = false
		return m, nil
	case "enter":
		target := resolveNamespacePath(m.directory.Path, m.nsMkdir.input.Value())
		if target == "" || target == m.directory.Path {
			m.status = "Enter a new directory name"
			return m, nil
		}
		m.nsMkdir.active = false
		m.status = fmt.Sprintf("Creating directory %s...", target)
		return m, runNamespaceMkdirCmd(m.client, target)
	}

	var cmd tea.Cmd
	m.nsMkdir.input, cmd = m.nsMkdir.input.Update(msg)
	return m, cmd
}

func (m model) updateSpacesKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	spaces := m.visibleSpaces()
	half := max(1, m.height/6)
	switch msg.String() {
	case "/":
		m.spaceFilter.column = m.spacesColumnSelected
		m.openFilterPopup()
		return m, nil
	case "S":
		m.spaceSort = m.nextSpaceSortState()
		m.spacesSelected = clampIndex(0, len(m.visibleSpaces()))
		m.status = fmt.Sprintf("Space sort: %s", m.spaceSortStateLabel())
	case "c":
		delete(m.spaceFilter.filters, m.spacesColumnSelected)
		m.spaceFilter.column = m.spacesColumnSelected
		m.spacesSelected = clampIndex(0, len(m.visibleSpaces()))
		m.status = fmt.Sprintf("Cleared space filter on %s", m.spaceSelectedColumnLabel())
	case "enter":
		return m.openSelectedSpaceStatus()
	case "up", "k":
		if m.spacesSelected > 0 {
			m.spacesSelected--
		}
	case "down", "j":
		if m.spacesSelected < len(spaces)-1 {
			m.spacesSelected++
		}
	case "ctrl+u":
		m.spacesSelected = max(0, m.spacesSelected-half)
	case "ctrl+d":
		if len(spaces) > 0 {
			m.spacesSelected = min(len(spaces)-1, m.spacesSelected+half)
		}
	case "g":
		m.spacesSelected = 0
	case "G":
		m.spacesSelected = max(0, len(spaces)-1)
	case "left":
		m.spacesColumnSelected = max(0, m.spacesColumnSelected-1)
		m.status = fmt.Sprintf("Selected space column: %s", m.spaceSelectedColumnLabel())
	case "right":
		m.spacesColumnSelected = min(spaceColumnCount()-1, m.spacesColumnSelected+1)
		m.status = fmt.Sprintf("Selected space column: %s", m.spaceSelectedColumnLabel())
	}

	return m, nil
}

func (m model) updateSpaceStatusKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	half := max(1, m.height/6)
	switch msg.String() {
	case "left", "backspace":
		m.spaceStatusActive = false
		m.status = "Returned to spaces list"
		return m, nil
	case "up", "k":
		if m.spaceStatusSelected > 0 {
			m.spaceStatusSelected--
		}
	case "down", "j":
		if m.spaceStatusSelected < len(m.spaceStatus)-1 {
			m.spaceStatusSelected++
		}
	case "ctrl+u":
		m.spaceStatusSelected = max(0, m.spaceStatusSelected-half)
	case "ctrl+d":
		m.spaceStatusSelected = min(len(m.spaceStatus)-1, m.spaceStatusSelected+half)
	case "g":
		m.spaceStatusSelected = 0
	case "G":
		m.spaceStatusSelected = max(0, len(m.spaceStatus)-1)
	}

	return m, nil
}

func (m model) updateIOShapingKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	half := max(1, m.height/6)
	rowCount := len(m.ioShapingMergedRows())
	if m.ioShapingMode == eos.IOShapingPressure {
		rowCount = len(m.ioShapingPressure)
	}
	switch msg.String() {
	case "a":
		return m.changeIOShapingMode(eos.IOShapingApps)
	case "u":
		return m.changeIOShapingMode(eos.IOShapingUsers)
	case "g":
		return m.changeIOShapingMode(eos.IOShapingGroups)
	case "n":
		return m.changeIOShapingMode(eos.IOShapingNodes)
	case "p":
		return m.changeIOShapingMode(eos.IOShapingPressure)
	case "N":
		return m.startIOShapingPolicyCreate()
	case "m":
		if !m.ioShapingConfigLoaded {
			m.status = "Loading IO shaping controller limits state..."
			if m.ioShapingConfigLoading {
				return m, nil
			}
			m.ioShapingConfigLoading = true
			return m, loadIOShapingConfigCmd(m.client, m.ioShapingGeneration)
		}
		nextEnabled := !m.ioShapingConfig.LimitsEnabled
		if nextEnabled {
			m.status = "Enabling IO shaping controller limits..."
		} else {
			m.status = "Disabling IO shaping controller limits..."
		}
		return m, runIOShapingLimitsToggleCmd(m.client, nextEnabled)
	case "up", "k":
		if m.ioShapingSelected > 0 {
			m.ioShapingSelected--
		}
	case "down", "j":
		if m.ioShapingSelected < rowCount-1 {
			m.ioShapingSelected++
		}
	case "ctrl+u":
		m.ioShapingSelected = max(0, m.ioShapingSelected-half)
	case "ctrl+d":
		m.ioShapingSelected = min(rowCount-1, m.ioShapingSelected+half)
	case "G":
		m.ioShapingSelected = max(0, rowCount-1)
	case "enter":
		return m.startIOShapingPolicyEdit()
	case "d":
		return m.startIOShapingPolicyDeleteConfirm()
	}
	m.ioShapingSelected = clampIndex(m.ioShapingSelected, rowCount)
	return m, nil
}

func (m model) changeIOShapingMode(mode eos.IOShapingMode) (tea.Model, tea.Cmd) {
	if m.ioShapingMode == mode {
		return m, nil
	}
	m.ioShapingGeneration++
	m.ioShapingMode = mode
	m.ioShapingSelected = 0
	m.ioShaping = nil
	m.ioShapingPressure = nil
	m.ioShapingPolicies = nil
	m.ioShapingLoading = true
	m.ioShapingErr = nil
	m.markIOShapingPolicyLoading()
	return m, tea.Batch(
		loadIOShapingViewCmd(m.client, mode, m.ioShapingGeneration),
		loadIOShapingPolicyDataCmd(m.client, mode, m.ioShapingGeneration),
	)
}

func (m model) updateGroupKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	groups := m.visibleGroups()
	switch msg.String() {
	case "enter":
		return m.startGroupDrainConfirm()
	case "A":
		return m.startGroupStatusEditAll()
	case "up", "k":
		m.groupsSelected = max(0, m.groupsSelected-1)
	case "down", "j":
		m.groupsSelected = min(len(groups)-1, m.groupsSelected+1)
	case "ctrl+d", "pgdown":
		m.groupsSelected = min(len(groups)-1, m.groupsSelected+10)
	case "ctrl+u", "pgup":
		m.groupsSelected = max(0, m.groupsSelected-10)
	case "left", "h":
		m.groupsColumnSelected = max(0, m.groupsColumnSelected-1)
	case "right", "l":
		m.groupsColumnSelected = min(groupColumnCount()-1, m.groupsColumnSelected+1)
	case "S":
		sortCol := groupSortColumn(m.groupsColumnSelected)
		if m.groupSort.column == int(sortCol) {
			m.groupSort.desc = !m.groupSort.desc
		} else {
			m.groupSort.column = int(sortCol)
			m.groupSort.desc = false
		}
	case "/":
		m.openFilterPopup()
	}
	return m, nil
}

func (m model) updateVIDKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	records := m.vidRecords
	half := max(1, m.height/6)
	switch msg.String() {
	case "left", "h":
		return m.changeVIDMode(m.vidMode.next(-1))
	case "right", "l":
		return m.changeVIDMode(m.vidMode.next(1))
	case "up", "k":
		if m.vidSelected > 0 {
			m.vidSelected--
		}
	case "down", "j":
		if m.vidSelected < len(records)-1 {
			m.vidSelected++
		}
	case "ctrl+u":
		m.vidSelected = max(0, m.vidSelected-half)
	case "ctrl+d":
		m.vidSelected = min(len(records)-1, m.vidSelected+half)
	case "g":
		m.vidSelected = 0
	case "G":
		m.vidSelected = max(0, len(records)-1)
	}

	return m, nil
}

func (m model) changeVIDMode(mode vidListMode) (tea.Model, tea.Cmd) {
	if mode == m.vidMode {
		return m, nil
	}
	m.vidGeneration++
	m.vidMode = mode
	m.vidRecords = nil
	m.vidSelected = 0
	m.vidLoading = true
	m.vidErr = nil
	m.status = fmt.Sprintf("Loading VID scope %s...", mode.label())
	return m, loadVIDCmd(m.client, mode, m.vidGeneration)
}

func (m model) updateAccessKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	records := m.visibleAccessRecords()
	half := max(1, m.height/6)
	switch msg.String() {
	case "/":
		m.accessFilter.column = m.accessColumnSelected
		m.openFilterPopup()
		return m, nil
	case "c":
		delete(m.accessFilter.filters, m.accessColumnSelected)
		m.accessFilter.column = m.accessColumnSelected
		m.accessSelected = clampIndex(m.accessSelected, len(m.visibleAccessRecords()))
		m.status = fmt.Sprintf("Cleared access filter on %s", m.accessFilterColumnLabel())
	case "left":
		m.accessColumnSelected = max(0, m.accessColumnSelected-1)
		m.accessFilter.column = m.accessColumnSelected
		m.status = fmt.Sprintf("Selected access column: %s", m.accessFilterColumnLabel())
	case "right":
		m.accessColumnSelected = min(int(accessFilterValue), m.accessColumnSelected+1)
		m.accessFilter.column = m.accessColumnSelected
		m.status = fmt.Sprintf("Selected access column: %s", m.accessFilterColumnLabel())
	case "enter":
		return m.startAccessActionPopup()
	case "s":
		return m.startAccessStallPopup()
	case "up", "k":
		if m.accessSelected > 0 {
			m.accessSelected--
		}
	case "down", "j":
		if m.accessSelected < len(records)-1 {
			m.accessSelected++
		}
	case "ctrl+u":
		m.accessSelected = max(0, m.accessSelected-half)
	case "ctrl+d":
		m.accessSelected = min(len(records)-1, m.accessSelected+half)
	case "g":
		m.accessSelected = 0
	case "G":
		m.accessSelected = max(0, len(records)-1)
	}

	return m, nil
}

func (m model) updateAccessActionKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.accessAction.focusInput {
		switch msg.String() {
		case "esc":
			m.accessAction.active = false
			m.status = "Access action cancelled"
			return m, nil
		case "enter":
			seconds, err := strconv.Atoi(m.accessAction.input.Value())
			if err != nil || seconds <= 0 {
				m.status = "Stall seconds must be a positive integer"
				return m, nil
			}
			m.accessAction.active = false
			m.status = fmt.Sprintf("Running eos access set stall %d...", seconds)
			return m, runAccessStallCmd(m.client, seconds)
		default:
			var cmd tea.Cmd
			m.accessAction.input, cmd = m.accessAction.input.Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "esc":
		m.accessAction.active = false
		m.status = "Access action cancelled"
		return m, nil
	case "g":
		m.accessAction.selected = 0
	case "G":
		m.accessAction.selected = max(0, len(m.accessAction.actions)-1)
	case "up", "k":
		if m.accessAction.selected > 0 {
			m.accessAction.selected--
		}
	case "down", "j":
		if m.accessAction.selected < len(m.accessAction.actions)-1 {
			m.accessAction.selected++
		}
	case "enter":
		if len(m.accessAction.actions) == 0 {
			m.accessAction.active = false
			return m, nil
		}
		action := m.accessAction.actions[m.accessAction.selected]
		m.accessAction.active = false
		switch action.kind {
		case accessActionAllow, accessActionUnallow, accessActionBan, accessActionUnban:
			m.status = fmt.Sprintf("Running eos access %s...", action.command)
			return m, runAccessRuleCmd(m.client, accessActionVerb(action.kind), m.accessAction.record.Category, m.accessAction.record.Value)
		case accessActionSetStall:
			return m.startAccessStallPopup()
		}
	}
	return m, nil
}

func (m model) updateGroupDrainKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.groupDrain.active = false
		return m, nil
	case "g":
		if m.groupDrain.confirm {
			m.groupDrain.button = buttonCancel
		} else {
			m.groupDrain.selected = 0
		}
	case "G":
		if m.groupDrain.confirm {
			m.groupDrain.button = buttonContinue
		} else {
			m.groupDrain.selected = len(groupStatusOptions) - 1
		}
	case "left", "right", "tab", "shift+tab":
		if m.groupDrain.confirm {
			if m.groupDrain.button == buttonCancel {
				m.groupDrain.button = buttonContinue
			} else {
				m.groupDrain.button = buttonCancel
			}
		}
	case "up", "k":
		if !m.groupDrain.confirm && m.groupDrain.selected > 0 {
			m.groupDrain.selected--
		}
	case "down", "j":
		if !m.groupDrain.confirm && m.groupDrain.selected < len(groupStatusOptions)-1 {
			m.groupDrain.selected++
		}
	case "enter":
		if m.groupDrain.applyAll {
			if m.groupDrain.confirm {
				if m.groupDrain.button == buttonCancel {
					m.groupDrain.active = false
					return m, nil
				}
				chosen := groupStatusOptions[m.groupDrain.selected]
				targets := append([]string(nil), m.groupDrain.targets...)
				m.groupDrain.active = false
				m.status = fmt.Sprintf("Setting %d groups to %s...", len(targets), chosen)
				return m, runBatchGroupSetCmd(m.client, targets, chosen)
			}
			m.groupDrain.confirm = true
			m.groupDrain.button = buttonCancel
			return m, nil
		}
		group := m.groupDrain.group
		chosen := groupStatusOptions[m.groupDrain.selected]
		m.groupDrain.active = false
		m.status = fmt.Sprintf("Setting group %s to %s...", group, chosen)
		return m, runGroupSetCmd(m.client, group, chosen)
	}

	return m, nil
}

func (m model) updateSpaceStatusEditKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.edit.active = false
		return m, nil
	case "g":
		if !m.edit.focusInput {
			m.edit.button = buttonCancel
			return m, nil
		}
	case "G":
		if !m.edit.focusInput {
			m.edit.button = buttonContinue
			return m, nil
		}
	case "tab", "shift+tab":
		m.edit.focusInput = !m.edit.focusInput
		if m.edit.focusInput {
			return m, m.edit.input.Focus()
		} else {
			m.edit.input.Blur()
			return m, nil
		}
	case "up", "down":
		if m.edit.stage == editStageInput {
			m.edit.focusInput = !m.edit.focusInput
			if m.edit.focusInput {
				return m, m.edit.input.Focus()
			} else {
				m.edit.input.Blur()
				return m, nil
			}
		}
	case "left", "right":
		if !m.edit.focusInput {
			if m.edit.button == buttonCancel {
				m.edit.button = buttonContinue
			} else {
				m.edit.button = buttonCancel
			}
		}
	case "enter":
		if !m.edit.focusInput && m.edit.button == buttonCancel {
			m.edit.active = false
			return m, nil
		}

		if m.edit.stage == editStageInput {
			if m.edit.focusInput || m.edit.button == buttonContinue {
				m.edit.stage = editStageConfirm
				m.edit.button = buttonCancel
				m.edit.focusInput = false
				m.edit.input.Blur()
				return m, nil
			}
		} else if m.edit.stage == editStageConfirm {
			if m.edit.button == buttonContinue {
				space := m.edit.space
				key := m.edit.record.Key
				value := m.edit.input.Value()
				m.edit.active = false
				m.status = fmt.Sprintf("Updating %s on space %s...", key, space)
				return m, runSpaceConfigCmd(m.client, space, key, value)
			}
		}
	}

	if m.edit.focusInput {
		var cmd tea.Cmd
		m.edit.input, cmd = m.edit.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) updateFSConfigStatusEditKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.fsEdit.active = false
		return m, nil
	case "g":
		if m.fsEdit.confirm {
			m.fsEdit.button = buttonCancel
		} else {
			m.fsEdit.selected = 0
		}
	case "G":
		if m.fsEdit.confirm {
			m.fsEdit.button = buttonContinue
		} else {
			m.fsEdit.selected = len(configStatusOptions) - 1
		}
	case "left", "right", "tab", "shift+tab":
		if m.fsEdit.confirm {
			if m.fsEdit.button == buttonCancel {
				m.fsEdit.button = buttonContinue
			} else {
				m.fsEdit.button = buttonCancel
			}
		}
	case "up", "k":
		if !m.fsEdit.confirm && m.fsEdit.selected > 0 {
			m.fsEdit.selected--
		}
	case "down", "j":
		if !m.fsEdit.confirm && m.fsEdit.selected < len(configStatusOptions)-1 {
			m.fsEdit.selected++
		}
	case "enter":
		chosen := configStatusOptions[m.fsEdit.selected]
		if m.fsEdit.applyAll {
			if m.fsEdit.confirm {
				if m.fsEdit.button == buttonCancel {
					m.fsEdit.active = false
					return m, nil
				}
				targets := append([]fileSystemTarget(nil), m.fsEdit.targets...)
				m.fsEdit.active = false
				m.status = fmt.Sprintf("Setting %d filesystems to %s...", len(targets), chosen)
				return m, runBatchFsConfigStatusCmd(m.client, targets, chosen)
			}
			m.fsEdit.confirm = true
			m.fsEdit.button = buttonCancel
			return m, nil
		}
		fsID := m.fsEdit.fsID
		m.fsEdit.active = false
		return m, runFsConfigStatusCmd(m.client, fsID, chosen)
	}
	return m, nil
}

func (m model) updateApollonDrainKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.apollon.active = false
		return m, nil
	case "g":
		m.apollon.button = buttonCancel
	case "G":
		m.apollon.button = buttonContinue
	case "left", "right":
		if m.apollon.button == buttonCancel {
			m.apollon.button = buttonContinue
		} else {
			m.apollon.button = buttonCancel
		}
	case "enter":
		if m.apollon.button == buttonCancel {
			m.apollon.active = false
			return m, nil
		}
		fsID := m.apollon.fsID
		instance := m.apollon.instance
		m.apollon.active = false
		m.status = fmt.Sprintf("Starting Apollon drain for filesystem %d on %s...", fsID, instance)
		return m, runApollonDrainCmd(m.client, fsID, instance)
	}
	return m, nil
}

func (m model) updateQDBCoupKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.qdbCoup.active = false
		return m, nil
	case "g":
		m.qdbCoup.button = buttonCancel
	case "G":
		m.qdbCoup.button = buttonContinue
	case "left", "right", "tab", "shift+tab":
		if m.qdbCoup.button == buttonCancel {
			m.qdbCoup.button = buttonContinue
		} else {
			m.qdbCoup.button = buttonCancel
		}
	case "enter":
		if m.qdbCoup.button == buttonCancel {
			m.qdbCoup.active = false
			return m, nil
		}
		host := m.qdbCoup.host
		m.qdbCoup.active = false
		m.status = fmt.Sprintf("Attempting QDB raft coup on %s...", host)
		return m, runQDBCoupCmd(m.client, host)
	}
	return m, nil
}

func (m model) updateQDBCoupResultKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		m.qdbCoupDone.active = false
	}
	return m, nil
}

func (m model) updatePopup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeFilterPopup("Filter selection cancelled")
		return m, nil
	case "enter":
		view := m.popup.view
		m.applyPopupSelection()
		if view == viewNamespace {
			return m.startNamespaceAttrLoad(false)
		}
		return m, nil
	case "up", "down", "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		m.popup.table, cmd = m.popup.table.Update(msg)
		m.popup.navigated = true
		return m, cmd
	}

	var cmd tea.Cmd
	m.popup.input, cmd = m.popup.input.Update(msg)
	m.popup.navigated = false
	m.updatePopupRows()
	return m, cmd
}

func (m model) updateLogKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Filter input mode: type to grep, enter to apply, esc to cancel.
	if m.log.filtering {
		switch msg.String() {
		case "ctrl+c":
			m.logGeneration++
			m.log = logOverlay{}
			return m, nil
		case "esc":
			m.log.filtering = false
			m.log.input.Blur()
			m.log.filtered = applyLogFilter(m.log.allLines, m.log.filter)
			m.refreshLogViewportContent(false)
		case "enter":
			m.log.filtering = false
			m.log.input.Blur()
			m.log.filter = m.log.input.Value()
			m.log.filtered = applyLogFilter(m.log.allLines, m.log.filter)
			m.refreshLogViewportContent(false)
			m.log.vp.GotoBottom()
		default:
			var cmd tea.Cmd
			m.log.input, cmd = m.log.input.Update(msg)
			// Live filter as user types.
			m.log.filtered = applyLogFilter(m.log.allLines, m.log.input.Value())
			m.refreshLogViewportContent(false)
			m.log.vp.GotoBottom()
			return m, cmd
		}
		return m, nil
	}

	// Normal navigation.
	switch msg.String() {
	case "ctrl+c", "esc", "q":
		m.logGeneration++
		m.log = logOverlay{}
	case "/":
		m.log.filtering = true
		m.log.input.SetValue(m.log.filter)
		m.log.input.Focus()
	case "f":
		m.log.plain = !m.log.plain
		m.refreshLogViewportContent(true)
	case "w":
		m.log.wrap = !m.log.wrap
		m.refreshLogViewportContent(true)
	case "n":
		return m.switchLogSource(1)
	case "p":
		return m.switchLogSource(-1)
	case "t":
		m.logGeneration++
		m.log.tailing = !m.log.tailing
		m.log.inFlight = false
		if m.log.tailing {
			m.log.inFlight = true
			return m, tea.Batch(
				loadLogCmd(m.client, m.currentLogTarget(), m.logGeneration),
				logTickCmd(m.logGeneration),
			)
		}
	case "r":
		if m.log.inFlight {
			return m, nil
		}
		m.log.inFlight = true
		return m, loadLogCmd(m.client, m.currentLogTarget(), m.logGeneration)
	case "g":
		m.log.vp.GotoTop()
	case "G":
		m.log.vp.GotoBottom()
	default:
		var cmd tea.Cmd
		m.log.vp, cmd = m.log.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}
