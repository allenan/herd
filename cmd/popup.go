package cmd

import (
	"fmt"

	htmux "github.com/allenan/herd/internal/tmux"
	"github.com/allenan/herd/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var popupNewCmd = &cobra.Command{
	Use:    "popup-new",
	Short:  "Run the new-session popup (internal)",
	Hidden: true,
	RunE:   runPopupNew,
}

func init() {
	popupNewCmd.Flags().String("mode", "new_project", "popup mode: new_project or add_session")
	popupNewCmd.Flags().String("dir", "", "initial directory")
	popupNewCmd.Flags().String("project", "", "project name (for add_session mode)")
	popupNewCmd.Flags().String("result-path", "", "path for result file")
	rootCmd.AddCommand(popupNewCmd)
}

func runPopupNew(cmd *cobra.Command, args []string) error {
	mode, _ := cmd.Flags().GetString("mode")
	dir, _ := cmd.Flags().GetString("dir")
	project, _ := cmd.Flags().GetString("project")
	resultPath, _ := cmd.Flags().GetString("result-path")

	if resultPath == "" {
		resultPath = htmux.PopupResultPath()
	}

	model := tui.NewPopupModel(mode, dir, project, resultPath)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("popup error: %w", err)
	}

	return nil
}
