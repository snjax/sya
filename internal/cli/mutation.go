package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/snjax/sya/internal/events"
	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
)

type MutationResult struct {
	ID     string               `json:"id"`
	File   string               `json:"file,omitempty"`
	From   string               `json:"from,omitempty"`
	To     string               `json:"to,omitempty"`
	Status string               `json:"status,omitempty"`
	OK     bool                 `json:"ok"`
	Error  *syaerr.ErrorPayload `json:"error,omitempty"`
	Err    error                `json:"-"`
}

type MutationResults struct {
	Results []MutationResult `json:"results"`
}

type mutationOptions struct {
	Attestations  map[string]string
	ExecuteChecks bool
}

type attestFlagValues []string

func (v *attestFlagValues) String() string {
	return strings.Join(*v, ",")
}

func (v *attestFlagValues) Set(value string) error {
	*v = append(*v, value)
	return nil
}

func (v *attestFlagValues) Type() string {
	return "attest"
}

type partialError struct {
	data any
	code int
}

func (e partialError) Error() string {
	return "partial success"
}

func (r MutationResult) HumanText(Colorizer) string {
	if r.OK {
		if r.From != "" || r.To != "" {
			return fmt.Sprintf("%s: %s -> %s", r.ID, r.From, r.To)
		}
		return fmt.Sprintf("%s: ok", r.ID)
	}
	if r.Error != nil {
		return fmt.Sprintf("%s: %s", r.ID, r.Error.Message)
	}
	return fmt.Sprintf("%s: failed", r.ID)
}

func (r MutationResults) HumanText(c Colorizer) string {
	lines := make([]string, 0, len(r.Results))
	for _, result := range r.Results {
		lines = append(lines, result.HumanText(c))
	}
	return strings.Join(lines, "\n")
}

func transitionForStatus(sch *schema.Schema, t *task.Task, to string) (schema.Transition, bool, error) {
	typeDef, ok := sch.Types[t.Type]
	if !ok {
		return schema.Transition{}, false, syaerr.SchemaInvalid{Message: "unknown task type: " + t.Type}
	}
	transitions, err := schema.ExpandTransitions(typeDef)
	if err != nil {
		return schema.Transition{}, false, err
	}
	for _, transition := range transitions {
		if transition.From == t.Status && transition.To == to {
			return transition, true, nil
		}
	}
	return schema.Transition{}, false, nil
}

func allowedOptions(sch *schema.Schema, resolver schema.Resolver, t *task.Task) []syaerr.TransitionOption {
	view, ok := resolver.Get(t.ID)
	if !ok {
		return nil
	}
	typeDef := sch.Types[t.Type]
	statuses := schema.AvailableTransitions(sch, resolver, view)
	options := make([]syaerr.TransitionOption, 0, len(statuses))
	for _, status := range statuses {
		options = append(options, syaerr.TransitionOption{
			To:          status.Transition.To,
			Kind:        string(status.Transition.Kind),
			Description: status.Transition.Description,
			Working:     stringIn(typeDef.Working, status.Transition.To),
			Terminal:    stringIn(typeDef.Terminal, status.Transition.To),
		})
	}
	return options
}

func passingAlternatives(sch *schema.Schema, resolver schema.Resolver, t *task.Task, excludeTo string) []syaerr.TransitionOption {
	view, ok := resolver.Get(t.ID)
	if !ok {
		return nil
	}
	statuses := schema.AvailableTransitions(sch, resolver, view)
	typeDef := sch.Types[t.Type]
	options := make([]syaerr.TransitionOption, 0, len(statuses))
	for _, status := range statuses {
		if !status.Passing || status.Transition.To == excludeTo {
			continue
		}
		options = append(options, syaerr.TransitionOption{
			To:          status.Transition.To,
			Kind:        string(status.Transition.Kind),
			Description: status.Transition.Description,
			Working:     stringIn(typeDef.Working, status.Transition.To),
			Terminal:    stringIn(typeDef.Terminal, status.Transition.To),
		})
	}
	return options
}

