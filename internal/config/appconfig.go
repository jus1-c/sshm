package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultAutoSyncTTL = "24h"

// KeyBindings represents configurable key bindings for the application
type KeyBindings struct {
	// Quit keys - keys that will quit the application
	QuitKeys []string `json:"quit_keys"`

	// DisableEscQuit - if true, ESC key won't quit the application (useful for vim users)
	DisableEscQuit bool `json:"disable_esc_quit"`
}

// SyncConfig represents private repository synchronization settings.
type SyncConfig struct {
	Enabled             bool   `json:"enabled"`
	RepoURL             string `json:"repo_url"`
	Branch              string `json:"branch"`
	LocalPath           string `json:"local_path"`
	AutoSyncOnStartup   bool   `json:"auto_sync_on_startup"`
	AutoSyncTTL         string `json:"auto_sync_ttl"`
	SyncSSHConfig       *bool  `json:"sync_ssh_config,omitempty"`
	SyncIncludedConfigs *bool  `json:"sync_included_configs,omitempty"`
	SyncPublicKeys      *bool  `json:"sync_public_keys,omitempty"`
	PublicKeyDir        string `json:"public_key_dir"`
	CommitAuthorName    string `json:"commit_author_name,omitempty"`
	CommitAuthorEmail   string `json:"commit_author_email,omitempty"`
	LastSyncAt          string `json:"last_sync_at,omitempty"`
	LastSyncStatus      string `json:"last_sync_status,omitempty"`
	LastSyncError       string `json:"last_sync_error,omitempty"`
}

// AppConfig represents the main application configuration
type AppConfig struct {
	CheckForUpdates *bool       `json:"check_for_updates,omitempty"`
	KeyBindings     KeyBindings `json:"key_bindings"`
	Sync            SyncConfig  `json:"sync"`
}

// IsUpdateCheckEnabled returns true if the update check is enabled (default: true)
func (c *AppConfig) IsUpdateCheckEnabled() bool {
	if c == nil || c.CheckForUpdates == nil {
		return true
	}
	return *c.CheckForUpdates
}

// GetDefaultKeyBindings returns the default key bindings configuration
func GetDefaultKeyBindings() KeyBindings {
	return KeyBindings{
		QuitKeys:       []string{"q", "ctrl+c"}, // Default keeps current behavior minus ESC
		DisableEscQuit: false,                   // Default to false for backward compatibility
	}
}

func syncDefaultBool(value bool) *bool {
	return &value
}

// GetDefaultSyncConfig returns the default sync configuration.
func GetDefaultSyncConfig() SyncConfig {
	configDir, err := GetSSHMConfigDir()
	if err != nil {
		configDir = filepath.Join("~", ".config", "sshm")
	}

	publicKeyDir, err := GetDefaultPublicKeyDir()
	if err != nil {
		publicKeyDir = filepath.Join("~", ".ssh", "ssh-key")
	}

	return SyncConfig{
		Branch:              "main",
		LocalPath:           filepath.Join(configDir, "sync-repo"),
		AutoSyncTTL:         DefaultAutoSyncTTL,
		SyncSSHConfig:       syncDefaultBool(true),
		SyncIncludedConfigs: syncDefaultBool(true),
		SyncPublicKeys:      syncDefaultBool(true),
		PublicKeyDir:        publicKeyDir,
	}
}

// GetDefaultAppConfig returns the default application configuration
func GetDefaultAppConfig() AppConfig {
	return AppConfig{
		KeyBindings: GetDefaultKeyBindings(),
		Sync:        GetDefaultSyncConfig(),
	}
}

// ShouldSyncSSHConfig returns true when SSH config files should be synced.
func (c SyncConfig) ShouldSyncSSHConfig() bool {
	return c.SyncSSHConfig == nil || *c.SyncSSHConfig
}

// ShouldSyncIncludedConfigs returns true when included SSH config files should be synced.
func (c SyncConfig) ShouldSyncIncludedConfigs() bool {
	return c.SyncIncludedConfigs == nil || *c.SyncIncludedConfigs
}

// ShouldSyncPublicKeys returns true when public keys should be synced.
func (c SyncConfig) ShouldSyncPublicKeys() bool {
	return c.SyncPublicKeys == nil || *c.SyncPublicKeys
}

// ValidateAutoSyncTTL returns a normalized positive Go duration string for auto-sync TTL.
func ValidateAutoSyncTTL(value string) (string, error) {
	ttl := strings.TrimSpace(value)
	if ttl == "" {
		return DefaultAutoSyncTTL, nil
	}
	duration, err := time.ParseDuration(ttl)
	if err != nil || duration <= 0 {
		return "", errors.New("auto-sync TTL must be a positive duration, for example 24h")
	}
	return ttl, nil
}

