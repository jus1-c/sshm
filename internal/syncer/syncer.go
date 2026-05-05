package syncer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Gu1llaum-3/sshm/internal/config"
)

// Action identifies a sync operation.
type Action string

const (
	ActionSync  Action = "sync"
	ActionPull  Action = "pull"
	ActionPush  Action = "push"
	ActionCheck Action = "check"
)

// Check reports one repository availability check.
type Check struct {
	Name   string
	Status string
	Detail string
}

// Result reports the outcome of a sync operation.
type Result struct {
	Action  Action
	OK      bool
	Summary string
	Details []string
	Checks  []Check
}

// Manager runs git-backed synchronization for SSHM data.
type Manager struct {
	SyncConfig    config.SyncConfig
	SSHConfigPath string
}

// New creates a sync manager.
func New(syncConfig config.SyncConfig, sshConfigPath string) Manager {
	return Manager{SyncConfig: syncConfig, SSHConfigPath: sshConfigPath}
}

// Run executes one sync action.
func (m Manager) Run(ctx context.Context, action Action) Result {
	switch action {
	case ActionCheck:
		return m.CheckAvailability(ctx)
	case ActionPull:
		return m.Pull(ctx)
	case ActionPush:
		return m.Push(ctx)
	default:
		return m.Sync(ctx)
	}
}

// CheckAvailability verifies that git and the private repo are usable.
func (m Manager) CheckAvailability(ctx context.Context) Result {
	result := Result{Action: ActionCheck, OK: true, Summary: "repository is available"}

	if _, err := exec.LookPath("git"); err != nil {
		result.OK = false
		result.Checks = append(result.Checks, Check{"git executable", "error", "git was not found in PATH"})
		result.Summary = "git is not available"
		return result
	}
	result.Checks = append(result.Checks, Check{"git executable", "ok", "git found in PATH"})

	repoURL := strings.TrimSpace(m.SyncConfig.RepoURL)
	if repoURL == "" {
		result.OK = false
		result.Checks = append(result.Checks, Check{"repo url", "error", "sync.repo_url is empty"})
		result.Summary = "repo URL is not configured"
		return result
	}
	result.Checks = append(result.Checks, Check{"repo url", "ok", repoURL})

	branch := m.branch()
	output, err := runCommand(ctx, "", nil, "git", "ls-remote", "--heads", repoURL, branch)
	if err != nil {
		result.OK = false
		result.Checks = append(result.Checks, Check{"remote access", "error", outputOrError(output, err)})
		result.Summary = "repo is not reachable"
		return result
	}
	if strings.TrimSpace(output) == "" {
		result.OK = false
		result.Checks = append(result.Checks, Check{"branch", "error", fmt.Sprintf("branch %q was not found", branch)})
		result.Summary = "configured branch was not found"
		return result
	}
	result.Checks = append(result.Checks, Check{"remote access", "ok", fmt.Sprintf("%s:%s is reachable", repoURL, branch)})

	localPath, err := m.localPath()
	if err != nil {
		result.OK = false
		result.Checks = append(result.Checks, Check{"local path", "error", err.Error()})
		result.Summary = "local sync path is invalid"
		return result
	}

	if _, err := os.Stat(localPath); errors.Is(err, os.ErrNotExist) {
		result.Checks = append(result.Checks, Check{"local clone", "warn", "not cloned yet"})
		return result
	}
	if _, err := os.Stat(filepath.Join(localPath, ".git")); err != nil {
		result.OK = false
		result.Checks = append(result.Checks, Check{"local clone", "error", "local path exists but is not a git repo"})
		result.Summary = "local sync path is not a git repo"
		return result
	}

	remoteURL, err := runGit(ctx, localPath, nil, "remote", "get-url", "origin")
	if err != nil {
		result.OK = false
		result.Checks = append(result.Checks, Check{"local remote", "error", outputOrError(remoteURL, err)})
		result.Summary = "local repo remote could not be read"
		return result
	}
	if strings.TrimSpace(remoteURL) != repoURL {
		result.OK = false
		result.Checks = append(result.Checks, Check{"local remote", "error", fmt.Sprintf("origin is %q, expected %q", strings.TrimSpace(remoteURL), repoURL)})
		result.Summary = "local repo remote does not match sync config"
		return result
	}
	result.Checks = append(result.Checks, Check{"local remote", "ok", strings.TrimSpace(remoteURL)})

	status, err := runGit(ctx, localPath, nil, "status", "--porcelain")
	if err != nil {
		result.OK = false
		result.Checks = append(result.Checks, Check{"working tree", "error", outputOrError(status, err)})
		result.Summary = "local repo status could not be checked"
		return result
	}
	if strings.TrimSpace(status) != "" {
		result.Checks = append(result.Checks, Check{"working tree", "warn", "local sync repo has uncommitted changes"})
	} else {
		result.Checks = append(result.Checks, Check{"working tree", "ok", "clean"})
	}

	return result
}

