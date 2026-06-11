package cli

import (
	"context"

	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var section string
		var file string
		cmd := app.command("edit <id>", "Edit a task body section", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runEdit(args[0], section, file)
		})
		cmd.Flags().StringVar(&section, "section", "", "section name")
		cmd.Flags().StringVar(&file, "file", "-", "section content file or -")
		return cmd
	})
}

func (a *App) runEdit(id, section, file string) (MutationResult, error) {
	if section == "" {
		return MutationResult{}, syaerr.Usage{Message: "--section is required"}
	}
	state, err := a.loadProject()
	if err != nil {
		return MutationResult{}, err
	}
	t, err := state.Index.Resolve(id)
	if err != nil {
		return MutationResult{}, err
	}
	typeDef := state.Schema.Types[t.Type]
	if !stringIn(typeDef.Sections, section) && !taskHasSection(t, section) {
		return MutationResult{}, syaerr.Usage{Message: "section is not declared for type and does not already exist: " + section}
	}
	data, err := readInputFile(file, a.in)
	if err != nil {
		return MutationResult{}, err
	}
	task.EditSection(t, section, data)
	if err := writeTask(state.Project.Root, t); err != nil {
		return MutationResult{}, err
	}
	return MutationResult{ID: t.ID, File: t.File, Status: t.Status, OK: true}, nil
}

func taskHasSection(t *task.Task, name string) bool {
	for _, section := range t.Body.Sections {
		if section.Name == name {
			return true
		}
	}
	return false
}
