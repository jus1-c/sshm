package syncer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jus1-c/sshm/internal/config"
)

func boolPtr(v bool) *bool { return &v }

// runGitTest runs git in dir and fails the test on error.
func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// setupSyncTest builds a bare remote seeded with one ssh/config file and returns
// the remote path, the local sync path and the local SSH config path.
func setupSyncTest(t *testing.T, remoteConfig string) (string, string, string) {
	t.Helper()

	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdg"))
	t.Setenv("GIT_AUTHOR_NAME", "sshm-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "sshm-test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "sshm-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "sshm-test@example.com")

	remote := filepath.Join(tmp, "remote.git")
	runGitTest(t, tmp, "init", "--bare", "-b", "main", remote)

	seed := filepath.Join(tmp, "seed")
	runGitTest(t, tmp, "clone", remote, seed)
	if err := os.MkdirAll(filepath.Join(seed, "ssh"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "ssh", "config"), []byte(remoteConfig), 0600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, seed, "add", "ssh")
	runGitTest(t, seed, "commit", "-m", "seed")
	runGitTest(t, seed, "push", "origin", "main")

	return remote, filepath.Join(tmp, "sync-repo"), filepath.Join(home, ".ssh", "config")
}

func testSyncConfig(remote, localPath string) config.SyncConfig {
	return config.SyncConfig{
		Enabled:             true,
		RepoURL:             remote,
		Branch:              "main",
		LocalPath:           localPath,
		SyncSSHConfig:       boolPtr(true),
		SyncIncludedConfigs: boolPtr(false),
		SyncPublicKeys:      boolPtr(false),
		CommitAuthorName:    "sshm-test",
		CommitAuthorEmail:   "sshm-test@example.com",
	}
}

// TestSyncPreservesLocalChanges verifies that a local edit survives a sync and
// reaches the remote even though the remote did not change.
func TestSyncPreservesLocalChanges(t *testing.T) {
	remote, localPath, sshConfigPath := setupSyncTest(t, "Host base\n  HostName 1.1.1.1\n")

	localEdit := "Host base\n  HostName 1.1.1.1\n\nHost local-edit\n  HostName 2.2.2.2\n"
	if err := os.WriteFile(sshConfigPath, []byte(localEdit), 0600); err != nil {
		t.Fatal(err)
	}

	manager := New(testSyncConfig(remote, localPath), sshConfigPath)
	result := manager.Sync(context.Background())
	if !result.OK {
		t.Fatalf("Sync failed: %s", result.Summary)
	}

	got, err := os.ReadFile(sshConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != localEdit {
		t.Fatalf("local SSH config was not preserved:\n%s", got)
	}

	remoteConfig := runGitTest(t, remote, "show", "main:ssh/config")
	if !strings.Contains(remoteConfig, "local-edit") {
		t.Fatalf("local change did not reach the remote:\n%s", remoteConfig)
	}
}

// TestSyncConflictAbortsWithoutTouchingLocal verifies that a conflicting remote
// change aborts the merge and leaves the local SSH config untouched.
func TestSyncConflictAbortsWithoutTouchingLocal(t *testing.T) {
	remote, localPath, sshConfigPath := setupSyncTest(t, "Host base\n  HostName 1.1.1.1\n")

	base := "Host base\n  HostName 1.1.1.1\n"
	if err := os.WriteFile(sshConfigPath, []byte(base), 0600); err != nil {
		t.Fatal(err)
	}

	manager := New(testSyncConfig(remote, localPath), sshConfigPath)
	if result := manager.Sync(context.Background()); !result.OK {
		t.Fatalf("initial Sync failed: %s", result.Summary)
	}

	// Remote side edits the same line.
	other := filepath.Join(t.TempDir(), "other")
	runGitTest(t, filepath.Dir(other), "clone", remote, other)
	if err := os.WriteFile(filepath.Join(other, "ssh", "config"), []byte("Host remote-change\n  HostName 3.3.3.3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, other, "add", "ssh")
	runGitTest(t, other, "commit", "-m", "remote change")
	runGitTest(t, other, "push", "origin", "main")

	// Local side edits the same line differently.
	localEdit := "Host local-change\n  HostName 4.4.4.4\n"
	if err := os.WriteFile(sshConfigPath, []byte(localEdit), 0600); err != nil {
		t.Fatal(err)
	}

	result := manager.Sync(context.Background())
	if result.OK {
		t.Fatal("expected Sync to fail on conflict")
	}
	if !strings.Contains(result.Summary, "conflicting") {
		t.Fatalf("expected a conflict error, got: %s", result.Summary)
	}

	got, err := os.ReadFile(sshConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != localEdit {
		t.Fatalf("local SSH config was modified on conflict:\n%s", got)
	}

	if _, err := os.Stat(filepath.Join(localPath, ".git", "MERGE_HEAD")); err == nil {
		t.Fatal("merge was not aborted: MERGE_HEAD still present")
	}
}
