package ui

import (
	"fmt"
	"strings"

	"github.com/Gu1llaum-3/sshm/internal/config"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	syncActionSync  = "sync"
	syncActionPull  = "pull"
	syncActionPush  = "push"
	syncActionCheck = "check"
	syncActionClose = "close"

	syncActionToggleEnabled    = "toggle_enabled"
	syncActionToggleAuto       = "toggle_auto"
	syncActionToggleConfig     = "toggle_config"
	syncActionToggleIncludes   = "toggle_includes"
	syncActionTogglePublicKeys = "toggle_public_keys"
)

const (
	syncMenuActions = iota
	syncMenuSettings
)

const (
	syncRepoInput = iota
	syncBranchInput
	syncLocalPathInput
	syncAutoSyncTTLInput
	syncPublicKeyDirInput
	syncAuthorNameInput
	syncAuthorEmailInput
)

type syncMenuItem struct {
	label  string
	action string
}

type syncMenuModel struct {
	syncConfig config.SyncConfig
	mode       int
	selected   int
	inputs     []textinput.Model
	focused    int
	status     string
	err        string
	running    bool
	styles     Styles
	width      int
	height     int
}

type syncMenuActionMsg struct {
	action string
}

type syncMenuCancelMsg struct{}

type syncSettingsSaveMsg struct {
	syncConfig config.SyncConfig
	err        error
}

func NewSyncMenu(appConfig *config.AppConfig, styles Styles, width, height int) *syncMenuModel {
	syncConfig := config.GetDefaultSyncConfig()
	if appConfig != nil {
		syncConfig = appConfig.Sync
	}

	return &syncMenuModel{
		syncConfig: syncConfig,
		mode:       syncMenuActions,
		styles:     styles,
		width:      width,
		height:     height,
	}
}

func (m *syncMenuModel) Init() tea.Cmd {
	return nil
}

func (m *syncMenuModel) Update(msg tea.Msg) (*syncMenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.styles = NewStyles(m.width)
		return m, nil

	case tea.KeyMsg:
		if m.mode == syncMenuSettings {
			return m.updateSettings(msg)
		}
		return m.updateActions(msg)
	}
	return m, nil
}

func (m *syncMenuModel) updateActions(msg tea.KeyMsg) (*syncMenuModel, tea.Cmd) {
	items := m.menuItems()
	switch msg.String() {
	case "ctrl+c", "esc", "q":
		return m, func() tea.Msg { return syncMenuCancelMsg{} }
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(items)-1 {
			m.selected++
		}
	case "enter":
		if len(items) == 0 || m.selected >= len(items) {
			return m, nil
		}
		item := items[m.selected]
		if item.action == "settings" {
			m.mode = syncMenuSettings
			m.inputs = newSyncSettingsInputs(m.syncConfig)
			m.focused = 0
			return m, m.updateSettingsFocus()
		}
		if m.running && item.action != syncActionClose {
			return m, nil
		}
		return m, func() tea.Msg { return syncMenuActionMsg{action: item.action} }
	}
	return m, nil
}

func (m *syncMenuModel) updateSettings(msg tea.KeyMsg) (*syncMenuModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.mode = syncMenuActions
		m.inputs = nil
		return m, nil
	case "ctrl+s":
		return m, m.saveSettings()
	case "tab", "down", "enter":
		m.focused = (m.focused + 1) % len(m.inputs)
		return m, m.updateSettingsFocus()
	case "shift+tab", "up":
		m.focused = (m.focused - 1 + len(m.inputs)) % len(m.inputs)
		return m, m.updateSettingsFocus()
	}

	var cmd tea.Cmd
	m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
	return m, cmd
}

