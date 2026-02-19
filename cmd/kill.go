package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/allenan/herd/internal/profile"
	htmux "github.com/allenan/herd/internal/tmux"
	"github.com/spf13/cobra"
)

var killCmd = &cobra.Command{
	Use:   "kill",
	Short: "Kill the herd tmux server and clean up state for a profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		prof, err := profile.Resolve(profileName)
		if err != nil {
			return fmt.Errorf("failed to resolve profile: %w", err)
		}
		htmux.Init(prof)

		// 1. Kill the tmux server (ignore errors — it may not be running)
		htmux.TmuxRun("kill-server")

		// 2. Kill any lingering sidebar processes
		var pattern string
		if prof.Name != "" {
			pattern = fmt.Sprintf("herd --sidebar --profile %s", prof.Name)
		} else {
			pattern = "herd --sidebar"
		}
		exec.Command("pkill", "-f", pattern).Run() //nolint: ignore exit status

		// 3. Remove the state directory
		if err := os.RemoveAll(prof.BaseDir); err != nil {
			return fmt.Errorf("failed to remove state directory %s: %w", prof.BaseDir, err)
		}

		label := "default"
		if prof.Name != "" {
			label = prof.Name
		}
		fmt.Printf("Killed herd (%s profile). State directory removed: %s\n", label, prof.BaseDir)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(killCmd)
}
