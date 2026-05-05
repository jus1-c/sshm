package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultKeyBindings(t *testing.T) {
	kb := GetDefaultKeyBindings()

	// Test default configuration
	if kb.DisableEscQuit {
		t.Error("Default configuration should allow ESC to quit (backward compatibility)")
	}

	// Test default quit keys
	expectedQuitKeys := []string{"q", "ctrl+c"}
	if len(kb.QuitKeys) != len(expectedQuitKeys) {
		t.Errorf("Expected %d quit keys, got %d", len(expectedQuitKeys), len(kb.QuitKeys))
	}

	for i, expected := range expectedQuitKeys {
		if i >= len(kb.QuitKeys) || kb.QuitKeys[i] != expected {
			t.Errorf("Expected quit key %s, got %s", expected, kb.QuitKeys[i])
		}
	}
}

func TestShouldQuitOnKey(t *testing.T) {
	tests := []struct {
		name           string
		keyBindings    KeyBindings
		key            string
		expectedResult bool
	}{
		{
			name: "Default config - ESC should quit",
			keyBindings: KeyBindings{
				QuitKeys:       []string{"q", "ctrl+c"},
				DisableEscQuit: false,
			},
			key:            "esc",
			expectedResult: true,
		},
		{
			name: "Disabled ESC quit - ESC should not quit",
			keyBindings: KeyBindings{
				QuitKeys:       []string{"q", "ctrl+c"},
				DisableEscQuit: true,
			},
			key:            "esc",
			expectedResult: false,
		},
		{
			name: "Q key should quit",
			keyBindings: KeyBindings{
				QuitKeys:       []string{"q", "ctrl+c"},
				DisableEscQuit: true,
			},
			key:            "q",
			expectedResult: true,
		},
		{
			name: "Ctrl+C should quit",
			keyBindings: KeyBindings{
				QuitKeys:       []string{"q", "ctrl+c"},
				DisableEscQuit: true,
			},
			key:            "ctrl+c",
			expectedResult: true,
		},
		{
			name: "Other keys should not quit",
			keyBindings: KeyBindings{
				QuitKeys:       []string{"q", "ctrl+c"},
				DisableEscQuit: true,
			},
			key:            "enter",
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.keyBindings.ShouldQuitOnKey(tt.key)
			if result != tt.expectedResult {
				t.Errorf("ShouldQuitOnKey(%q) = %v, expected %v", tt.key, result, tt.expectedResult)
			}
		})
	}
}

