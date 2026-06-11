package cli

import (
	"context"

	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		return app.command("link <id> <relation> <id2>", "Link two tasks", cobra.ExactArgs(3), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runLink(args[0], args[1], args[2], true)
		})
	})
	registerCommand(func(app *App) *cobra.Command {
		return app.command("unlink <id> <relation> <id2>", "Unlink two tasks", cobra.ExactArgs(3), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runLink(args[0], args[1], args[2], false)
		})
	})
}

type LinkResult struct {
	From     string `json:"from"`
	Relation string `json:"relation"`
	To       string `json:"to"`
	File     string `json:"file"`
	Action   string `json:"action"`
	OK       bool   `json:"ok"`
}

func (r LinkResult) HumanText(Colorizer) string {
	action := r.Action
	if action == "" {
		action = "linked"
	}
	return action + " " + r.From + " " + r.Relation + " " + r.To
}

func (a *App) runLink(left, relation, right string, add bool) (LinkResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return LinkResult{}, err
	}
	leftTask, err := state.Index.Resolve(left)
	if err != nil {
		return LinkResult{}, err
	}
	rightTask, err := state.Index.Resolve(right)
	if err != nil {
		return LinkResult{}, err
	}
	from, canonical, to, err := canonicalRelation(state.Schema, leftTask.ID, relation, rightTask.ID)
	if err != nil {
		return LinkResult{}, err
	}
	if err := relationTypeCheck(state.Schema, state.Index, from, canonical, to); err != nil {
		return LinkResult{}, err
	}
	relationDef := state.Schema.Relations[canonical]
	if add && relationDef.Graph == "dag" && wouldCreateCycle(state.Index, from, canonical, to) {
		return LinkResult{}, syaerr.Usage{Message: "relation would create a cycle"}
	}
	source, err := state.Index.Resolve(from)
	if err != nil {
		return LinkResult{}, err
	}
	if source.Relations == nil {
		source.Relations = make(map[string][]string)
	}
	if add {
		if !stringIn(source.Relations[canonical], to) {
			source.Relations[canonical] = append(source.Relations[canonical], to)
		}
	} else {
		source.Relations[canonical] = removeString(source.Relations[canonical], to)
		if len(source.Relations[canonical]) == 0 {
			delete(source.Relations, canonical)
		}
	}
	if err := writeTask(state.Project.Root, source); err != nil {
		return LinkResult{}, err
	}
	action := "linked"
	if !add {
		action = "unlinked"
	}
	return LinkResult{From: from, Relation: canonical, To: to, File: source.File, Action: action, OK: true}, nil
}