func checkTransition(state *projectState, t *task.Task, transition schema.Transition, opts mutationOptions) []syaerr.Violation {
	view, ok := state.Index.Resolver().Get(t.ID)
	if !ok {
		return nil
	}
	evalOpts := schema.EvalOptions{Attestations: opts.Attestations}
	if opts.ExecuteChecks {
		evalOpts.CheckRunner = shellCheckRunner{dir: state.Project.Root}
		evalOpts.CheckEnv = taskGuardEnv(t)
	}
	return convertViolationsForTask(state, t, schema.EvaluateWithOptions(state.Schema, state.Index.Resolver(), view, transition, evalOpts))
}

func convertViolations(state *projectState, violations []schema.Violation) []syaerr.Violation {
	return convertViolationsForTask(state, nil, violations)
}

func convertViolationsForTask(state *projectState, source *task.Task, violations []schema.Violation) []syaerr.Violation {
	out := make([]syaerr.Violation, 0, len(violations))
	for _, violation := range violations {
		offending := make([]syaerr.Candidate, 0, len(violation.Offending))
		for _, ref := range violation.Offending {
			candidate := syaerr.Candidate{
				ID:     ref.ID,
				Type:   ref.Type,
				Status: ref.Status,
			}
			if state != nil && state.Index != nil {
				if t, err := state.Index.Resolve(ref.ID); err == nil {
					candidate.Title = t.Title
					candidate.Type = t.Type
					candidate.Status = t.Status
					candidate.File = t.File
				}
			}
			offending = append(offending, candidate)
		}
		file := ""
		if violation.Section != "" && source != nil && state != nil {
			file = filepath.Join(state.Project.Root, filepath.FromSlash(source.File))
		}
		out = append(out, syaerr.Violation{
			Kind:      violation.Kind,
			Field:     violation.Field,
			Relation:  violation.Relation,
			Section:   violation.Section,
			File:      file,
			Message:   violation.Message,
			Hint:      violation.Hint,
			Deferred:  violation.Deferred,
			ExitCode:  violation.ExitCode,
			Stderr:    violation.Stderr,
			Question:  violation.Question,
			AttestID:  violation.AttestID,
			Offending: offending,
		})
	}
	return out
}

func transitionError(state *projectState, t *task.Task, transition schema.Transition, violations []syaerr.Violation) error {
	return syaerr.TransitionBlocked{
		Task: t.ID,
		Transition: syaerr.TransitionRef{
			From:        transition.From,
			To:          transition.To,
			Kind:        string(transition.Kind),
			Description: transition.Description,
		},
		Violations:   violations,
		Alternatives: passingAlternatives(state.Schema, state.Index.Resolver(), t, transition.To),
	}
}

func (a *App) transitionDenied(state *projectState, t *task.Task, to string, err error) MutationResult {
	payload := syaerr.Payload(err)
	if to == "" {
		to = payload.To
	}
	if to == "" && payload.Transition != nil {
		to = payload.Transition.To
	}
	if state != nil && t != nil {
		a.recordTransitionEvent(state, t.ID, t.Status, to, events.ResultDenied, syaerr.ErrorType(err), nil, payload.Violations)
	}
	return MutationResult{ID: taskID(t), File: taskFile(t), OK: false, Error: &payload, Err: err}
}

func (a *App) transitionOK(state *projectState, t *task.Task, from, to string, write bool, attest []events.Attestation) MutationResult {
	if write {
		a.recordTransitionEvent(state, t.ID, from, to, events.ResultOK, "", attest, nil)
	}
	return MutationResult{ID: t.ID, File: t.File, From: from, To: to, Status: to, OK: true}
}

func (a *App) recordTransitionEvent(state *projectState, taskID, from, to, result, errorType string, attest []events.Attestation, violations []syaerr.Violation) {
	if state == nil {
		return
	}
	event := events.Event{
		TS:         a.now().UTC(),
		Actor:      a.Actor(),
		Task:       taskID,
		From:       from,
		To:         to,
		Result:     result,
		ErrorType:  errorType,
		Attest:     attest,
		Violations: violations,
	}
	if err := a.appendEvent(state.Project.Root, event); err != nil && !a.quiet {
		fmt.Fprintf(a.err, "warning: could not append event: %v\n", err)
	}
	if result == events.ResultDenied {
		a.fireDeniedTransitionAlert(state.Project, event)
	}
}

