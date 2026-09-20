package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/jus1-c/sshm/internal/version"

	"github.com/spf13/cobra"
)

var (
	updateCheckOnly bool
	updateVersion   string
	updateForce     bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update sshm to the latest release",
	Long: `Download the latest sshm release for this platform, verify its checksum
and replace the current binary in place.

Examples:
  sshm update                  # Update to the latest stable release
  sshm update --check          # Only check whether an update is available
  sshm update --version v1.12.2 # Install a specific release
  sshm update --force          # Reinstall even if already up to date`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
		defer cancel()

		if updateCheckOnly {
			info, err := version.CheckForUpdates(ctx, AppVersion)
			if err != nil {
				return err
			}
			if info.Available {
				fmt.Printf("Update available: %s -> %s\n%s\n", info.CurrentVer, info.LatestVer, info.ReleaseURL)
			} else {
				fmt.Printf("sshm %s is up to date\n", AppVersion)
			}
			return nil
		}

		result, err := version.SelfUpdate(ctx, AppVersion, updateVersion, updateForce)
		if err != nil {
			return err
		}
		if !result.Updated {
			fmt.Printf("sshm %s is already up to date (latest %s)\n", result.FromVersion, result.ToVersion)
			return nil
		}
		fmt.Printf("Updated sshm %s -> %s\n", result.FromVersion, result.ToVersion)
		fmt.Printf("Installed to %s\n", result.Path)
		return nil
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "Only check for a newer release")
	updateCmd.Flags().StringVar(&updateVersion, "version", "", "Install a specific release tag (for example v1.12.2)")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Reinstall even if the current version is already up to date")
	RootCmd.AddCommand(updateCmd)
}