func TestAppConfigBasics(t *testing.T) {
	// Test default config creation
	defaultConfig := GetDefaultAppConfig()

	if defaultConfig.KeyBindings.DisableEscQuit {
		t.Error("Default configuration should allow ESC to quit")
	}

	expectedQuitKeys := []string{"q", "ctrl+c"}
	if len(defaultConfig.KeyBindings.QuitKeys) != len(expectedQuitKeys) {
		t.Errorf("Expected %d quit keys, got %d", len(expectedQuitKeys), len(defaultConfig.KeyBindings.QuitKeys))
	}

	// CheckForUpdates should be nil by default
	if defaultConfig.CheckForUpdates != nil {
		t.Error("Default configuration should have CheckForUpdates as nil")
	}

	// IsUpdateCheckEnabled should return true by default
	if !defaultConfig.IsUpdateCheckEnabled() {
		t.Error("IsUpdateCheckEnabled should return true when CheckForUpdates is nil")
	}

	if defaultConfig.Sync.Branch != "main" {
		t.Errorf("Expected default sync branch main, got %s", defaultConfig.Sync.Branch)
	}
	if defaultConfig.Sync.AutoSyncTTL != DefaultAutoSyncTTL {
		t.Errorf("Expected default auto sync TTL %s, got %s", DefaultAutoSyncTTL, defaultConfig.Sync.AutoSyncTTL)
	}
	if defaultConfig.Sync.Enabled {
		t.Error("Sync should be disabled by default")
	}
	if !defaultConfig.Sync.ShouldSyncSSHConfig() || !defaultConfig.Sync.ShouldSyncIncludedConfigs() || !defaultConfig.Sync.ShouldSyncPublicKeys() {
		t.Error("Default sync config should include SSH config, included configs, and public keys")
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestIsUpdateCheckEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *AppConfig
		expected bool
	}{
		{
			name:     "nil AppConfig returns true",
			config:   nil,
			expected: true,
		},
		{
			name:     "CheckForUpdates nil returns true",
			config:   &AppConfig{},
			expected: true,
		},
		{
			name:     "CheckForUpdates true returns true",
			config:   &AppConfig{CheckForUpdates: boolPtr(true)},
			expected: true,
		},
		{
			name:     "CheckForUpdates false returns false",
			config:   &AppConfig{CheckForUpdates: boolPtr(false)},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsUpdateCheckEnabled()
			if result != tt.expected {
				t.Errorf("IsUpdateCheckEnabled() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestMergeWithDefaults(t *testing.T) {
	// Test config with missing QuitKeys
	incompleteConfig := AppConfig{
		KeyBindings: KeyBindings{
			DisableEscQuit: true,
			// QuitKeys is missing
		},
	}

	mergedConfig := mergeWithDefaults(incompleteConfig)

	// Should preserve DisableEscQuit
	if !mergedConfig.KeyBindings.DisableEscQuit {
		t.Error("Should preserve DisableEscQuit as true")
	}

	// Should fill in default QuitKeys
	expectedQuitKeys := []string{"q", "ctrl+c"}
	if len(mergedConfig.KeyBindings.QuitKeys) != len(expectedQuitKeys) {
		t.Errorf("Expected %d quit keys, got %d", len(expectedQuitKeys), len(mergedConfig.KeyBindings.QuitKeys))
	}

	if mergedConfig.Sync.Branch != "main" {
		t.Errorf("Expected sync branch default main, got %s", mergedConfig.Sync.Branch)
	}
	if mergedConfig.Sync.LocalPath == "" {
		t.Error("Expected sync local path default to be set")
	}
	if mergedConfig.Sync.AutoSyncTTL != DefaultAutoSyncTTL {
		t.Errorf("Expected sync TTL default %s, got %s", DefaultAutoSyncTTL, mergedConfig.Sync.AutoSyncTTL)
	}

	invalidTTLConfig := mergeWithDefaults(AppConfig{Sync: SyncConfig{AutoSyncTTL: "invalid"}})
	if invalidTTLConfig.Sync.AutoSyncTTL != DefaultAutoSyncTTL {
		t.Errorf("Expected invalid sync TTL to default to %s, got %s", DefaultAutoSyncTTL, invalidTTLConfig.Sync.AutoSyncTTL)
	}

	if !mergedConfig.Sync.ShouldSyncPublicKeys() {
		t.Error("Expected sync public keys default to be enabled")
	}
}

func TestValidateAutoSyncTTL(t *testing.T) {
	if got, err := ValidateAutoSyncTTL(" 12h "); err != nil || got != "12h" {
		t.Fatalf("ValidateAutoSyncTTL() = %q, %v; expected 12h, nil", got, err)
	}
	if got, err := ValidateAutoSyncTTL(""); err != nil || got != DefaultAutoSyncTTL {
		t.Fatalf("ValidateAutoSyncTTL(empty) = %q, %v; expected default", got, err)
	}
	if _, err := ValidateAutoSyncTTL("0s"); err == nil {
		t.Fatal("ValidateAutoSyncTTL(0s) expected error")
	}
	if _, err := ValidateAutoSyncTTL("not-a-duration"); err == nil {
		t.Fatal("ValidateAutoSyncTTL(not-a-duration) expected error")
	}
}

func TestSyncConfigShouldAutoSync(t *testing.T) {
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		config   SyncConfig
		expected bool
	}{
		{
			name:     "disabled sync",
			config:   SyncConfig{Enabled: false, AutoSyncOnStartup: true, AutoSyncTTL: "24h"},
			expected: false,
		},
		{
			name:     "auto startup disabled",
			config:   SyncConfig{Enabled: true, AutoSyncOnStartup: false, AutoSyncTTL: "24h"},
			expected: false,
		},
		{
			name:     "never synced",
			config:   SyncConfig{Enabled: true, AutoSyncOnStartup: true, AutoSyncTTL: "24h"},
			expected: true,
		},
		{
			name:     "within TTL",
			config:   SyncConfig{Enabled: true, AutoSyncOnStartup: true, AutoSyncTTL: "24h", LastSyncAt: now.Add(-23 * time.Hour).Format(time.RFC3339)},
			expected: false,
		},
		{
			name:     "expired TTL",
			config:   SyncConfig{Enabled: true, AutoSyncOnStartup: true, AutoSyncTTL: "24h", LastSyncAt: now.Add(-25 * time.Hour).Format(time.RFC3339)},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.ShouldAutoSync(now); got != tt.expected {
				t.Errorf("ShouldAutoSync() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestSaveAndLoadAppConfigIntegration(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "sshm_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a custom config file directly in temp directory
	configPath := filepath.Join(tempDir, "config.json")

	customConfig := AppConfig{
		CheckForUpdates: boolPtr(false),
		KeyBindings: KeyBindings{
			QuitKeys:       []string{"q"},
			DisableEscQuit: true,
		},
	}

	// Save config directly to file
	data, err := json.MarshalIndent(customConfig, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	err = os.WriteFile(configPath, data, 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Read and unmarshal config
	readData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var loadedConfig AppConfig
	err = json.Unmarshal(readData, &loadedConfig)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Verify the loaded config matches what we saved
	if !loadedConfig.KeyBindings.DisableEscQuit {
		t.Error("DisableEscQuit should be true")
	}

	if len(loadedConfig.KeyBindings.QuitKeys) != 1 || loadedConfig.KeyBindings.QuitKeys[0] != "q" {
		t.Errorf("Expected quit keys to be ['q'], got %v", loadedConfig.KeyBindings.QuitKeys)
	}

	// Verify CheckForUpdates is correctly persisted and reloaded
	if loadedConfig.CheckForUpdates == nil {
		t.Fatal("CheckForUpdates should not be nil after round-trip")
	}
	if *loadedConfig.CheckForUpdates != false {
		t.Errorf("CheckForUpdates should be false after round-trip, got %v", *loadedConfig.CheckForUpdates)
	}
	if loadedConfig.IsUpdateCheckEnabled() {
		t.Error("IsUpdateCheckEnabled should return false when CheckForUpdates is false")
	}
}