// Pull updates local SSHM files from the private repo.
func (m Manager) Pull(ctx context.Context) Result {
	result := Result{Action: ActionPull, OK: true}
	repoPath, err := m.ensureRepo(ctx, &result)
	if err != nil {
		return failResult(result, err)
	}

	if err := m.pullRepo(ctx, repoPath, &result); err != nil {
		return failResult(result, err)
	}
	if err := m.importFromRepo(repoPath, &result); err != nil {
		return failResult(result, err)
	}

	result.Summary = "pulled sync repo into local SSH files"
	return result
}

// Push exports local SSHM files and pushes them to the private repo.
func (m Manager) Push(ctx context.Context) Result {
	result := Result{Action: ActionPush, OK: true}
	repoPath, err := m.ensureRepo(ctx, &result)
	if err != nil {
		return failResult(result, err)
	}

	if err := m.exportToRepo(repoPath, &result); err != nil {
		return failResult(result, err)
	}
	changed, err := m.commitAndPush(ctx, repoPath, &result)
	if err != nil {
		return failResult(result, err)
	}
	if changed {
		result.Summary = "pushed local SSH files to sync repo"
	} else {
		result.Summary = "sync repo already matches local SSH files"
	}
	return result
}

// Sync pulls remote files, imports them locally, then pushes local state back.
func (m Manager) Sync(ctx context.Context) Result {
	result := Result{Action: ActionSync, OK: true}
	repoPath, err := m.ensureRepo(ctx, &result)
	if err != nil {
		return failResult(result, err)
	}

	if err := m.pullRepo(ctx, repoPath, &result); err != nil {
		return failResult(result, err)
	}
	if err := m.importFromRepo(repoPath, &result); err != nil {
		return failResult(result, err)
	}
	if err := m.exportToRepo(repoPath, &result); err != nil {
		return failResult(result, err)
	}
	changed, err := m.commitAndPush(ctx, repoPath, &result)
	if err != nil {
		return failResult(result, err)
	}
	if changed {
		result.Summary = "synced local SSH files with private repo"
	} else {
		result.Summary = "local SSH files and sync repo are already aligned"
	}
	return result
}

func (m Manager) ensureRepo(ctx context.Context, result *Result) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git was not found in PATH")
	}

	repoURL := strings.TrimSpace(m.SyncConfig.RepoURL)
	if repoURL == "" {
		return "", fmt.Errorf("sync repo URL is not configured")
	}

	localPath, err := m.localPath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(filepath.Join(localPath, ".git")); err == nil {
		remoteURL, err := runGit(ctx, localPath, nil, "remote", "get-url", "origin")
		if err != nil {
			return "", fmt.Errorf("failed to read local sync repo remote: %s", outputOrError(remoteURL, err))
		}
		if strings.TrimSpace(remoteURL) != repoURL {
			return "", fmt.Errorf("local sync repo origin is %q, expected %q", strings.TrimSpace(remoteURL), repoURL)
		}
		result.Details = append(result.Details, "using existing local sync repo")
		return localPath, nil
	}

	if info, err := os.Stat(localPath); err == nil && info.IsDir() {
		empty, err := isDirEmpty(localPath)
		if err != nil {
			return "", err
		}
		if !empty {
			return "", fmt.Errorf("local sync path exists but is not a git repo: %s", localPath)
		}
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return "", err
	}

	output, err := runCommand(ctx, "", nil, "git", "clone", "--branch", m.branch(), repoURL, localPath)
	if err != nil {
		return "", fmt.Errorf("failed to clone sync repo: %s", outputOrError(output, err))
	}
	result.Details = append(result.Details, "cloned sync repo")
	return localPath, nil
}

