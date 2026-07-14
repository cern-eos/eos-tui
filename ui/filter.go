package ui

import (
	"cmp"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/table"

	"github.com/lobis/eos-tui/eos"
)

var globMatcherCache sync.Map

func (m model) visibleFSTs() []eos.FstRecord {
	fsts := make([]eos.FstRecord, 0, len(m.fsts))
	for _, node := range m.fsts {
		t := strings.ToLower(node.Type)
		// A node is an FST if it's explicitly type 'fst' OR if it has registered filesystems.
		if t == "fst" || node.FileSystemCount > 0 {
			fsts = append(fsts, node)
		}
	}

	if len(m.fstFilter.filters) > 0 {
		filtered := make([]eos.FstRecord, 0, len(fsts))
		for _, node := range fsts {
			if m.matchesNodeFilters(node) {
				filtered = append(filtered, node)
			}
		}
		fsts = filtered
	}
	if m.fstSort.column >= 0 {
		sort.SliceStable(fsts, func(i, j int) bool {
			return m.lessNode(fsts[i], fsts[j])
		})
	}
	return fsts
}

func (m model) visibleFileSystems() []eos.FileSystemRecord {
	fileSystems := append([]eos.FileSystemRecord(nil), m.fileSystems...)
	if len(m.fsFilter.filters) > 0 {
		filtered := make([]eos.FileSystemRecord, 0, len(fileSystems))
		for _, fs := range fileSystems {
			if m.matchesFileSystemFilters(fs) {
				filtered = append(filtered, fs)
			}
		}
		fileSystems = filtered
	}
	if m.fsSort.column >= 0 {
		sort.SliceStable(fileSystems, func(i, j int) bool {
			return m.lessFileSystem(fileSystems[i], fileSystems[j])
		})
	}
	return fileSystems
}

func (m model) visibleGroups() []eos.GroupRecord {
	groups := append([]eos.GroupRecord(nil), m.groups...)
	if len(m.groupFilter.filters) > 0 {
		filtered := make([]eos.GroupRecord, 0, len(groups))
		for _, g := range groups {
			if m.matchesGroupFilters(g) {
				filtered = append(filtered, g)
			}
		}
		groups = filtered
	}
	if m.groupSort.column >= 0 {
		sort.SliceStable(groups, func(i, j int) bool {
			return m.lessGroup(groups[i], groups[j])
		})
	}
	return groups
}