func moveTask(state *projectState, root string, t *task.Task, transition schema.Transition, actor string, now time.Time, reason string, write bool) error {
	from := t.Status
	t.Status = transition.To
	if write {
		if err := appendTransitionLog(t, now, actor, from, transition.To, string(transition.Kind), reason); err != nil {
			return err
		}
		return writeTask(state, t)
	}
	t.Status = from
	return nil
}

func parseAttestations(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(values))
	for _, raw := range values {
		id, answer, ok := strings.Cut(raw, "=")
		id = strings.TrimSpace(id)
		answer = strings.TrimSpace(answer)
		if !ok || id == "" || answer == "" {
			return nil, syaerr.Usage{Message: "expected --attest id=\"yes: <justification>\""}
		}
		out[id] = answer
	}
	return out, nil
}

func transitionAttestations(transition schema.Transition, answers map[string]string) []events.Attestation {
	if len(answers) == 0 {
		return nil
	}
	out := make([]events.Attestation, 0)
	seen := make(map[string]bool)
	for _, guard := range transition.Guards {
		if guard.Kind != schema.GuardAttest || guard.Params == nil {
			continue
		}
		id, ok := guard.Params["id"].(string)
		if !ok || id == "" || seen[id] {
			continue
		}
		answer := answers[id]
		if !attestAnswerValid(answer) {
			continue
		}
		out = append(out, events.Attestation{ID: id, Answer: answer})
		seen[id] = true
	}
	return out
}

func appendAttestationLogs(t *task.Task, ts time.Time, actor string, attest []events.Attestation) error {
	for _, item := range attest {
		if err := appendTaskLog(t, ts, actor, fmt.Sprintf("attested %s: %s", item.ID, item.Answer)); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) emitGuardSuccesses(transition schema.Transition, opts mutationOptions) {
	if a == nil || a.quiet {
		return
	}
	seenAttest := make(map[string]bool)
	for _, guard := range transition.Guards {
		switch guard.Kind {
		case schema.GuardCheck:
			if !opts.ExecuteChecks {
				continue
			}
			fmt.Fprintf(a.err, "✓ check passed: %s\n", checkSuccessLabel(guard))
		case schema.GuardAttest:
			id, ok := guard.Params["id"].(string)
			if !ok || id == "" || seenAttest[id] || !attestAnswerValid(opts.Attestations[id]) {
				continue
			}
			fmt.Fprintf(a.err, "✓ attested %s\n", id)
			seenAttest[id] = true
		}
	}
}

func checkSuccessLabel(guard schema.Guard) string {
	if strings.TrimSpace(guard.Message) != "" {
		return strings.TrimSpace(guard.Message)
	}
	if guard.Params != nil {
		if run, ok := guard.Params["run"].(string); ok && strings.TrimSpace(run) != "" {
			return strings.TrimSpace(run)
		}
	}
	return "check"
}

func attestAnswerValid(answer string) bool {
	trimmed := strings.TrimSpace(answer)
	if !strings.HasPrefix(strings.ToLower(trimmed), "yes:") {
		return false
	}
	return len([]rune(strings.TrimSpace(trimmed[len("yes:"):]))) >= 10
}

func taskGuardEnv(t *task.Task) map[string]string {
	return map[string]string{
		"SYA_TASK_ID":   t.ID,
		"SYA_TASK_FILE": t.File,
	}
}

type shellCheckRunner struct {
	dir string
}

func (r shellCheckRunner) Run(command string, timeout time.Duration, env map[string]string) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = r.dir
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	tail := stderrTail(stderr.String(), 4096)
	if ctx.Err() == context.DeadlineExceeded {
		return -1, tail, ctx.Err()
	}
	if err == nil {
		return 0, tail, nil
	}
	var exitErr *exec.ExitError
	if ok := errors.As(err, &exitErr); ok {
		return exitErr.ExitCode(), tail, err
	}
	return -1, tail, err
}