func (m Manager) pullRepo(ctx context.Context, repoPath string, result *Result) error {
	branch := m.branch()
	if output, err := runGit(ctx, repoPath, nil, "fetch", "origin", branch); err != nil {
		return fmt.Errorf("failed to fetch sync repo: %s", outputOrError(output, err))
	}
	if output, err := runGit(ctx, repoPath, nil, "pull", "--ff-only", "origin", branch); err != nil {
		return fmt.Errorf("failed to pull sync repo: %s", outputOrError(output, err))
	}
	result.Details = append(result.Details, "pulled latest repo state")
	return nil
}

func (m Manager) exportToRepo(repoPath string, result *Result) error {
	repoSSHDir := filepath.Join(repoPath, "ssh")
	if err := os.MkdirAll(repoSSHDir, 0755); err != nil {
		return err
	}

	sshConfigPath, err := m.sshConfigPath()
	if err != nil {
		return err
	}

	if m.SyncConfig.ShouldSyncSSHConfig() {
		if err := copyFileIfExists(sshConfigPath, filepath.Join(repoSSHDir, "config"), 0644, false); err != nil {
			return err
		}
		result.Details = append(result.Details, "exported SSH config")
	}

	if m.SyncConfig.ShouldSyncSSHConfig() && m.SyncConfig.ShouldSyncIncludedConfigs() {
		if err := m.exportIncludedConfigs(repoSSHDir, sshConfigPath, result); err != nil {
			return err
		}
	}

	if m.SyncConfig.ShouldSyncPublicKeys() {
		if err := m.exportPublicKeys(repoSSHDir, result); err != nil {
			return err
		}
	}

	metadata := map[string]string{
		"managed_by":       "sshm",
		"last_exported_at": time.Now().Format(time.RFC3339),
		"ssh_config_path":  sshConfigPath,
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(repoPath, "metadata.json"), data, 0644)
}

func (m Manager) exportIncludedConfigs(repoSSHDir, sshConfigPath string, result *Result) error {
	includedDir := filepath.Join(repoSSHDir, "includes")
	if err := os.RemoveAll(includedDir); err != nil {
		return err
	}

	files, err := config.GetAllConfigFilesFromBase(sshConfigPath)
	if err != nil {
		return err
	}
	sshDir, err := config.GetSSHDirectory()
	if err != nil {
		return err
	}
	sshDir, err = filepath.Abs(sshDir)
	if err != nil {
		return err
	}
	mainConfigAbs, err := filepath.Abs(sshConfigPath)
	if err != nil {
		return err
	}

	copied := 0
	for _, file := range files {
		absFile, err := filepath.Abs(file)
		if err != nil || absFile == mainConfigAbs {
			continue
		}
		if !isWithinDir(absFile, sshDir) {
			result.Details = append(result.Details, fmt.Sprintf("skipped include outside .ssh: %s", file))
			continue
		}
		rel, err := filepath.Rel(sshDir, absFile)
		if err != nil {
			continue
		}
		if err := copyFileIfExists(absFile, filepath.Join(includedDir, rel), 0644, false); err != nil {
			return err
		}
		copied++
	}
	if copied > 0 {
		result.Details = append(result.Details, fmt.Sprintf("exported %d included config file(s)", copied))
	}
	return nil
}

func (m Manager) exportPublicKeys(repoSSHDir string, result *Result) error {
	keyDir, err := expandPath(m.SyncConfig.PublicKeyDir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(keyDir); errors.Is(err, os.ErrNotExist) {
		result.Details = append(result.Details, "public key directory does not exist; skipped keys")
		return nil
	}

	repoKeyDir := filepath.Join(repoSSHDir, "ssh-key")
	if err := os.RemoveAll(repoKeyDir); err != nil {
		return err
	}

	copied := 0
	if err := filepath.WalkDir(keyDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".pub") {
			return nil
		}

		rel, err := filepath.Rel(keyDir, path)
		if err != nil {
			return err
		}
		if err := copyFileIfExists(path, filepath.Join(repoKeyDir, rel), 0644, false); err != nil {
			return err
		}
		copied++
		return nil
	}); err != nil {
		return err
	}
	if copied > 0 {
		result.Details = append(result.Details, fmt.Sprintf("exported %d public key file(s)", copied))
	}
	return nil
}

