package cli

import (
	"context"

	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var steal bool
		var attest attestFlagValues
		cmd := app.command("claim <id>", "Claim a task", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.withProjectMutationLock(func() (any, error) {
				return app.runClaim(args[0], steal, attest)
			})
		})
		cmd.Flags().BoolVar(&steal, "steal", false, "steal claim from current assignee")
		cmd.Flags().Var(&attest, "attest", "attest guard answer: id=\"yes: <justification>\"")
		return cmd
	})
}

func (a *App) runClaim(id string, steal bool, attestValues attestFlagValues) (MutationResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return MutationResult{}, err
	}
	attestations, err := parseAttestations(attestValues)
	if err != nil {
		return MutationResult{}, err
	}
	if attestations == nil {
		attestations = map[string]string{}
	}
	opts := mutationOptions{Attestations: attestations, ExecuteChecks: true}
	t, err := state.Index.Resolve(id)
	if err != nil {
		return MutationResult{}, err
	}
	actor := a.Actor()
	if t.Assignee != "" && t.Assignee != actor && !steal {
		err := syaerr.AlreadyClaimed{Task: t.ID, Assignee: t.Assignee}
		return a.transitionDenied(state, t, "working", err), err
	}
	typeDef := state.Schema.Types[t.Type]
	if len(typeDef.Working) == 0 {
		return MutationResult{}, syaerr.Usage{Message: "task type has no working statuses"}
	}
	if stringIn(typeDef.Working, t.Status) {
		fromAssignee := t.Assignee
		t.Assignee = actor
		message := "claimed"
		if steal && fromAssignee != "" && fromAssignee != actor {
			message = "claim stolen from " + fromAssignee
		}
		if err := appendTaskLog(t, a.now(), actor, message); err != nil {
			return MutationResult{}, err
		}
		if err := writeTask(state, t); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{ID: t.ID, File: t.File, Status: t.Status, OK: true}, nil
	}
	view, ok := state.Index.Resolver().Get(t.ID)
	if !ok {
		return MutationResult{}, syaerr.NotFound{ID: t.ID}
	}
	statuses := schema.AvailableTransitionsWithOptions(state.Schema, state.Index.Resolver(), view, schema.EvalOptions{
		CheckRunner:  shellCheckRunner{dir: state.Project.Root},
		CheckEnv:     taskGuardEnv(t),
		Attestations: opts.Attestations,
	})
	var blockedWorking *schema.TransitionStatus
	for _, status := range statuses {
		if !stringIn(typeDef.Working, status.Transition.To) {
			continue
		}
		if !status.Passing {
			if blockedWorking == nil {
				blockedWorking = &status
			}
			continue
		}
		from := t.Status
		t.Assignee = actor
		attest := transitionAttestations(status.Transition, opts.Attestations)
		if err := appendAttestationLogs(t, a.now(), actor, attest); err != nil {
			return MutationResult{}, err
		}
		if err := moveTask(state, state.Project.Root, t, status.Transition, actor, a.now(), "", true); err != nil {
			return MutationResult{}, err
		}
		a.emitGuardSuccesses(status.Transition, opts)
		return a.transitionOK(state, t, from, status.Transition.To, true, attest), nil
	}
	if blockedWorking != nil {
		err := transitionError(state, t, blockedWorking.Transition, convertViolationsForTask(state, t, blockedWorking.Violations))
		return a.transitionDenied(state, t, blockedWorking.Transition.To, err), err
	}
	err = syaerr.ClaimNotReachable{
		Task:        t.ID,
		TaskType:    t.Type,
		Working:     append([]string(nil), typeDef.Working...),
		From:        t.Status,
		NextAdvance: nextPassingAdvance(statuses),
	}
	return a.transitionDenied(state, t, "working", err), err
}

func nextPassingAdvance(statuses []schema.TransitionStatus) *syaerr.TransitionOption {
	for _, status := range statuses {
		if !status.Passing || status.Transition.Kind != schema.TransitionAdvance {
			continue
		}
		return &syaerr.TransitionOption{
			To:          status.Transition.To,
			Kind:        string(status.Transition.Kind),
			Description: status.Transition.Description,
		}
	}
	return nil
}
