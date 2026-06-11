package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		return app.command("transitions <id>", "Show available transitions", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runTransitions(args[0])
		})
	})
}

type TransitionsResult struct {
	Task        string             `json:"task"`
	Status      string             `json:"status"`
	Transitions []TransitionStatus `json:"transitions"`
}

type TransitionStatus struct {
	To          string             `json:"to"`
	Kind        string             `json:"kind,omitempty"`
	Description string             `json:"description,omitempty"`
	Passing     bool               `json:"passing"`
	Violations  []syaerr.Violation `json:"violations,omitempty"`
}

func (r TransitionsResult) HumanText(Colorizer) string {
	if len(r.Transitions) == 0 {
		return "no transitions"
	}
	lines := make([]string, 0, len(r.Transitions))
	for _, transition := range r.Transitions {
		status := "blocked"
		if transition.Passing {
			status = "ok"
		}
		lines = append(lines, fmt.Sprintf("%s -> %s [%s]", r.Status, transition.To, status))
	}
	return strings.Join(lines, "\n")
}

func (a *App) runTransitions(id string) (TransitionsResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return TransitionsResult{}, err
	}
	t, err := state.Index.Resolve(id)
	if err != nil {
		return TransitionsResult{}, err
	}
	view, ok := state.Index.Resolver().Get(t.ID)
	if !ok {
		return TransitionsResult{}, syaerr.NotFound{ID: t.ID}
	}
	result := TransitionsResult{Task: t.ID, Status: t.Status}
	for _, status := range schema.AvailableTransitions(state.Schema, state.Index.Resolver(), view) {
		result.Transitions = append(result.Transitions, TransitionStatus{
			To:          status.Transition.To,
			Kind:        string(status.Transition.Kind),
			Description: status.Transition.Description,
			Passing:     status.Passing,
			Violations:  convertViolations(state, status.Violations),
		})
	}
	return result, nil
}
