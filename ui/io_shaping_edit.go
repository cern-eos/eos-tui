package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lobis/eos-tui/eos"
)

func (m model) selectedIOShapingRow() (ioShapingMergedRow, bool) {
	rows := m.ioShapingMergedRows()
	if len(rows) == 0 || m.ioShapingSelected < 0 || m.ioShapingSelected >= len(rows) {
		return ioShapingMergedRow{}, false
	}
	return rows[m.ioShapingSelected], true
}

func ioShapingTargetLabel(mode eos.IOShapingMode) string {
	switch mode {
	case eos.IOShapingUsers:
		return "UID"
	case eos.IOShapingGroups:
		return "GID"
	default:
		return "Application"
	}
}

func ioShapingTargetPrompt(mode eos.IOShapingMode) string {
	switch mode {
	case eos.IOShapingUsers:
		return "uid> "
	case eos.IOShapingGroups:
		return "gid> "
	default:
		return "app> "
	}
}

func (m model) ioShapingPolicyRecordForTarget(mode eos.IOShapingMode, targetID string) (eos.IOShapingPolicyRecord, bool) {
	policyType := "app"
	switch mode {
	case eos.IOShapingUsers:
		policyType = "uid"
	case eos.IOShapingGroups:
		policyType = "gid"
	}

	for _, policy := range m.ioShapingPolicies {
		if strings.ToLower(policy.Type) == policyType && policy.ID == targetID {
			return policy, true
		}
	}
	return eos.IOShapingPolicyRecord{}, false
}

func newIOShapingEditInput(prompt string) textinput.Model {
	input := textinput.New()
	input.Prompt = prompt
	input.CharLimit = 64
	input.Width = 24
	return input
}

func (m model) ioShapingPolicyEditForTarget(targetID string, createMode bool) ioShapingPolicyEdit {
	edit := ioShapingPolicyEdit{
		active:     true,
		stage:      ioShapingEditStageSelect,
		mode:       m.ioShapingMode,
		targetID:   targetID,
		createMode: createMode,
		selected:   ioShapingEditFieldEnabled,
		button:     buttonCancel,
		input:      newIOShapingEditInput("value> "),
	}

	if policy, ok := m.ioShapingPolicyRecordForTarget(m.ioShapingMode, targetID); ok {
		edit.hadPolicy = true
		edit.enabled = policy.Enabled
		edit.limitRead = formatIOShapingPolicyRate(policy.LimitReadBytesPerSec)
		edit.limitWrite = formatIOShapingPolicyRate(policy.LimitWriteBytesPerSec)
		edit.reservationRead = formatIOShapingPolicyRate(policy.ReservationReadBytesPerSec)
		edit.reservationWrite = formatIOShapingPolicyRate(policy.ReservationWriteBytesPerSec)
	} else {
		edit.enabled = true
		edit.limitRead = "0"
		edit.limitWrite = "0"
		edit.reservationRead = "0"
		edit.reservationWrite = "0"
	}

	return edit
}

func (m model) startIOShapingPolicyEdit() (tea.Model, tea.Cmd) {
	if !ioShapingModeHasPolicies(m.ioShapingMode) {
		m.status = "This IO shaping view is read-only"
		return m, nil
	}
	row, ok := m.selectedIOShapingRow()
	if !ok {
		return m, nil
	}
	if row.total {
		m.status = "The total IO shaping row is read-only"
		return m, nil
	}

	m.ioShapingEdit = m.ioShapingPolicyEditForTarget(row.id, false)
	return m, nil
}

func (m model) startIOShapingPolicyCreate() (tea.Model, tea.Cmd) {
	if !ioShapingModeHasPolicies(m.ioShapingMode) {
		m.status = "Switch to apps, users, or groups to create IO shaping policies"
		return m, nil
	}
	input := newIOShapingEditInput(ioShapingTargetPrompt(m.ioShapingMode))
	cmd := input.Focus()
	m.ioShapingEdit = ioShapingPolicyEdit{
		active:     true,
		stage:      ioShapingEditStageTarget,
		mode:       m.ioShapingMode,
		createMode: true,
		input:      input,
	}
	return m, cmd
}

func (m model) startIOShapingPolicyDeleteConfirm() (tea.Model, tea.Cmd) {
	if !ioShapingModeHasPolicies(m.ioShapingMode) {
		m.status = "This IO shaping view is read-only"
		return m, nil
	}
	row, ok := m.selectedIOShapingRow()
	if !ok || row.policy == nil {
		if ok && row.total {
			m.status = "The total IO shaping row is read-only"
			return m, nil
		}
		m.alert = errorAlert{
			active:  true,
			message: "No IO shaping policy is configured for the selected row.",
		}
		return m, nil
	}

	m.ioShapingEdit = ioShapingPolicyEdit{
		active:    true,
		stage:     ioShapingEditStageDeleteConfirm,
		mode:      m.ioShapingMode,
		targetID:  row.id,
		hadPolicy: true,
		button:    buttonCancel,
	}
	return m, nil
}

func (m model) updateIOShapingPolicyEditKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.ioShapingEdit.stage {
	case ioShapingEditStageTarget:
		switch msg.String() {
		case "esc":
			m.ioShapingEdit.active = false
			m.ioShapingEdit.err = ""
			m.ioShapingEdit.input.Blur()
			return m, nil
		case "enter":
			targetID := strings.TrimSpace(m.ioShapingEdit.input.Value())
			if targetID == "" {
				m.ioShapingEdit.err = "A target name is required."
				return m, nil
			}
			m.ioShapingEdit = m.ioShapingPolicyEditForTarget(targetID, true)
			return m, nil
		}

		var cmd tea.Cmd
		m.ioShapingEdit.input, cmd = m.ioShapingEdit.input.Update(msg)
		m.ioShapingEdit.err = ""
		return m, cmd
	case ioShapingEditStageSelect:
		switch msg.String() {
		case "esc":
			m.ioShapingEdit.active = false
			m.ioShapingEdit.err = ""
			return m, nil
		case "g":
			m.ioShapingEdit.selected = ioShapingEditFieldEnabled
			m.ioShapingEdit.err = ""
			return m, nil
		case "G":
			m.ioShapingEdit.selected = ioShapingEditFieldApply
			m.ioShapingEdit.err = ""
			return m, nil
		case "d":
			if !m.ioShapingEdit.hadPolicy {
				m.ioShapingEdit.err = "No existing policy to delete."
				return m, nil
			}
			m.ioShapingEdit.stage = ioShapingEditStageDeleteConfirm
			m.ioShapingEdit.button = buttonCancel
			return m, nil
		case "e":
			m.ioShapingEdit.enabled = !m.ioShapingEdit.enabled
			m.ioShapingEdit.err = ""
			return m, nil
		case "y":
			return m.applyIOShapingPolicyEdit()
		case "up", "k":
			if m.ioShapingEdit.selected > ioShapingEditFieldEnabled {
				m.ioShapingEdit.selected--
			}
		case "down", "j":
			if m.ioShapingEdit.selected < ioShapingEditFieldApply {
				m.ioShapingEdit.selected++
			}
		case "enter":
			m.ioShapingEdit.err = ""
			switch m.ioShapingEdit.selected {
			case ioShapingEditFieldEnabled:
				m.ioShapingEdit.enabled = !m.ioShapingEdit.enabled
				return m, nil
			case ioShapingEditFieldLimitRead, ioShapingEditFieldLimitWrite, ioShapingEditFieldReservationRead, ioShapingEditFieldReservationWrite:
				m.ioShapingEdit.stage = ioShapingEditStageInput
				m.ioShapingEdit.input.SetValue("")
				return m, m.ioShapingEdit.input.Focus()
			case ioShapingEditFieldApply:
				return m.applyIOShapingPolicyEdit()
			}
		}
	case ioShapingEditStageDeleteConfirm:
		switch msg.String() {
		case "esc":
			m.ioShapingEdit.active = false
			return m, nil
		case "g":
			m.ioShapingEdit.button = buttonCancel
		case "G":
			m.ioShapingEdit.button = buttonContinue
		case "left", "right", "tab", "shift+tab":
			if m.ioShapingEdit.button == buttonCancel {
				m.ioShapingEdit.button = buttonContinue
			} else {
				m.ioShapingEdit.button = buttonCancel
			}
		case "enter":
			if m.ioShapingEdit.button == buttonCancel {
				m.ioShapingEdit.active = false
				return m, nil
			}
			id := m.ioShapingEdit.targetID
			mode := m.ioShapingEdit.mode
			m.ioShapingEdit.active = false
			m.ioShapingLoading = true
			m.status = fmt.Sprintf("Deleting IO shaping policy for %s", id)
			return m, runIOShapingPolicyRemoveCmd(m.client, mode, id)
		}
	case ioShapingEditStageInput:
		switch msg.String() {
		case "esc":
			m.ioShapingEdit.stage = ioShapingEditStageSelect
			m.ioShapingEdit.err = ""
			m.ioShapingEdit.input.Blur()
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.ioShapingEdit.input.Value())
			if _, err := parseIOShapingRate(value); err != nil {
				m.ioShapingEdit.err = err.Error()
				return m, nil
			}
			m.ioShapingEdit.setValueForField(m.ioShapingEdit.selected, value)
			m.ioShapingEdit.stage = ioShapingEditStageSelect
			m.ioShapingEdit.err = ""
			m.ioShapingEdit.input.Blur()
			return m, nil
		}

		var cmd tea.Cmd
		m.ioShapingEdit.input, cmd = m.ioShapingEdit.input.Update(msg)
		m.ioShapingEdit.err = ""
		return m, cmd
	}

	return m, nil
}