func (m Manager) importFromRepo(repoPath string, result *Result) error {
	repoSSHDir := filepath.Join(repoPath, "ssh")
	if _, err := os.Stat(repoSSHDir); errors.Is(err, os.ErrNotExist) {
		result.Details = append(result.Details, "repo has no ssh directory yet")
		return nil
	}

	sshConfigPath, err := m.sshConfigPath()
	if err != nil {
		return err
	}

	if m.SyncConfig.ShouldSyncSSHConfig() {
		if err := copyFileIfExists(filepath.Join(repoSSHDir, "config"), sshConfigPath, 0600, true); err != nil {
			return err
		}
		result.Details = append(result.Details, "imported SSH config")
	}

	if m.SyncConfig.ShouldSyncSSHConfig() && m.SyncConfig.ShouldSyncIncludedConfigs() {
		if err := m.importIncludedConfigs(repoSSHDir, result); err != nil {
			return err
		}
	}

	if m.SyncConfig.ShouldSyncPublicKeys() {
		if err := m.importPublicKeys(repoSSHDir, result); err != nil {
			return err
		}
	}

	return nil
}

func (m Manager) importIncludedConfigs(repoSSHDir string, result *Result) error {
	sourceDir := filepath.Join(repoSSHDir, "includes")
	if _, err := os.Stat(sourceDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	sshDir, err := config.GetSSHDirectory()
	if err != nil {
		return err
	}

	copied := 0
	if err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if err := copyFileIfExists(path, filepath.Join(sshDir, rel), 0600, true); err != nil {
			return err
		}
		copied++
		return nil
	}); err != nil {
		return err
	}
	if copied > 0 {
		result.Details = append(result.Details, fmt.Sprintf("imported %d included config file(s)", copied))
	}
	return nil
}

func (m Manager) importPublicKeys(repoSSHDir string, result *Result) error {
	sourceDir := filepath.Join(repoSSHDir, "ssh-key")
	if _, err := os.Stat(sourceDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	destDir, err := expandPath(m.SyncConfig.PublicKeyDir)
	if err != nil {
		return err
	}

	copied := 0
	if err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if err := copyFileIfExists(path, filepath.Join(destDir, rel), 0644, true); err != nil {
			return err
		}
		copied++
		return nil
	}); err != nil {
		return err
	}
	if copied > 0 {
		result.Details = append(result.Details, fmt.Sprintf("imported %d public key file(s)", copied))
	}
	return nil
}

func (m Manager) commitAndPush(ctx context.Context, repoPath string, result *Result) (bool, error) {
	status, err := runGit(ctx, repoPath, nil, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("failed to inspect sync repo status: %s", outputOrError(status, err))
	}
	if strings.TrimSpace(status) == "" {
		return false, nil
	}

	if output, err := runGit(ctx, repoPath, nil, "add", "ssh", "metadata.json"); err != nil {
		return false, fmt.Errorf("failed to stage sync files: %s", outputOrError(output, err))
	}
	if output, err := runGit(ctx, repoPath, m.gitEnv(), "commit", "-m", "Sync SSHM configuration"); err != nil {
		return false, fmt.Errorf("failed to commit sync files: %s", outputOrError(output, err))
	}
	if output, err := runGit(ctx, repoPath, nil, "push", "origin", m.branch()); err != nil {
		return false, fmt.Errorf("failed to push sync repo: %s", outputOrError(output, err))
	}
	result.Details = append(result.Details, "committed and pushed sync changes")
	return true, nil
}

func (m Manager) branch() string {
	if strings.TrimSpace(m.SyncConfig.Branch) == "" {
		return "main"
	}
	return strings.TrimSpace(m.SyncConfig.Branch)
}

func (m Manager) localPath() (string, error) {
	return expandPath(m.SyncConfig.LocalPath)
}

