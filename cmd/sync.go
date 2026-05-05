package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/Gu1llaum-3/sshm/internal/config"
	"github.com/Gu1llaum-3/sshm/internal/syncer"

	"github.com/spf13/cobra"
)

var (
	syncRepoURL       string
	syncBranch        string
	syncLocalPath     string
	syncAutoSyncTTL   string
	syncPublicKeyDir  string
	syncAuthorName    string
	syncAuthorEmail   string
	syncEnable        bool
	syncDisable       bool
	syncAutoStartup   bool
	syncNoAutoStartup bool
	syncCheck         bool
	syncPull          bool
	syncPush          bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync SSH configuration with a private git repository",
	Long:  `Sync SSH configuration and saved public keys with a private git repository.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		appConfig, err := config.LoadAppConfig()
		if err != nil {
			return err
		}

		changed, err := applySyncFlags(cmd, appConfig)
		if err != nil {
			return err
		}
		if changed {
			if err := config.SaveAppConfig(appConfig); err != nil {
				return err
			}
		}

		action := syncer.ActionSync
		switch {
		case syncCheck:
			action = syncer.ActionCheck
		case syncPull:
			action = syncer.ActionPull
		case syncPush:
			action = syncer.ActionPush
		}

		if !appConfig.Sync.Enabled && action != syncer.ActionCheck {
			return fmt.Errorf("sync is disabled; run 'sshm sync --enable --repo <repo-url>' first")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		manager := syncer.New(appConfig.Sync, configFile)
		result := manager.Run(ctx, action)
		recordSyncResult(appConfig, result)
		_ = config.SaveAppConfig(appConfig)
		printSyncResult(cmd, result)

		if !result.OK {
			return fmt.Errorf("%s", result.Summary)
		}
		return nil
	},
}

func applySyncFlags(cmd *cobra.Command, appConfig *config.AppConfig) (bool, error) {
	changed := false
	flags := cmd.Flags()

	if flags.Changed("repo") {
		appConfig.Sync.RepoURL = syncRepoURL
		appConfig.Sync.Enabled = true
		changed = true
	}
	if flags.Changed("branch") {
		appConfig.Sync.Branch = syncBranch
		changed = true
	}
	if flags.Changed("local-path") {
		appConfig.Sync.LocalPath = syncLocalPath
		changed = true
	}
	if flags.Changed("auto-sync-ttl") {
		ttl, err := config.ValidateAutoSyncTTL(syncAutoSyncTTL)
		if err != nil {
			return false, err
		}
		appConfig.Sync.AutoSyncTTL = ttl
		changed = true
	}
	if flags.Changed("public-key-dir") {
		appConfig.Sync.PublicKeyDir = syncPublicKeyDir
		changed = true
	}
	if flags.Changed("author-name") {
		appConfig.Sync.CommitAuthorName = syncAuthorName
		changed = true
	}
	if flags.Changed("author-email") {
		appConfig.Sync.CommitAuthorEmail = syncAuthorEmail
		changed = true
	}
	if flags.Changed("enable") && syncEnable {
		appConfig.Sync.Enabled = true
		changed = true
	}
	if flags.Changed("disable") && syncDisable {
		appConfig.Sync.Enabled = false
		changed = true
	}
	if flags.Changed("auto-startup") {
		appConfig.Sync.AutoSyncOnStartup = syncAutoStartup
		changed = true
	}
	if flags.Changed("no-auto-startup") && syncNoAutoStartup {
		appConfig.Sync.AutoSyncOnStartup = false
		changed = true
	}

	return changed, nil
}

func recordSyncResult(appConfig *config.AppConfig, result syncer.Result) {
	if result.Action != syncer.ActionCheck {
		appConfig.Sync.LastSyncAt = time.Now().Format(time.RFC3339)
	}
	appConfig.Sync.LastSyncStatus = result.Summary
	if result.OK {
		appConfig.Sync.LastSyncError = ""
	} else {
		appConfig.Sync.LastSyncError = result.Summary
	}
}

func printSyncResult(cmd *cobra.Command, result syncer.Result) {
	out := cmd.OutOrStdout()
	status := "OK"
	if !result.OK {
		status = "ERROR"
	}
	fmt.Fprintf(out, "[%s] %s\n", status, result.Summary)
	for _, check := range result.Checks {
		fmt.Fprintf(out, "- %s: %s", check.Name, check.Status)
		if check.Detail != "" {
			fmt.Fprintf(out, " (%s)", check.Detail)
		}
		fmt.Fprintln(out)
	}
	for _, detail := range result.Details {
		fmt.Fprintf(out, "- %s\n", detail)
	}
}

func init() {
	syncCmd.Flags().StringVar(&syncRepoURL, "repo", "", "Private git repository URL (for example git@github.com:user/sshm-sync.git)")
	syncCmd.Flags().StringVar(&syncBranch, "branch", "main", "Sync repository branch")
	syncCmd.Flags().StringVar(&syncLocalPath, "local-path", "", "Local sync repository path")
	syncCmd.Flags().StringVar(&syncAutoSyncTTL, "auto-sync-ttl", config.DefaultAutoSyncTTL, "Minimum time between startup auto-sync runs (for example 24h, 12h, 30m)")
	syncCmd.Flags().StringVar(&syncPublicKeyDir, "public-key-dir", "", "Directory containing saved public keys")
	syncCmd.Flags().StringVar(&syncAuthorName, "author-name", "", "Git commit author name for sync commits")
	syncCmd.Flags().StringVar(&syncAuthorEmail, "author-email", "", "Git commit author email for sync commits")
	syncCmd.Flags().BoolVar(&syncEnable, "enable", false, "Enable sync")
	syncCmd.Flags().BoolVar(&syncDisable, "disable", false, "Disable sync")
	syncCmd.Flags().BoolVar(&syncAutoStartup, "auto-startup", false, "Enable automatic sync on startup")
	syncCmd.Flags().BoolVar(&syncNoAutoStartup, "no-auto-startup", false, "Disable automatic sync on startup")
	syncCmd.Flags().BoolVar(&syncCheck, "check", false, "Check repository availability only")
	syncCmd.Flags().BoolVar(&syncPull, "pull", false, "Pull remote files into local SSH files")
	syncCmd.Flags().BoolVar(&syncPush, "push", false, "Push local SSH files to the sync repo")
	RootCmd.AddCommand(syncCmd)
}
