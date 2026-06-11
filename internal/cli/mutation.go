package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

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
}

type MutationResults struct {
	Results []MutationResult `json:"results"`
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
	statuses := schema.AvailableTransitions(sch, resolver, view)
	options := make([]syaerr.TransitionOption, 0, len(statuses))
	for _, status := range statuses {
		options = append(options, syaerr.TransitionOption{
			To:          status.Transition.To,
			Kind:        string(status.Transition.Kind),
			Description: status.Transition.Description,
		})
	}
	return options
}

func passingAlternatives(sch *schema.Schema, resolver schema.Resolver, t *task.Task) []syaerr.TransitionOption {
	view, ok := resolver.Get(t.ID)
	if !ok {
		return nil
	}
	statuses := schema.AvailableTransitions(sch, resolver, view)
	options := make([]syaerr.TransitionOption, 0, len(statuses))
	for _, status := range statuses {
		if !status.Passing {
			continue
		}
		options = append(options, syaerr.TransitionOption{
			To:          status.Transition.To,
			Kind:        string(status.Transition.Kind),
			Description: status.Transition.Description,
		})
	}
	return options
}

func checkTransition(state *projectState, t *task.Task, transition schema.Transition) []syaerr.Violation {
	view, ok := state.Index.Resolver().Get(t.ID)
	if !ok {
		return nil
	}
	return convertViolations(schema.Evaluate(state.Schema, state.Index.Resolver(), view, transition))
}

func convertViolations(violations []schema.Violation) []syaerr.Violation {
	out := make([]syaerr.Violation, 0, len(violations))
	for _, violation := range violations {
		offending := make([]syaerr.Candidate, 0, len(violation.Offending))
		for _, ref := range violation.Offending {
			offending = append(offending, syaerr.Candidate{
				ID:     ref.ID,
				Type:   ref.Type,
				Status: ref.Status,
			})
		}
		out = append(out, syaerr.Violation{
			Kind:      violation.Kind,
			Field:     violation.Field,
			Relation:  violation.Relation,
			Section:   violation.Section,
			Message:   violation.Message,
			Hint:      violation.Hint,
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
		Alternatives: passingAlternatives(state.Schema, state.Index.Resolver(), t),
	}
}

func moveTask(state *projectState, root string, t *task.Task, transition schema.Transition, actor string, now time.Time, reason string, write bool) error {
	from := t.Status
	t.Status = transition.To
	if write {
		appendTransitionLog(t, now, actor, from, transition.To, string(transition.Kind), reason)
		return writeTask(root, t)
	}
	t.Status = from
	return nil
}

func appendTransitionLog(t *task.Task, ts time.Time, actor, from, to, kind, reason string) {
	marker := ""
	if kind == string(schema.TransitionSetback) {
		marker = " ↩"
	}
	line := fmt.Sprintf("- %s @%s: %s -> %s%s", ts.UTC().Format(time.RFC3339), actor, from, to, marker)
	if strings.TrimSpace(reason) != "" {
		line += ": " + strings.TrimSpace(reason)
	}
	appendLogLine(t, line)
}

func appendLogLine(t *task.Task, line string) {
	idx := -1
	for i, section := range t.Body.Sections {
		if section.Name == "Log" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Body.Sections = append(t.Body.Sections, task.Section{Name: "Log", Raw: []byte("## Log\n" + line + "\n")})
		rebuildTaskBody(t)
		return
	}
	raw := t.Body.Sections[idx].Raw
	if len(raw) > 0 && !strings.HasSuffix(string(raw), "\n") {
		raw = append(raw, '\n')
	}
	raw = append(raw, []byte(line+"\n")...)
	t.Body.Sections[idx].Raw = raw
	rebuildTaskBody(t)
}

func rebuildTaskBody(t *task.Task) {
	var raw []byte
	for _, section := range t.Body.Sections {
		raw = append(raw, section.Raw...)
	}
	t.Body.Raw = raw
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
	origins := idx.CanonicalOrigins()
	graph := make(map[string][]string)
	for edge := range origins {
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
