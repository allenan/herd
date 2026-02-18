package cmd

import (
	"fmt"

	"github.com/allenan/herd/internal/profile"
	htmux "github.com/allenan/herd/internal/tmux"
	"github.com/spf13/cobra"
)

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload the sidebar with the current binary on disk",
	RunE: func(cmd *cobra.Command, args []string) error {
		prof, err := profile.Resolve(profileName)
		if err != nil {
			return fmt.Errorf("failed to resolve profile: %w", err)
		}
		htmux.Init(prof)

		paneID, err := htmux.FindSidebarPane()
		if err != nil {
			return fmt.Errorf("failed to find sidebar pane: %w", err)
		}

		if err := htmux.ReloadSidebar(paneID, prof.Name); err != nil {
			return fmt.Errorf("failed to reload sidebar: %w", err)
		}

		fmt.Println("Sidebar reloaded.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(reloadCmd)
}