func (m *syncMenuModel) menuItems() []syncMenuItem {
	return []syncMenuItem{
		{"Sync now", syncActionSync},
		{"Pull from repo", syncActionPull},
		{"Push local changes", syncActionPush},
		{"Check repo availability", syncActionCheck},
		{"Settings", "settings"},
		{fmt.Sprintf("Toggle sync enabled (%s)", yesNo(m.syncConfig.Enabled)), syncActionToggleEnabled},
		{fmt.Sprintf("Toggle auto-sync on startup (%s)", yesNo(m.syncConfig.AutoSyncOnStartup)), syncActionToggleAuto},
		{fmt.Sprintf("Toggle SSH config sync (%s)", yesNo(m.syncConfig.ShouldSyncSSHConfig())), syncActionToggleConfig},
		{fmt.Sprintf("Toggle included config sync (%s)", yesNo(m.syncConfig.ShouldSyncIncludedConfigs())), syncActionToggleIncludes},
		{fmt.Sprintf("Toggle public key sync (%s)", yesNo(m.syncConfig.ShouldSyncPublicKeys())), syncActionTogglePublicKeys},
		{"Close", syncActionClose},
	}
}

func newSyncSettingsInputs(syncConfig config.SyncConfig) []textinput.Model {
	inputs := make([]textinput.Model, 7)
	fields := []struct {
		placeholder string
		value       string
		limit       int
		width       int
	}{
		{"git@github.com:user/sshm-sync.git", syncConfig.RepoURL, 300, 70},
		{"main", syncConfig.Branch, 80, 30},
		{"~/.config/sshm/sync-repo", syncConfig.LocalPath, 300, 70},
		{"24h", syncConfig.AutoSyncTTL, 40, 20},
		{"~/.ssh/ssh-key", syncConfig.PublicKeyDir, 300, 70},
		{"", syncConfig.CommitAuthorName, 120, 50},
		{"", syncConfig.CommitAuthorEmail, 160, 50},
	}

	for i, field := range fields {
		inputs[i] = textinput.New()
		inputs[i].Placeholder = field.placeholder
		inputs[i].CharLimit = field.limit
		inputs[i].Width = field.width
		inputs[i].SetValue(field.value)
	}
	inputs[0].Focus()
	return inputs
}

func (m *syncMenuModel) updateSettingsFocus() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.inputs {
		if i == m.focused {
			cmds = append(cmds, m.inputs[i].Focus())
		} else {
			m.inputs[i].Blur()
		}
	}
	return tea.Batch(cmds...)
}

func (m *syncMenuModel) saveSettings() tea.Cmd {
	return func() tea.Msg {
		syncConfig := m.syncConfig
		syncConfig.RepoURL = strings.TrimSpace(m.inputs[syncRepoInput].Value())
		syncConfig.Branch = strings.TrimSpace(m.inputs[syncBranchInput].Value())
		syncConfig.LocalPath = strings.TrimSpace(m.inputs[syncLocalPathInput].Value())
		ttl, err := config.ValidateAutoSyncTTL(m.inputs[syncAutoSyncTTLInput].Value())
		if err != nil {
			return syncSettingsSaveMsg{err: err}
		}
		syncConfig.AutoSyncTTL = ttl
		syncConfig.PublicKeyDir = strings.TrimSpace(m.inputs[syncPublicKeyDirInput].Value())
		syncConfig.CommitAuthorName = strings.TrimSpace(m.inputs[syncAuthorNameInput].Value())
		syncConfig.CommitAuthorEmail = strings.TrimSpace(m.inputs[syncAuthorEmailInput].Value())
		if syncConfig.Branch == "" {
			syncConfig.Branch = "main"
		}
		defaults := config.GetDefaultSyncConfig()
		if syncConfig.LocalPath == "" {
			syncConfig.LocalPath = defaults.LocalPath
		}
		if syncConfig.PublicKeyDir == "" {
			syncConfig.PublicKeyDir = defaults.PublicKeyDir
		}
		if syncConfig.RepoURL != "" {
			syncConfig.Enabled = true
		}
		return syncSettingsSaveMsg{syncConfig: syncConfig}
	}
}

