package cli

import (
	"context"

	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts updateOptions
		cmd := app.command("update <id>", "Update task metadata", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			opts.ID = args[0]
			return app.withProjectMutationLock(func() (any, error) {
				return app.runUpdate(opts)
			})
		})
		cmd.Flags().StringVar(&opts.Title, "title", "", "new title")
		cmd.Flags().StringVar(&opts.Priority, "priority", "", "new priority")
		cmd.Flags().StringVar(&opts.Assignee, "assignee", "", "new assignee")
		cmd.Flags().StringVar(&opts.Parent, "parent", "", "parent id or none")
		cmd.Flags().Var(&opts.Fields, "field", "field key=value")
		cmd.Flags().Var(&opts.Relations, "rel", "relation=ID")
		return cmd
	})
}

type updateOptions struct {
	ID        string
	Title     string
	Priority  string
	Assignee  string
	Parent    string
	Fields    stringList
	Relations stringList
}

func (a *App) runUpdate(opts updateOptions) (MutationResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return MutationResult{}, err
	}
	t, err := state.Index.Resolve(opts.ID)
	if err != nil {
		return MutationResult{}, err
	}
	typeDef := state.Schema.Types[t.Type]
	if opts.Title != "" {
		t.Title = opts.Title
	}
	if opts.Priority != "" {
		t.Priority = opts.Priority
	}
	if opts.Assignee != "" {
		t.Assignee = opts.Assignee
	}
	if opts.Parent != "" {
		if opts.Parent == "none" {
			t.Parent = ""
		} else {
			parent, err := validateParent(state.Index, state.Schema, t.Type, opts.Parent)
			if err != nil {
				return MutationResult{}, err
			}
			t.Parent = parent
		}
	}
	for _, raw := range opts.Fields {
		key, value, err := parseKeyValue(raw)
		if err != nil {
			return MutationResult{}, err
		}
		field, ok := declaredField(typeDef, key)
		if !ok {
			return MutationResult{}, syaerr.Usage{Message: "field is not declared for type: " + key}
		}
		parsed, err := parseFieldValue(field, value)
		if err != nil {
			return MutationResult{}, err
		}
		if t.Fields == nil {
			t.Fields = make(map[string]any)
		}
		t.Fields[key] = parsed
	}
	touched := map[string]struct{}{t.ID: {}}
	if err := a.applyUpdateRelations(state, t, opts.Relations, touched); err != nil {
		return MutationResult{}, err
	}
	for id := range touched {
		changed, err := state.Index.Resolve(id)
		if err != nil {
			return MutationResult{}, err
		}
		if err := writeTask(state.Project.Root, changed); err != nil {
			return MutationResult{}, err
		}
	}
	return MutationResult{ID: t.ID, File: t.File, Status: t.Status, OK: true}, nil
}

func (a *App) applyUpdateRelations(state *projectState, base *task.Task, rawRelations []string, touched map[string]struct{}) error {
	if len(rawRelations) == 0 {
		return nil
	}
	pending := make([]canonicalRelationEdge, 0, len(rawRelations))
	seen := make(map[string]struct{})
	for _, raw := range rawRelations {
		relation, rawID, err := parseKeyValue(raw)
		if err != nil {
			return err
		}
		target, err := state.Index.Resolve(rawID)
		if err != nil {
			return err
		}
		from, canonical, to, err := canonicalRelation(state.Schema, base.ID, relation, target.ID)
		if err != nil {
			return err
		}
		key := from + "\x00" + canonical + "\x00" + to
		if _, ok := seen[key]; ok {
			return syaerr.Usage{Message: "duplicate relation flag: " + relation + "=" + rawID}
		}
		seen[key] = struct{}{}
		if err := relationTypeCheck(state.Schema, state.Index, from, canonical, to); err != nil {
			return err
		}
		relationDef := state.Schema.Relations[canonical]
		if relationDef.Graph == "dag" && wouldCreateCycleWithPending(state.Index, pending, from, canonical, to) {
			return syaerr.Usage{Message: "relation would create a cycle"}
		}
		source, err := state.Index.Resolve(from)
		if err != nil {
			return err
		}
		if source.Relations == nil {
			source.Relations = make(map[string][]string)
		}
		if !stringIn(source.Relations[canonical], to) {
			source.Relations[canonical] = append(source.Relations[canonical], to)
		}
		touched[source.ID] = struct{}{}
		pending = append(pending, canonicalRelationEdge{From: from, Relation: canonical, To: to})
	}
	return nil
}
