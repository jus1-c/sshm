package cmd

import "testing"

func TestSyncCommand(t *testing.T) {
	if syncCmd.Use != "sync" {
		t.Errorf("Expected Use 'sync', got '%s'", syncCmd.Use)
	}

	if syncCmd.Short != "Sync SSH configuration with a private git repository" {
		t.Errorf("Unexpected sync command short description: %s", syncCmd.Short)
	}

	if err := syncCmd.Args(syncCmd, []string{"extra"}); err == nil {
		t.Error("Expected error for sync command arguments")
	}
}

func TestSyncCommandRegistration(t *testing.T) {
	found := false
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "sync" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Sync command not found in root command")
	}
}