func (m *syncMenuModel) SetStatus(result string, err string) {
	m.status = result
	m.err = err
	m.running = false
}

func (m *syncMenuModel) SetRunning(action string) {
	m.running = true
	m.err = ""
	m.status = fmt.Sprintf("%s is running...", action)
}

func (m *syncMenuModel) SetSyncConfig(syncConfig config.SyncConfig) {
	m.syncConfig = syncConfig
}

func (m *syncMenuModel) View() string {
	if m.mode == syncMenuSettings {
		return m.renderSettings()
	}
	return m.renderActions()
}

func (m *syncMenuModel) renderActions() string {
	var b strings.Builder
	b.WriteString(m.styles.FormTitle.Render("SSHM Sync Center"))
	b.WriteString("\n\n")
	b.WriteString(m.renderSyncSummary())
	b.WriteString("\n")

	items := m.menuItems()
	for i, item := range items {
		label := item.label
		if m.running && item.action != syncActionClose {
			label += " (disabled while running)"
		}
		if i == m.selected {
			b.WriteString(m.styles.Selected.Render("▶ " + label))
		} else {
			b.WriteString("  " + label)
		}
		b.WriteString("\n")
	}

	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(m.styles.HelpText.Render(m.status))
		b.WriteString("\n")
	}
	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(m.styles.ErrorText.Render(m.err))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.styles.FormHelp.Render("↑/↓: navigate • Enter: select • Esc/q: close"))
	return m.center(b.String())
}

func (m *syncMenuModel) renderSettings() string {
	var b strings.Builder
	b.WriteString(m.styles.FormTitle.Render("Sync Settings"))
	b.WriteString("\n\n")

	fields := []struct {
		index int
		label string
	}{
		{syncRepoInput, "Private repo URL"},
		{syncBranchInput, "Branch"},
		{syncLocalPathInput, "Local repo path"},
		{syncAutoSyncTTLInput, "Auto-sync TTL"},
		{syncPublicKeyDirInput, "Public key directory"},
		{syncAuthorNameInput, "Commit author name"},
		{syncAuthorEmailInput, "Commit author email"},
	}

	for _, field := range fields {
		style := m.styles.FormField
		if m.focused == field.index {
			style = m.styles.FocusedLabel
		}
		b.WriteString(style.Render(field.label))
		b.WriteString("\n")
		b.WriteString(m.inputs[field.index].View())
		b.WriteString("\n\n")
	}

	b.WriteString(m.styles.FormHelp.Render("Ctrl+S: save • Esc: back • Tab/↑/↓: navigate"))
	return m.center(b.String())
}

func (m *syncMenuModel) renderSyncSummary() string {
	lines := []string{
		fmt.Sprintf("Enabled: %s", yesNo(m.syncConfig.Enabled)),
		fmt.Sprintf("Repo: %s", emptyDash(m.syncConfig.RepoURL)),
		fmt.Sprintf("Branch: %s", emptyDash(m.syncConfig.Branch)),
		fmt.Sprintf("Local path: %s", emptyDash(m.syncConfig.LocalPath)),
		fmt.Sprintf("Auto startup: %s", yesNo(m.syncConfig.AutoSyncOnStartup)),
		fmt.Sprintf("Auto-sync TTL: %s", emptyDash(m.syncConfig.AutoSyncTTL)),
		fmt.Sprintf("Sync public keys: %s", yesNo(m.syncConfig.ShouldSyncPublicKeys())),
	}
	if m.syncConfig.LastSyncAt != "" {
		lines = append(lines, "Last sync: "+m.syncConfig.LastSyncAt)
	}
	if m.syncConfig.LastSyncStatus != "" {
		lines = append(lines, "Last status: "+m.syncConfig.LastSyncStatus)
	}
	return m.styles.HelpText.Render(strings.Join(lines, "\n"))
}

func (m *syncMenuModel) center(content string) string {
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		m.styles.FormContainer.Render(content),
	)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