func (m model) applyIOShapingPolicyEdit() (tea.Model, tea.Cmd) {
	update, err := m.ioShapingEdit.policyUpdate()
	if err != nil {
		m.ioShapingEdit.err = err.Error()
		return m, nil
	}
	m.ioShapingEdit.active = false
	m.ioShapingLoading = true
	m.status = fmt.Sprintf("Updating IO shaping policy for %s", update.ID)
	return m, runIOShapingPolicySetCmd(m.client, update)
}

func (edit ioShapingPolicyEdit) policyUpdate() (eos.IOShapingPolicyUpdate, error) {
	limitRead, err := parseIOShapingRate(edit.limitRead)
	if err != nil {
		return eos.IOShapingPolicyUpdate{}, fmt.Errorf("limit read: %w", err)
	}
	limitWrite, err := parseIOShapingRate(edit.limitWrite)
	if err != nil {
		return eos.IOShapingPolicyUpdate{}, fmt.Errorf("limit write: %w", err)
	}
	reservationRead, err := parseIOShapingRate(edit.reservationRead)
	if err != nil {
		return eos.IOShapingPolicyUpdate{}, fmt.Errorf("reservation read: %w", err)
	}
	reservationWrite, err := parseIOShapingRate(edit.reservationWrite)
	if err != nil {
		return eos.IOShapingPolicyUpdate{}, fmt.Errorf("reservation write: %w", err)
	}

	return eos.IOShapingPolicyUpdate{
		Mode:                        edit.mode,
		ID:                          edit.targetID,
		Enabled:                     edit.enabled,
		LimitReadBytesPerSec:        limitRead,
		LimitWriteBytesPerSec:       limitWrite,
		ReservationReadBytesPerSec:  reservationRead,
		ReservationWriteBytesPerSec: reservationWrite,
	}, nil
}

func (edit ioShapingPolicyEdit) valueForField(field ioShapingEditField) string {
	switch field {
	case ioShapingEditFieldLimitRead:
		return edit.limitRead
	case ioShapingEditFieldLimitWrite:
		return edit.limitWrite
	case ioShapingEditFieldReservationRead:
		return edit.reservationRead
	case ioShapingEditFieldReservationWrite:
		return edit.reservationWrite
	default:
		return ""
	}
}

func (edit *ioShapingPolicyEdit) setValueForField(field ioShapingEditField, value string) {
	switch field {
	case ioShapingEditFieldLimitRead:
		edit.limitRead = value
	case ioShapingEditFieldLimitWrite:
		edit.limitWrite = value
	case ioShapingEditFieldReservationRead:
		edit.reservationRead = value
	case ioShapingEditFieldReservationWrite:
		edit.reservationWrite = value
	}
}