func stderrTail(text string, limit int) string {
	if len(text) <= limit {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(text[len(text)-limit:])
}

func appendTransitionLog(t *task.Task, ts time.Time, actor, from, to, kind, reason string) error {
	marker := ""
	if kind == string(schema.TransitionSetback) {
		marker = " ↩"
	}
	line := fmt.Sprintf("%s -> %s%s", from, to, marker)
	if strings.TrimSpace(reason) != "" {
		line += ": " + strings.TrimSpace(reason)
	}
	return appendTaskLog(t, ts, actor, line)
}

func appendTaskLog(t *task.Task, ts time.Time, actor, message string) error {
	return task.AppendLogAt(t, ts.UTC(), actor, message)
}

func declaredField(typeDef schema.TypeDef, name string) (schema.FieldDef, bool) {
	field, ok := typeDef.Fields[name]
	return field, ok
}

func parseFieldValue(field schema.FieldDef, raw string) (any, error) {
	value := parseScalar(raw)
	switch field.Type {
	case "", "string":
		return fmt.Sprint(value), nil
	case "bool":
		parsed, ok := value.(bool)
		if !ok {
			return nil, syaerr.Usage{Message: "field expects bool"}
		}
		return parsed, nil
	case "int":
		parsed, ok := value.(int)
		if !ok {
			return nil, syaerr.Usage{Message: "field expects int"}
		}
		return parsed, nil
	case "enum":
		str := fmt.Sprint(value)
		if !stringIn(field.Values, str) {
			return nil, syaerr.Usage{Message: "field value not in enum: " + str}
		}
		return str, nil
	default:
		return value, nil
	}
}

func canonicalRelation(sch *schema.Schema, left, relation, right string) (from, canonical, to string, err error) {
	if def, ok := sch.Relations[relation]; ok {
		if def.Symmetric && right < left {
			return right, relation, left, nil
		}
		return left, relation, right, nil
	}
	for name, def := range sch.Relations {
		if def.Reverse == relation {
			if def.Symmetric && left < right {
				return left, name, right, nil
			}
			return right, name, left, nil
		}
	}
	return "", "", "", syaerr.Usage{Message: "unknown relation: " + relation}
}

func relationTypeCheck(sch *schema.Schema, idx *index.Index, from, relation, to string) error {
	def := sch.Relations[relation]
	source, err := idx.Resolve(from)
	if err != nil {
		return err
	}
	target, err := idx.Resolve(to)
	if err != nil {
		return err
	}
	if len(def.From) > 0 && !typeAllowed(source.Type, def.From) {
		return syaerr.Usage{Message: "relation " + relation + " cannot originate from type " + source.Type}
	}
	if len(def.To) > 0 && !typeAllowed(target.Type, def.To) {
		return syaerr.Usage{Message: "relation " + relation + " cannot target type " + target.Type}
	}
	return nil
}

func wouldCreateCycle(idx *index.Index, from, relation, to string) bool {
	return wouldCreateCycleWithPending(idx, nil, from, relation, to)
}

type canonicalRelationEdge struct {
	From     string
	Relation string
	To       string
}

func wouldCreateCycleWithPending(idx *index.Index, pending []canonicalRelationEdge, from, relation, to string) bool {
	origins := idx.CanonicalOrigins()
	graph := make(map[string][]string)
	for edge := range origins {
		if edge.Relation != relation {
			continue
		}
		graph[edge.From] = append(graph[edge.From], edge.To)
	}
	for _, edge := range pending {
		if edge.Relation != relation {
			continue
		}
		graph[edge.From] = append(graph[edge.From], edge.To)
	}
	graph[from] = append(graph[from], to)
	seen := make(map[string]bool)
	var visit func(string) bool
	visit = func(id string) bool {
		if id == from {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		for _, next := range graph[id] {
			if visit(next) {
				return true
			}
		}
		return false
	}
	for _, next := range graph[to] {
		if visit(next) {
			return true
		}
	}
	return false
}

func taskID(t *task.Task) string {
	if t == nil {
		return ""
	}
	return t.ID
}

func taskFile(t *task.Task) string {
	if t == nil {
		return ""
	}
	return t.File
}

func removeString(values []string, target string) []string {
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

func sortedTaskIDs(tasks []*task.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)
	return ids
}