func (m model) visibleAccessRecords() []eos.AccessRecord {
	records := append([]eos.AccessRecord(nil), m.accessRecords...)
	if len(m.accessFilter.filters) == 0 {
		return records
	}

	filtered := make([]eos.AccessRecord, 0, len(records))
	for _, record := range records {
		if m.matchesAccessFilters(record) {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func (m model) visibleSpaces() []eos.SpaceRecord {
	spaces := append([]eos.SpaceRecord(nil), m.spaces...)
	if len(m.spaceFilter.filters) > 0 {
		filtered := make([]eos.SpaceRecord, 0, len(spaces))
		for _, s := range spaces {
			if m.matchesSpaceFilters(s) {
				filtered = append(filtered, s)
			}
		}
		spaces = filtered
	}
	if m.spaceSort.column >= 0 {
		sort.SliceStable(spaces, func(i, j int) bool {
			return m.lessSpace(spaces[i], spaces[j])
		})
	}
	return spaces
}

func (m model) visibleNamespaceEntries() []eos.Entry {
	entries := append([]eos.Entry(nil), m.directory.Entries...)
	if len(m.nsFilter.filters) == 0 {
		return entries
	}

	filtered := make([]eos.Entry, 0, len(entries))
	for _, entry := range entries {
		if m.matchesNamespaceFilters(entry) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (m model) matchesNodeFilters(node eos.FstRecord) bool {
	for column, query := range m.fstFilter.filters {
		if query == "" {
			continue
		}
		if !matchesFilterQuery(m.fstFilterValueForColumn(node, column), query) {
			return false
		}
	}
	return true
}

func (m model) matchesNodeFiltersExcept(node eos.FstRecord, excludeColumn int) bool {
	for col, query := range m.fstFilter.filters {
		if col == excludeColumn || query == "" {
			continue
		}
		if !matchesFilterQuery(m.fstFilterValueForColumn(node, col), query) {
			return false
		}
	}
	return true
}

func (m model) matchesFileSystemFilters(fs eos.FileSystemRecord) bool {
	for column, query := range m.fsFilter.filters {
		if query == "" {
			continue
		}
		if !matchesFilterQuery(m.fsFilterValueForColumn(fs, column), query) {
			return false
		}
	}
	return true
}

func (m model) matchesFileSystemFiltersExcept(fs eos.FileSystemRecord, excludeColumn int) bool {
	for col, query := range m.fsFilter.filters {
		if col == excludeColumn || query == "" {
			continue
		}
		if !matchesFilterQuery(m.fsFilterValueForColumn(fs, col), query) {
			return false
		}
	}
	return true
}

func (m model) matchesGroupFilters(g eos.GroupRecord) bool {
	for col, filter := range m.groupFilter.filters {
		if filter == "" {
			continue
		}
		if !matchesFilterQuery(m.groupFilterValueForColumn(g, col), filter) {
			return false
		}
	}
	return true
}

func (m model) matchesGroupFiltersExcept(g eos.GroupRecord, excludeColumn int) bool {
	for col, filter := range m.groupFilter.filters {
		if col == excludeColumn || filter == "" {
			continue
		}
		if !matchesFilterQuery(m.groupFilterValueForColumn(g, col), filter) {
			return false
		}
	}
	return true
}

func (m model) matchesSpaceFilters(s eos.SpaceRecord) bool {
	for col, filter := range m.spaceFilter.filters {
		if filter == "" {
			continue
		}
		if !matchesFilterQuery(m.spaceFilterValueForColumn(s, col), filter) {
			return false
		}
	}
	return true
}

func (m model) matchesSpaceFiltersExcept(s eos.SpaceRecord, excludeColumn int) bool {
	for col, filter := range m.spaceFilter.filters {
		if col == excludeColumn || filter == "" {
			continue
		}
		if !matchesFilterQuery(m.spaceFilterValueForColumn(s, col), filter) {
			return false
		}
	}
	return true
}

func (m model) matchesNamespaceFilters(entry eos.Entry) bool {
	for col, filter := range m.nsFilter.filters {
		if filter == "" {
			continue
		}
		if !matchesFilterQuery(m.namespaceFilterValueForColumn(entry, col), filter) {
			return false
		}
	}
	return true
}

func (m model) matchesAccessFilters(record eos.AccessRecord) bool {
	for col, filter := range m.accessFilter.filters {
		if filter == "" {
			continue
		}
		if !matchesFilterQuery(m.accessFilterValueForColumn(record, col), filter) {
			return false
		}
	}
	return true
}

func (m model) matchesAccessFiltersExcept(record eos.AccessRecord, excludeColumn int) bool {
	for col, filter := range m.accessFilter.filters {
		if col == excludeColumn || filter == "" {
			continue
		}
		if !matchesFilterQuery(m.accessFilterValueForColumn(record, col), filter) {
			return false
		}
	}
	return true
}

func (m model) matchesNamespaceFiltersExcept(entry eos.Entry, excludeColumn int) bool {
	for col, filter := range m.nsFilter.filters {
		if col == excludeColumn || filter == "" {
			continue
		}
		if !matchesFilterQuery(m.namespaceFilterValueForColumn(entry, col), filter) {
			return false
		}
	}
	return true
}

func matchesFilterQuery(value, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}

	value = strings.ToLower(value)
	if !strings.ContainsAny(query, "*?") {
		return strings.Contains(value, query)
	}

	return matchesGlobPattern(value, query)
}

func matchesGlobPattern(value, pattern string) bool {
	re := globPatternRegexp(pattern)
	if re == nil {
		return false
	}
	return re.MatchString(value)
}

func globPatternRegexp(pattern string) *regexp.Regexp {
	if cached, ok := globMatcherCache.Load(pattern); ok {
		if re, ok := cached.(*regexp.Regexp); ok {
			return re
		}
		return nil
	}

	var expr strings.Builder
	expr.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			expr.WriteString(".*")
		case '?':
			expr.WriteString(".")
		default:
			expr.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	expr.WriteString("$")

	re, err := regexp.Compile(expr.String())
	if err != nil {
		globMatcherCache.Store(pattern, (*regexp.Regexp)(nil))
		return nil
	}
	globMatcherCache.Store(pattern, re)
	return re
}

func (m model) fstFilterValue(node eos.FstRecord) string {
	return m.fstFilterValueForColumn(node, m.fstFilter.column)
}

func (m model) fstFilterValueForColumn(node eos.FstRecord, column int) string {
	switch fstFilterColumn(column) {
	case fstFilterHost:
		return node.Host
	case fstFilterPort:
		return fmt.Sprintf("%d", node.Port)
	case fstFilterGeotag:
		return node.Geotag
	case fstFilterStatus:
		return node.Status
	case fstFilterActivated:
		return node.Activated
	case fstFilterHeartbeatDelta:
		return fmt.Sprintf("%d", node.HeartbeatDelta)
	case fstFilterNoFS:
		return fmt.Sprintf("%d", node.FileSystemCount)
	case fstFilterEOSVersion:
		return node.EOSVersion
	case fstFilterType:
		return node.Type
	default:
		return node.Host
	}
}

func (m model) fsFilterValue(fs eos.FileSystemRecord) string {
	return m.fsFilterValueForColumn(fs, m.fsFilter.column)
}

func (m model) fsFilterValueForColumn(fs eos.FileSystemRecord, column int) string {
	switch fsFilterColumn(column) {
	case fsFilterPort:
		return fmt.Sprintf("%d", fs.Port)
	case fsFilterID:
		return fmt.Sprintf("%d", fs.ID)
	case fsFilterPath:
		return fs.Path
	case fsFilterGroup:
		return fs.SchedGroup
	case fsFilterGeotag:
		return fs.Geotag
	case fsFilterBoot:
		return fs.Boot
	case fsFilterConfigStatus:
		return fs.ConfigStatus
	case fsFilterDrain:
		return fs.DrainStatus
	case fsFilterUsage:
		return fmt.Sprintf("%.2f", usagePercent(fs.UsedBytes, fs.CapacityBytes))
	case fsFilterStatus:
		return fs.Active
	case fsFilterHealth:
		return fs.Health
	default:
		return fs.Host
	}
}

func (m model) groupFilterValue(g eos.GroupRecord) string {
	return m.groupFilterValueForColumn(g, m.groupFilter.column)
}

func (m model) accessFilterValue(record eos.AccessRecord) string {
	return m.accessFilterValueForColumn(record, m.accessFilter.column)
}

func (m model) spaceFilterValue(s eos.SpaceRecord) string {
	return m.spaceFilterValueForColumn(s, m.spaceFilter.column)
}

func (m model) spaceFilterValueForColumn(s eos.SpaceRecord, column int) string {
	switch spaceFilterColumn(column) {
	case spaceFilterName:
		return s.Name
	case spaceFilterType:
		return s.Type
	case spaceFilterStatus:
		return s.Status
	case spaceFilterGroups:
		return fmt.Sprintf("%d", s.Groups)
	case spaceFilterFiles:
		return fmt.Sprintf("%d", s.NumFiles)
	case spaceFilterDirs:
		return fmt.Sprintf("%d", s.NumContainers)
	case spaceFilterUsage:
		return fmt.Sprintf("%.2f", usagePercent(s.UsedBytes, s.CapacityBytes))
	default:
		return s.Name
	}
}

func (m model) namespaceFilterValueForColumn(entry eos.Entry, _ int) string {
	return strings.TrimSpace(fmt.Sprintf("%s %s %s", entry.Name, entry.Path, entryTypeLabel(entry)))
}

func (m model) groupFilterValueForColumn(g eos.GroupRecord, column int) string {
	switch groupFilterColumn(column) {
	case groupFilterName:
		return g.Name
	case groupFilterStatus:
		return g.Status
	case groupFilterNoFS:
		return fmt.Sprintf("%d", g.NoFS)
	case groupFilterCapacity:
		return humanBytes(g.CapacityBytes)
	case groupFilterUsed:
		return humanBytes(g.UsedBytes)
	case groupFilterFree:
		return humanBytes(g.FreeBytes)
	case groupFilterFiles:
		return fmt.Sprintf("%d", g.NumFiles)
	default:
		return g.Name
	}
}

func (m model) accessFilterValueForColumn(record eos.AccessRecord, column int) string {
	switch accessFilterColumn(column) {
	case accessFilterCategory:
		return record.Category
	case accessFilterRule:
		return record.Rule
	case accessFilterValue:
		return record.Value
	default:
		return record.Category
	}
}

func (m model) lessNode(a, b eos.FstRecord) bool {
	var primary int
	switch fstSortColumn(m.fstSort.column) {
	case fstSortType:
		primary = cmp.Compare(a.Type, b.Type)
	case fstSortHost:
		primary = cmp.Compare(a.Host, b.Host)
	case fstSortPort:
		primary = cmp.Compare(a.Port, b.Port)
	case fstSortStatus:
		primary = cmp.Compare(a.Status, b.Status)
	case fstSortGeotag:
		primary = cmp.Compare(a.Geotag, b.Geotag)
	case fstSortActivated:
		primary = cmp.Compare(a.Activated, b.Activated)
	case fstSortNoFS:
		primary = cmp.Compare(a.FileSystemCount, b.FileSystemCount)
	case fstSortHeartbeat:
		primary = cmp.Compare(a.HeartbeatDelta, b.HeartbeatDelta)
	case fstSortEOSVersion:
		primary = cmp.Compare(a.EOSVersion, b.EOSVersion)
	default:
		primary = cmp.Compare(a.Host, b.Host)
	}
	if primary != 0 {
		if m.fstSort.desc {
			return primary > 0
		}
		return primary < 0
	}

	if tie := cmp.Compare(a.Host, b.Host); tie != 0 {
		return tie < 0
	}
	return cmp.Compare(a.Port, b.Port) < 0
}

func (m model) lessFileSystem(a, b eos.FileSystemRecord) bool {
	var primary int
	switch fsSortColumn(m.fsSort.column) {
	case fsSortHost:
		primary = cmp.Compare(a.Host, b.Host)
	case fsSortPort:
		primary = cmp.Compare(a.Port, b.Port)
	case fsSortID:
		primary = cmp.Compare(a.ID, b.ID)
	case fsSortPath:
		primary = cmp.Compare(a.Path, b.Path)
	case fsSortGroup:
		primary = cmp.Compare(a.SchedGroup, b.SchedGroup)
	case fsSortGeotag:
		primary = cmp.Compare(a.Geotag, b.Geotag)
	case fsSortBoot:
		primary = cmp.Compare(a.Boot, b.Boot)
	case fsSortConfigStatus:
		primary = cmp.Compare(a.ConfigStatus, b.ConfigStatus)
	case fsSortDrain:
		primary = cmp.Compare(a.DrainStatus, b.DrainStatus)
	case fsSortUsed:
		primary = cmp.Compare(usagePercent(a.UsedBytes, a.CapacityBytes), usagePercent(b.UsedBytes, b.CapacityBytes))
	case fsSortStatus:
		primary = cmp.Compare(a.Active, b.Active)
	case fsSortHealth:
		primary = cmp.Compare(a.Health, b.Health)
	default:
		primary = cmp.Compare(a.ID, b.ID)
	}
	if primary != 0 {
		if m.fsSort.desc {
			return primary > 0
		}
		return primary < 0
	}

	if tie := cmp.Compare(a.ID, b.ID); tie != 0 {
		return tie < 0
	}
	if tie := cmp.Compare(a.Host, b.Host); tie != 0 {
		return tie < 0
	}
	return cmp.Compare(a.Path, b.Path) < 0
}

func (m model) lessGroup(a, b eos.GroupRecord) bool {
	var primary int
	switch groupSortColumn(m.groupSort.column) {
	case groupSortName:
		primary = cmp.Compare(a.Name, b.Name)
	case groupSortStatus:
		primary = cmp.Compare(a.Status, b.Status)
	case groupSortNoFS:
		primary = cmp.Compare(a.NoFS, b.NoFS)
	case groupSortCapacity:
		primary = cmp.Compare(a.CapacityBytes, b.CapacityBytes)
	case groupSortUsed:
		primary = cmp.Compare(a.UsedBytes, b.UsedBytes)
	case groupSortFree:
		primary = cmp.Compare(a.FreeBytes, b.FreeBytes)
	case groupSortFiles:
		primary = cmp.Compare(a.NumFiles, b.NumFiles)
	default:
		primary = cmp.Compare(a.Name, b.Name)
	}
	if primary != 0 {
		if m.groupSort.desc {
			return primary > 0
		}
		return primary < 0
	}

	if tie := cmp.Compare(a.Name, b.Name); tie != 0 {
		return tie < 0
	}
	return cmp.Compare(a.Status, b.Status) < 0
}

func (m model) lessSpace(a, b eos.SpaceRecord) bool {
	var primary int
	switch spaceSortColumn(m.spaceSort.column) {
	case spaceSortName:
		primary = cmp.Compare(a.Name, b.Name)
	case spaceSortType:
		primary = cmp.Compare(a.Type, b.Type)
	case spaceSortStatus:
		primary = cmp.Compare(a.Status, b.Status)
	case spaceSortGroups:
		primary = cmp.Compare(a.Groups, b.Groups)
	case spaceSortFiles:
		primary = cmp.Compare(a.NumFiles, b.NumFiles)
	case spaceSortDirs:
		primary = cmp.Compare(a.NumContainers, b.NumContainers)
	case spaceSortUsage:
		primary = cmp.Compare(usagePercent(a.UsedBytes, a.CapacityBytes), usagePercent(b.UsedBytes, b.CapacityBytes))
	default:
		primary = cmp.Compare(a.Name, b.Name)
	}
	if primary != 0 {
		if m.spaceSort.desc {
			return primary > 0
		}
		return primary < 0
	}

	if tie := cmp.Compare(a.Name, b.Name); tie != 0 {
		return tie < 0
	}
	return cmp.Compare(a.Status, b.Status) < 0
}

func equivalentNodeSortValue(column int, a, b eos.FstRecord) bool {
	switch fstSortColumn(column) {
	case fstSortType:
		return a.Type == b.Type
	case fstSortHost:
		return a.Host == b.Host
	case fstSortStatus:
		return a.Status == b.Status
	case fstSortGeotag:
		return a.Geotag == b.Geotag
	case fstSortActivated:
		return a.Activated == b.Activated
	case fstSortNoFS:
		return a.FileSystemCount == b.FileSystemCount
	case fstSortHeartbeat:
		return a.HeartbeatDelta == b.HeartbeatDelta
	case fstSortEOSVersion:
		return a.EOSVersion == b.EOSVersion
	default:
		return a.Host == b.Host
	}
}

func equivalentFileSystemSortValue(column int, a, b eos.FileSystemRecord) bool {
	switch fsSortColumn(column) {
	case fsSortHost:
		return a.Host == b.Host
	case fsSortPort:
		return a.Port == b.Port
	case fsSortID:
		return a.ID == b.ID
	case fsSortPath:
		return a.Path == b.Path
	case fsSortGroup:
		return a.SchedGroup == b.SchedGroup
	case fsSortGeotag:
		return a.Geotag == b.Geotag
	case fsSortBoot:
		return a.Boot == b.Boot
	case fsSortConfigStatus:
		return a.ConfigStatus == b.ConfigStatus
	case fsSortDrain:
		return a.DrainStatus == b.DrainStatus
	case fsSortUsed:
		return usagePercent(a.UsedBytes, a.CapacityBytes) == usagePercent(b.UsedBytes, b.CapacityBytes)
	case fsSortStatus:
		return a.Active == b.Active
	case fsSortHealth:
		return a.Health == b.Health
	default:
		return a.ID == b.ID
	}
}

func (m model) nextNodeSortState() sortState {
	selected := m.fstColumnSelected
	if m.fstSort.column != selected {
		return sortState{column: selected}
	}
	if !m.fstSort.desc {
		return sortState{column: selected, desc: true}
	}
	return sortState{column: int(fstSortNone)}
}

func (m model) nextFileSystemSortState() sortState {
	selected := m.fsColumnSelected
	if m.fsSort.column != selected {
		return sortState{column: selected}
	}
	if !m.fsSort.desc {
		return sortState{column: selected, desc: true}
	}
	return sortState{column: int(fsSortNone)}
}

func (m model) nextSpaceSortState() sortState {
	selected := m.spacesColumnSelected
	if m.spaceSort.column != selected {
		return sortState{column: selected}
	}
	if !m.spaceSort.desc {
		return sortState{column: selected, desc: true}
	}
	return sortState{column: int(spaceSortNone)}
}

func (m model) nodeColumnIsEnum(column int) bool {
	switch fstFilterColumn(column) {
	case fstFilterType, fstFilterStatus, fstFilterActivated:
		return true
	default:
		return false
	}
}

func (m model) fsColumnIsEnum(column int) bool {
	switch fsFilterColumn(column) {
	case fsFilterBoot, fsFilterConfigStatus, fsFilterDrain, fsFilterStatus:
		return true
	default:
		return false
	}
}

func nodeColumnCount() int {
	return 8 // the 8 navigable visible columns; fstFilterType/fstSortType are not user-navigable
}

func fsColumnCount() int {
	return 12
}

func groupColumnCount() int {
	return 7
}

func spaceColumnCount() int {
	return 7
}

func (m model) uniqueGroupValues(column int) []string {
	seen := make(map[string]bool)
	var values []string
	for _, g := range m.groups {
		val := m.groupFilterValueForColumn(g, column)
		if val != "" && !seen[val] {
			seen[val] = true
			values = append(values, val)
		}
	}
	sort.Strings(values)
	return values
}

func (m model) uniqueSpaceValues(column int) []string {
	seen := make(map[string]bool)
	var values []string
	for _, s := range m.spaces {
		val := m.spaceFilterValueForColumn(s, column)
		if val != "" && !seen[val] {
			seen[val] = true
			values = append(values, val)
		}
	}
	sort.Strings(values)
	return values
}

func (m model) fstSelectedColumnLabel() string {
	column := m.fstFilter.column
	m.fstFilter.column = m.fstColumnSelected
	label := m.fstFilterColumnLabel()
	m.fstFilter.column = column
	return label
}

func (m model) fsSelectedColumnLabel() string {
	column := m.fsFilter.column
	m.fsFilter.column = m.fsColumnSelected
	label := m.fsFilterColumnLabel()
	m.fsFilter.column = column
	return label
}

func (m model) spaceSelectedColumnLabel() string {
	column := m.spaceFilter.column
	m.spaceFilter.column = m.spacesColumnSelected
	label := m.spaceFilterColumnLabel()
	m.spaceFilter.column = column
	return label
}

func (m model) fstSortStateLabel() string {
	if m.fstSort.column < 0 {
		return "none"
	}
	return fmt.Sprintf("%s %s", m.fstSortColumnLabel(), sortDirectionLabel(m.fstSort.desc))
}

func (m model) fsSortStateLabel() string {
	if m.fsSort.column < 0 {
		return "none"
	}
	return fmt.Sprintf("%s %s", m.fsSortColumnLabel(), sortDirectionLabel(m.fsSort.desc))
}

func (m model) spaceSortStateLabel() string {
	if m.spaceSort.column < 0 {
		return "none"
	}
	return fmt.Sprintf("%s %s", m.spaceSortColumnLabel(), sortDirectionLabel(m.spaceSort.desc))
}

func (m model) fstFilterColumnLabel() string {
	switch fstFilterColumn(m.fstFilter.column) {
	case fstFilterHost:
		return "host"
	case fstFilterPort:
		return "port"
	case fstFilterGeotag:
		return "geotag"
	case fstFilterStatus:
		return "status"
	case fstFilterActivated:
		return "activated"
	case fstFilterHeartbeatDelta:
		return "heartbeatdelta"
	case fstFilterNoFS:
		return "nofs"
	case fstFilterEOSVersion:
		return "eos version"
	case fstFilterType:
		return "type"
	default:
		return "host"
	}
}

func (m model) fstSortColumnLabel() string {
	switch fstSortColumn(m.fstSort.column) {
	case fstSortHost:
		return "host"
	case fstSortPort:
		return "port"
	case fstSortGeotag:
		return "geotag"
	case fstSortStatus:
		return "status"
	case fstSortActivated:
		return "activated"
	case fstSortHeartbeat:
		return "heartbeatdelta"
	case fstSortNoFS:
		return "nofs"
	case fstSortEOSVersion:
		return "eos version"
	case fstSortType:
		return "type"
	case fstSortNone:
		return "none"
	default:
		return "host"
	}
}

func (m model) fsFilterColumnLabel() string {
	switch fsFilterColumn(m.fsFilter.column) {
	case fsFilterPort:
		return "port"
	case fsFilterID:
		return "id"
	case fsFilterPath:
		return "path"
	case fsFilterGroup:
		return "schedgroup"
	case fsFilterGeotag:
		return "geotag"
	case fsFilterBoot:
		return "boot"
	case fsFilterConfigStatus:
		return "configstatus"
	case fsFilterDrain:
		return "drain"
	case fsFilterUsage:
		return "usage %"
	case fsFilterStatus:
		return "active"
	case fsFilterHealth:
		return "health"
	default:
		return "host"
	}
}

func (m model) fsSortColumnLabel() string {
	switch fsSortColumn(m.fsSort.column) {
	case fsSortHost:
		return "host"
	case fsSortPort:
		return "port"
	case fsSortID:
		return "id"
	case fsSortPath:
		return "path"
	case fsSortGroup:
		return "schedgroup"
	case fsSortGeotag:
		return "geotag"
	case fsSortBoot:
		return "boot"
	case fsSortConfigStatus:
		return "configstatus"
	case fsSortDrain:
		return "drain"
	case fsSortUsed:
		return "usage %"
	case fsSortStatus:
		return "active"
	case fsSortHealth:
		return "health"
	case fsSortNone:
		return "none"
	default:
		return "host"
	}
}

func (m model) groupFilterColumnLabel() string {
	switch groupFilterColumn(m.groupFilter.column) {
	case groupFilterName:
		return "name"
	case groupFilterStatus:
		return "status"
	case groupFilterNoFS:
		return "nofs"
	case groupFilterCapacity:
		return "capacity"
	case groupFilterUsed:
		return "used"
	case groupFilterFree:
		return "free"
	case groupFilterFiles:
		return "files"
	default:
		return "name"
	}
}

func (m model) spaceFilterColumnLabel() string {
	switch spaceFilterColumn(m.spaceFilter.column) {
	case spaceFilterName:
		return "name"
	case spaceFilterType:
		return "type"
	case spaceFilterStatus:
		return "status"
	case spaceFilterGroups:
		return "groups"
	case spaceFilterFiles:
		return "files"
	case spaceFilterDirs:
		return "dirs"
	case spaceFilterUsage:
		return "usage %"
	default:
		return "name"
	}
}

func (m model) accessFilterColumnLabel() string {
	switch accessFilterColumn(m.accessFilter.column) {
	case accessFilterCategory:
		return "category"
	case accessFilterRule:
		return "rule"
	case accessFilterValue:
		return "value"
	default:
		return "category"
	}
}

func (m model) spaceSortColumnLabel() string {
	switch spaceSortColumn(m.spaceSort.column) {
	case spaceSortName:
		return "name"
	case spaceSortType:
		return "type"
	case spaceSortStatus:
		return "status"
	case spaceSortGroups:
		return "groups"
	case spaceSortFiles:
		return "files"
	case spaceSortDirs:
		return "dirs"
	case spaceSortUsage:
		return "usage %"
	case spaceSortNone:
		return "none"
	default:
		return "name"
	}
}

func (m model) groupSortColumnLabel() string {
	switch groupSortColumn(m.groupSort.column) {
	case groupSortName:
		return "name"
	case groupSortStatus:
		return "status"
	case groupSortNoFS:
		return "nofs"
	case groupSortCapacity:
		return "capacity"
	case groupSortUsed:
		return "used"
	case groupSortFree:
		return "free"
	case groupSortFiles:
		return "files"
	case groupSortNone:
		return "none"
	default:
		return "name"
	}
}

func (m model) activeFilterColumnLabel() string {
	switch m.activeView {
	case viewFileSystems:
		return m.fsFilterColumnLabel()
	case viewNamespaceStats:
		return m.statsFilterColumnLabel(m.statsDetailColumnSelected)
	case viewNamespace:
		return "entry"
	case viewSpaces:
		return m.spaceFilterColumnLabel()
	case viewGroups:
		return m.groupFilterColumnLabel()
	case viewAccess:
		return m.accessFilterColumnLabel()
	default:
		return m.fstFilterColumnLabel()
	}
}

func (m *model) openFilterPopup() {
	m.popup.active = true
	m.popup.view = m.activeView
	m.popup.navigated = false
	if m.activeView == viewFileSystems {
		m.popup.column = m.fsColumnSelected
		m.popup.input.SetValue(m.fsFilter.filters[m.fsColumnSelected])
	} else if m.activeView == viewNamespaceStats {
		m.popup.column = m.statsDetailColumnSelected
		m.popup.input.SetValue(m.statsFilter.filters[m.statsDetailColumnSelected])
	} else if m.activeView == viewNamespace {
		m.popup.column = namespaceFilterQueryColumn
		m.popup.input.SetValue(m.nsFilter.filters[namespaceFilterQueryColumn])
	} else if m.activeView == viewSpaces {
		m.popup.column = m.spacesColumnSelected
		m.popup.input.SetValue(m.spaceFilter.filters[m.spacesColumnSelected])
	} else if m.activeView == viewGroups {
		m.popup.column = m.groupsColumnSelected
		m.popup.input.SetValue(m.groupFilter.filters[m.groupsColumnSelected])
	} else if m.activeView == viewAccess {
		m.popup.column = m.accessColumnSelected
		m.popup.input.SetValue(m.accessFilter.filters[m.accessColumnSelected])
	} else {
		m.popup.column = m.fstColumnSelected
		m.popup.input.SetValue(m.fstFilter.filters[m.fstColumnSelected])
	}
	m.popup.input.CursorEnd()
	m.popup.input.Focus()
	m.popup.table.Focus()
	// Populate rows immediately so the table is ready for keyboard navigation
	// without requiring a text-input event first.
	m.updatePopupRows()
	m.popup.table.SetCursor(0)
	m.status = fmt.Sprintf("Select filter for %s", m.activeFilterColumnLabel())
}

func (m *model) closeFilterPopup(status string) {
	m.popup.active = false
	m.popup.navigated = false
	m.popup.input.Blur()
	m.popup.input.SetValue("")
	m.popup.values = nil
	m.popup.table.SetRows(nil)
	m.status = status
}

func (m *model) applyPopupSelection() {
	typedValue := strings.TrimSpace(m.popup.input.Value())
	row := m.popup.table.SelectedRow()
	value := typedValue
	if m.popup.navigated || typedValue == "" {
		if len(row) == 0 {
			m.closeFilterPopup("No filter value selected")
			return
		}
		value = row[0]
		if value == "(no matches)" && typedValue != "" {
			value = typedValue
		}
	}
	if value == "(no matches)" {
		m.closeFilterPopup("No matching filter value")
		return
	}
	if value == "(no filter)" {
		value = ""
	}

	switch m.popup.view {
	case viewFileSystems:
		m.fsFilter.column = m.popup.column
		if value == "" {
			delete(m.fsFilter.filters, m.popup.column)
		} else {
			m.fsFilter.filters[m.popup.column] = value
		}
		m.fsSelected = clampIndex(0, len(m.visibleFileSystems()))
		m.closeFilterPopup(fmt.Sprintf("Filesystem filters active: %d", len(m.fsFilter.filters)))
	case viewNamespace:
		m.nsFilter.column = m.popup.column
		if value == "" {
			delete(m.nsFilter.filters, m.popup.column)
		} else {
			m.nsFilter.filters[m.popup.column] = value
		}
		m.nsSelected = clampIndex(0, len(m.visibleNamespaceEntries()))
		m.closeFilterPopup(fmt.Sprintf("Namespace filters active: %d", len(m.nsFilter.filters)))
	case viewSpaces:
		m.spaceFilter.column = m.popup.column
		if value == "" {
			delete(m.spaceFilter.filters, m.popup.column)
		} else {
			m.spaceFilter.filters[m.popup.column] = value
		}
		m.spacesSelected = clampIndex(0, len(m.visibleSpaces()))
		m.closeFilterPopup(fmt.Sprintf("Space filters active: %d", len(m.spaceFilter.filters)))
	case viewGroups:
		m.groupFilter.column = m.popup.column
		if value == "" {
			delete(m.groupFilter.filters, m.popup.column)
		} else {
			m.groupFilter.filters[m.popup.column] = value
		}
		m.groupsSelected = clampIndex(0, len(m.visibleGroups()))
		m.closeFilterPopup(fmt.Sprintf("Group filters active: %d", len(m.groupFilter.filters)))
	case viewNamespaceStats:
		m.statsFilter.column = m.popup.column
		if value == "" {
			delete(m.statsFilter.filters, m.popup.column)
		} else {
			m.statsFilter.filters[m.popup.column] = value
		}
		m.statsPaneFocus = statsFocusDetail
		m.statsDetailSelected = 0
		m.statsDetailOffsetX = 0
		m.closeFilterPopup(fmt.Sprintf("Stats detail filters active: %d", len(m.statsFilter.filters)))
	case viewAccess:
		m.accessFilter.column = m.popup.column
		if value == "" {
			delete(m.accessFilter.filters, m.popup.column)
		} else {
			m.accessFilter.filters[m.popup.column] = value
		}
		m.accessSelected = clampIndex(0, len(m.visibleAccessRecords()))
		m.closeFilterPopup(fmt.Sprintf("Access filters active: %d", len(m.accessFilter.filters)))
	default:
		m.fstFilter.column = m.popup.column
		if value == "" {
			delete(m.fstFilter.filters, m.popup.column)
		} else {
			m.fstFilter.filters[m.popup.column] = value
		}
		m.fstSelected = clampIndex(0, len(m.visibleFSTs()))
		m.closeFilterPopup(fmt.Sprintf("Node filters active: %d", len(m.fstFilter.filters)))
	}
}

func (m *model) updatePopupRows() {
	needle := strings.TrimSpace(m.popup.input.Value())
	values := m.popupValues()
	rows := make([]table.Row, 0, len(values))
	for _, value := range values {
		label := value
		if label == "" {
			label = "(no filter)"
		}
		if needle == "" || matchesFilterQuery(label, needle) {
			rows = append(rows, table.Row{label})
		}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"(no matches)"}}
	}
	popupWidth := min(70, max(1, m.contentWidth()-4))
	m.popup.table.SetColumns([]table.Column{{Title: "value", Width: max(1, popupWidth-4)}})
	m.popup.table.SetRows(rows)
	m.popup.table.SetHeight(min(14, max(6, m.height/3)))
	m.popup.table.SetWidth(popupWidth)
	m.popup.table.SetCursor(0)
}

func (m model) popupValues() []string {
	values := []string{""}
	seen := map[string]bool{"": true}
	// Only show values that pass all *other* active filters so the list stays
	// consistent with what the user would actually see after applying the filter.
	switch m.popup.view {
	case viewFileSystems:
		for _, fs := range m.fileSystems {
			if !m.matchesFileSystemFiltersExcept(fs, m.popup.column) {
				continue
			}
			value := m.fsFilterValueForColumn(fs, m.popup.column)
			if !seen[value] {
				seen[value] = true
				values = append(values, value)
			}
		}
	case viewNamespace:
		for _, entry := range m.directory.Entries {
			if !m.matchesNamespaceFiltersExcept(entry, m.popup.column) {
				continue
			}
			value := entry.Name
			if value == "" {
				value = entry.Path
			}
			if !seen[value] {
				seen[value] = true
				values = append(values, value)
			}
		}
	case viewSpaces:
		for _, s := range m.spaces {
			if !m.matchesSpaceFiltersExcept(s, m.popup.column) {
				continue
			}
			value := m.spaceFilterValueForColumn(s, m.popup.column)
			if !seen[value] {
				seen[value] = true
				values = append(values, value)
			}
		}
	case viewGroups:
		for _, g := range m.groups {
			if !m.matchesGroupFiltersExcept(g, m.popup.column) {
				continue
			}
			value := m.groupFilterValueForColumn(g, m.popup.column)
			if !seen[value] {
				seen[value] = true
				values = append(values, value)
			}
		}
	case viewNamespaceStats:
		for _, value := range m.statsPopupValues() {
			if !seen[value] {
				seen[value] = true
				values = append(values, value)
			}
		}
	case viewAccess:
		for _, record := range m.accessRecords {
			if !m.matchesAccessFiltersExcept(record, m.popup.column) {
				continue
			}
			value := m.accessFilterValueForColumn(record, m.popup.column)
			if !seen[value] {
				seen[value] = true
				values = append(values, value)
			}
		}
	default:
		for _, node := range m.fsts {
			if !m.matchesNodeFiltersExcept(node, m.popup.column) {
				continue
			}
			value := m.fstFilterValueForColumn(node, m.popup.column)
			if !seen[value] {
				seen[value] = true
				values = append(values, value)
			}
		}
	}
	sort.Strings(values[1:])
	return values
}

func sortDirectionLabel(desc bool) string {
	if desc {
		return "desc"
	}
	return "asc"
}