func (m Manager) sshConfigPath() (string, error) {
	if strings.TrimSpace(m.SSHConfigPath) != "" {
		return expandPath(m.SSHConfigPath)
	}
	return config.GetDefaultSSHConfigPath()
}

func (m Manager) gitEnv() []string {
	var env []string
	if m.SyncConfig.CommitAuthorName != "" {
		env = append(env, "GIT_AUTHOR_NAME="+m.SyncConfig.CommitAuthorName, "GIT_COMMITTER_NAME="+m.SyncConfig.CommitAuthorName)
	}
	if m.SyncConfig.CommitAuthorEmail != "" {
		env = append(env, "GIT_AUTHOR_EMAIL="+m.SyncConfig.CommitAuthorEmail, "GIT_COMMITTER_EMAIL="+m.SyncConfig.CommitAuthorEmail)
	}
	return env
}

func runGit(ctx context.Context, dir string, extraEnv []string, args ...string) (string, error) {
	return runCommand(ctx, dir, extraEnv, "git", args...)
}

func runCommand(ctx context.Context, dir string, extraEnv []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if name == "git" {
		cmd.Env = gitCommandEnv(extraEnv)
	} else if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func gitCommandEnv(extraEnv []string) []string {
	env := os.Environ()
	if !envHasKey(env, "GIT_TERMINAL_PROMPT") && !envHasKey(extraEnv, "GIT_TERMINAL_PROMPT") {
		env = append(env, "GIT_TERMINAL_PROMPT=0")
	}
	if !envHasKey(env, "GIT_SSH_COMMAND") && !envHasKey(extraEnv, "GIT_SSH_COMMAND") {
		env = append(env, "GIT_SSH_COMMAND="+defaultGitSSHCommand())
	}
	return append(env, extraEnv...)
}

func defaultGitSSHCommand() string {
	sshPath := "ssh"
	if windowsSSHPath := windowsOpenSSHPath(); windowsSSHPath != "" {
		sshPath = windowsSSHPath
	}
	return sshPath + " -o BatchMode=yes -o StrictHostKeyChecking=accept-new"
}

func windowsOpenSSHPath() string {
	if runtime.GOOS != "windows" {
		return ""
	}

	windowsDir := os.Getenv("WINDIR")
	if windowsDir == "" {
		windowsDir = os.Getenv("SystemRoot")
	}
	if windowsDir == "" {
		return ""
	}

	sshPath := filepath.Join(windowsDir, "System32", "OpenSSH", "ssh.exe")
	if _, err := os.Stat(sshPath); err != nil {
		return ""
	}
	return filepath.ToSlash(sshPath)
}

func envHasKey(env []string, key string) bool {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func outputOrError(output string, err error) string {
	if strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output)
	}
	if err != nil {
		return err.Error()
	}
	return "unknown error"
}

func failResult(result Result, err error) Result {
	result.OK = false
	result.Summary = err.Error()
	return result
}

func expandPath(path string) (string, error) {
	path = strings.TrimSpace(os.ExpandEnv(path))
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return homeDir, nil
		}
		path = filepath.Join(homeDir, path[2:])
	}
	return filepath.Clean(path), nil
}

func isDirEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func isWithinDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func copyFileIfExists(src, dst string, mode fs.FileMode, backup bool) error {
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return copyFile(src, dst, mode, backup)
}

func copyFile(src, dst string, mode fs.FileMode, backup bool) error {
	equal, err := filesEqual(src, dst)
	if err != nil {
		return err
	}
	if equal {
		return nil
	}

	if backup {
		if err := backupFile(dst); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

func filesEqual(a, b string) (bool, error) {
	aBytes, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	bBytes, err := os.ReadFile(b)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return string(aBytes) == string(bBytes), nil
}

func backupFile(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	backupDir, err := config.GetSSHMBackupDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	hash := sha256.Sum256([]byte(path))
	suffix := hex.EncodeToString(hash[:])[:8]
	backupName := fmt.Sprintf("%s.%s.%s.sync.backup", filepath.Base(path), time.Now().Format("20060102150405"), suffix)
	return copyFile(path, filepath.Join(backupDir, backupName), 0600, false)
}
