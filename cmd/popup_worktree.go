package cmd

import (
	"fmt"

	"github.com/allenan/herd/internal/profile"
	"github.com/allenan/herd/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var popupWorktreeCmd = &cobra.Command{
	Use:    "popup-worktree",
	Short:  "Run the worktree popup (internal)",
	Hidden: true,
	RunE:   runPopupWorktree,
}

func init() {
	popupWorktreeCmd.Flags().String("project", "", "project name")
	popupWorktreeCmd.Flags().String("repo-root", "", "git repository root path")
	popupWorktreeCmd.Flags().String("result-path", "", "path for result file")
	rootCmd.AddCommand(popupWorktreeCmd)
}

func runPopupWorktree(cmd *cobra.Command, args []string) error {
	project, _ := cmd.Flags().GetString("project")
	repoRoot, _ := cmd.Flags().GetString("repo-root")
	resultPath, _ := cmd.Flags().GetString("result-path")

	if repoRoot == "" {
		return fmt.Errorf("--repo-root is required")
	}
	if resultPath == "" {
		prof, err := profile.Resolve(profileName)
		if err != nil {
			return fmt.Errorf("failed to resolve profile: %w", err)
		}
		resultPath = prof.PopupResultPath()
	}

	model := tui.NewWorktreePopupModel(project, repoRoot, resultPath)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("popup error: %w", err)
	}

	return nil
}