func (m model) renderIOShapingPolicyEditPopup() string {
	targetLabel := ioShapingTargetLabel(m.ioShapingEdit.mode)

	if m.ioShapingEdit.stage == ioShapingEditStageTarget {
		lines := []string{
			m.styles.popupTitle.Render("New IO Shaping Policy"),
			fmt.Sprintf("Enter %s to configure:", strings.ToLower(targetLabel)),
			"",
			m.ioShapingEdit.input.View(),
			"",
			m.styles.status.Render("enter continue  •  esc cancel"),
		}
		if m.ioShapingEdit.err != "" {
			lines = append(lines, "", m.styles.error.Render(m.ioShapingEdit.err))
		}
		return m.renderModal(lines, lipgloss.Color("62"), 0)
	}

	title := "Edit IO Shaping Policy"
	if m.ioShapingEdit.createMode {
		title = "New IO Shaping Policy"
	}

	lines := []string{
		m.styles.popupTitle.Render(title),
		fmt.Sprintf("%s: %s", targetLabel, m.styles.value.Render(m.ioShapingEdit.targetID)),
		fmt.Sprintf("Existing policy: %s", m.styles.value.Render(boolLabel(m.ioShapingEdit.hadPolicy))),
		"",
	}

	rows := []struct {
		field ioShapingEditField
		text  string
	}{
		{ioShapingEditFieldEnabled, fmt.Sprintf("%-18s %s", "Enabled", boolLabel(m.ioShapingEdit.enabled))},
		{ioShapingEditFieldLimitRead, fmt.Sprintf("%-18s %s", "Limit Read B/s", m.ioShapingEdit.limitRead)},
		{ioShapingEditFieldLimitWrite, fmt.Sprintf("%-18s %s", "Limit Write B/s", m.ioShapingEdit.limitWrite)},
		{ioShapingEditFieldReservationRead, fmt.Sprintf("%-18s %s", "Reservation Read B/s", m.ioShapingEdit.reservationRead)},
		{ioShapingEditFieldReservationWrite, fmt.Sprintf("%-18s %s", "Reservation Write B/s", m.ioShapingEdit.reservationWrite)},
		{ioShapingEditFieldApply, "Apply changes"},
	}

	for _, row := range rows {
		prefix := "  "
		text := row.text
		if row.field == m.ioShapingEdit.selected {
			if m.ioShapingEdit.stage == ioShapingEditStageInput && row.field != ioShapingEditFieldApply && row.field != ioShapingEditFieldEnabled {
				text = fmt.Sprintf("%s  %s", row.text, m.styles.status.Render("(editing)"))
			}
			lines = append(lines, m.styles.selected.Render("▶ "+text))
		} else {
			lines = append(lines, prefix+text)
		}
	}

	if m.ioShapingEdit.stage == ioShapingEditStageInput {
		lines = append(lines, "", m.ioShapingEdit.input.View())
		lines = append(lines, m.styles.status.Render("enter save  •  esc cancel"))
	} else if m.ioShapingEdit.stage == ioShapingEditStageDeleteConfirm {
		cancelBtn := "[ Cancel ]"
		deleteBtn := "[ Delete ]"
		if m.ioShapingEdit.button == buttonCancel {
			cancelBtn = m.styles.selected.Render(cancelBtn)
		} else {
			deleteBtn = m.styles.selected.Render(deleteBtn)
		}
		lines = []string{
			m.styles.popupTitle.Render("Delete IO Shaping Policy"),
			fmt.Sprintf("%s: %s", targetLabel, m.styles.value.Render(m.ioShapingEdit.targetID)),
			"",
			"This will remove the configured shaping policy.",
			"",
			lipgloss.JoinHorizontal(lipgloss.Left, cancelBtn, "  ", deleteBtn),
			"",
			m.styles.status.Render("g cancel  •  G delete  •  enter confirm  •  esc close"),
		}
	} else {
		lines = append(lines, "", m.styles.status.Render("↑↓ select  •  e toggle enabled  •  y apply  •  enter edit/toggle/apply  •  d delete  •  esc cancel"))
	}
	if m.ioShapingEdit.stage != ioShapingEditStageDeleteConfirm {
		lines = append(lines, m.styles.status.Render("values accept raw bytes/s or KB/MB/GB suffixes"))
	}
	if m.ioShapingEdit.err != "" {
		lines = append(lines, "", m.styles.error.Render(m.ioShapingEdit.err))
	}

	return m.renderModal(lines, lipgloss.Color("62"), 0)
}

func parseIOShapingRate(raw string) (uint64, error) {
	value := strings.TrimSpace(strings.ToUpper(raw))
	value = strings.TrimSuffix(value, "/S")
	value = strings.ReplaceAll(value, " ", "")
	if value == "" {
		return 0, nil
	}

	if n, err := strconv.ParseUint(value, 10, 64); err == nil {
		return n, nil
	}

	split := 0
	for split < len(value) {
		ch := value[split]
		if (ch >= '0' && ch <= '9') || ch == '.' {
			split++
			continue
		}
		break
	}
	if split == 0 || split == len(value) {
		return 0, fmt.Errorf("use bytes/s like 15000000 or a suffix like 15MB")
	}

	numberPart := value[:split]
	unitPart := value[split:]
	multiplier := float64(0)
	switch unitPart {
	case "B":
		multiplier = 1
	case "K", "KB":
		multiplier = 1e3
	case "M", "MB":
		multiplier = 1e6
	case "G", "GB":
		multiplier = 1e9
	case "T", "TB":
		multiplier = 1e12
	default:
		return 0, fmt.Errorf("unsupported rate unit %q", unitPart)
	}

	number, err := strconv.ParseFloat(numberPart, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid rate %q", raw)
	}
	if number < 0 {
		return 0, fmt.Errorf("rate must be non-negative")
	}
	return uint64(math.Round(number * multiplier)), nil
}

func formatIOShapingPolicyRate(value float64) string {
	return strconv.FormatUint(uint64(math.Round(value)), 10)
}

func boolLabel(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
