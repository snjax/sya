package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var message string
		cmd := app.command("comment <id>", "Append a task comment to Log", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runComment(args[0], message)
		})
		cmd.Flags().StringVarP(&message, "message", "m", "", "comment text")
		return cmd
	})
}

type CommentResult struct {
	ID      string `json:"id"`
	File    string `json:"file"`
	Message string `json:"message"`
}

func (r CommentResult) HumanText(Colorizer) string {
	return fmt.Sprintf("commented %s", r.ID)
}

func (a *App) runComment(id, message string) (CommentResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return CommentResult{}, err
	}
	message = strings.TrimRight(message, "\n")
	if strings.TrimSpace(message) == "" {
		return CommentResult{}, syaerr.Usage{Message: "comment message is required"}
	}
	t, err := state.Index.Resolve(id)
	if err != nil {
		return CommentResult{}, err
	}
	appendLogLine(t, fmt.Sprintf("- %s @%s: %s", a.now().UTC().Format(time.RFC3339), a.Actor(), message))
	if err := writeTask(state.Project.Root, t); err != nil {
		return CommentResult{}, err
	}
	return CommentResult{ID: t.ID, File: t.File, Message: message}, nil
}