// AutoSyncTTLDuration returns the configured auto-sync TTL or the default if invalid.
func (c SyncConfig) AutoSyncTTLDuration() time.Duration {
	ttl, err := ValidateAutoSyncTTL(c.AutoSyncTTL)
	if err != nil {
		ttl = DefaultAutoSyncTTL
	}
	duration, _ := time.ParseDuration(ttl)
	return duration
}

// ShouldAutoSync reports whether startup auto-sync should run at the given time.
func (c SyncConfig) ShouldAutoSync(now time.Time) bool {
	if !c.Enabled || !c.AutoSyncOnStartup {
		return false
	}

	lastSync := strings.TrimSpace(c.LastSyncAt)
	if lastSync == "" {
		return true
	}

	lastSyncAt, err := time.Parse(time.RFC3339, lastSync)
	if err != nil {
		return true
	}
	if now.Before(lastSyncAt) {
		return false
	}

	return now.Sub(lastSyncAt) >= c.AutoSyncTTLDuration()
}

// SetSyncSSHConfig updates the SSH config sync option.
func (c *SyncConfig) SetSyncSSHConfig(value bool) {
	c.SyncSSHConfig = syncDefaultBool(value)
}

// SetSyncIncludedConfigs updates the included config sync option.
func (c *SyncConfig) SetSyncIncludedConfigs(value bool) {
	c.SyncIncludedConfigs = syncDefaultBool(value)
}

// SetSyncPublicKeys updates the public key sync option.
func (c *SyncConfig) SetSyncPublicKeys(value bool) {
	c.SyncPublicKeys = syncDefaultBool(value)
}

// GetAppConfigPath returns the path to the application config file
func GetAppConfigPath() (string, error) {
	configDir, err := GetSSHMConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "config.json"), nil
}

// LoadAppConfig loads the application configuration from file
// If the file doesn't exist, it returns the default configuration
func LoadAppConfig() (*AppConfig, error) {
	configPath, err := GetAppConfigPath()
	if err != nil {
		return nil, err
	}

	// If config file doesn't exist, return default config and create the file
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := GetDefaultAppConfig()

		// Create config directory if it doesn't exist
		configDir := filepath.Dir(configPath)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return nil, err
		}

		// Save default config to file
		if err := SaveAppConfig(&defaultConfig); err != nil {
			// If we can't save, just return the default config without erroring
			// This allows the app to work even if config file can't be created
			return &defaultConfig, nil
		}

		return &defaultConfig, nil
	}

	// Read existing config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Validate and fill in missing fields with defaults
	config = mergeWithDefaults(config)

	return &config, nil
}

// SaveAppConfig saves the application configuration to file
func SaveAppConfig(config *AppConfig) error {
	if config == nil {
		return errors.New("config cannot be nil")
	}

	configPath, err := GetAppConfigPath()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// mergeWithDefaults ensures all required fields are set with defaults if missing
func mergeWithDefaults(config AppConfig) AppConfig {
	defaults := GetDefaultAppConfig()

	// If QuitKeys is empty, use defaults
	if len(config.KeyBindings.QuitKeys) == 0 {
		config.KeyBindings.QuitKeys = defaults.KeyBindings.QuitKeys
	}

	if config.Sync.Branch == "" {
		config.Sync.Branch = defaults.Sync.Branch
	}
	if config.Sync.LocalPath == "" {
		config.Sync.LocalPath = defaults.Sync.LocalPath
	}
	if config.Sync.PublicKeyDir == "" {
		config.Sync.PublicKeyDir = defaults.Sync.PublicKeyDir
	}
	if ttl, err := ValidateAutoSyncTTL(config.Sync.AutoSyncTTL); err == nil {
		config.Sync.AutoSyncTTL = ttl
	} else {
		config.Sync.AutoSyncTTL = defaults.Sync.AutoSyncTTL
	}
	if config.Sync.SyncSSHConfig == nil {
		config.Sync.SyncSSHConfig = defaults.Sync.SyncSSHConfig
	}
	if config.Sync.SyncIncludedConfigs == nil {
		config.Sync.SyncIncludedConfigs = defaults.Sync.SyncIncludedConfigs
	}
	if config.Sync.SyncPublicKeys == nil {
		config.Sync.SyncPublicKeys = defaults.Sync.SyncPublicKeys
	}

	return config
}

// ShouldQuitOnKey checks if the given key should trigger quit based on configuration
func (kb *KeyBindings) ShouldQuitOnKey(key string) bool {
	// Special handling for ESC key
	if key == "esc" {
		return !kb.DisableEscQuit
	}

	// Check if key is in the quit keys list
	for _, quitKey := range kb.QuitKeys {
		if quitKey == key {
			return true
		}
	}

	return false
}
