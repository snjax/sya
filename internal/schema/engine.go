package schema

import (
	"fmt"
	"reflect"
	"sort"
)

type Resolver interface {
	Get(id string) (TaskView, bool)
}

type TaskView interface {
	Status() string
	Type() string
	Relations(name string) []string
	Children() []string
	Parent() (string, bool)
	Field(name string) (any, bool)
	SectionNonEmpty(name string) bool
	Archived() bool
}

type Violation struct {
	Kind      string    `json:"kind"`
	Message   string    `json:"message"`
	Hint      string    `json:"hint,omitempty"`
	Relation  string    `json:"relation,omitempty"`
	Field     string    `json:"field,omitempty"`
	Section   string    `json:"section,omitempty"`
	Offending []TaskRef `json:"offending,omitempty"`
}

type TaskRef struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Status   string `json:"status,omitempty"`
	Archived bool   `json:"archived,omitempty"`
}

type TransitionStatus struct {
	Transition Transition  `json:"transition"`
	Passing    bool        `json:"passing"`
	Violations []Violation `json:"violations,omitempty"`
}

type BlockedStatus struct {
	Blocked     bool               `json:"blocked"`
	DeadEnd     bool               `json:"dead_end,omitempty"`
	Transitions []TransitionStatus `json:"transitions,omitempty"`
}

func Evaluate(schema *Schema, resolver Resolver, task TaskView, transition Transition) []Violation {
	if schema == nil || task == nil {
		return nil
	}
	var violations []Violation
	typeDef, ok := schema.Types[task.Type()]
	if !ok {
		return nil
	}
	violations = append(violations, evaluateImplicitBlocking(schema, resolver, task, typeDef, transition)...)
	for _, guard := range transition.Guards {
		if violation, failed := evaluateGuard(schema, resolver, task, guard); failed {
			violations = append(violations, violation)
		}
	}
	return violations
}

func AvailableTransitions(schema *Schema, resolver Resolver, task TaskView) []TransitionStatus {
	if schema == nil || task == nil {
		return nil
	}
	typeDef, ok := schema.Types[task.Type()]
	if !ok {
		return nil
	}
	transitions, err := ExpandTransitions(typeDef)
	if err != nil {
		return nil
	}
	var statuses []TransitionStatus
	for _, transition := range transitions {
		if transition.From != task.Status() {
			continue
		}
		violations := Evaluate(schema, resolver, task, transition)
		statuses = append(statuses, TransitionStatus{
			Transition: transition,
			Passing:    len(violations) == 0,
			Violations: violations,
		})
	}
	sortTransitionStatuses(statuses, typeDef.Pipeline)
	return statuses
}

func Ready(schema *Schema, resolver Resolver, task TaskView) bool {
	if schema == nil || task == nil {
		return false
	}
	typeDef, ok := schema.Types[task.Type()]
	if !ok || isTerminalOrParked(typeDef, task.Status()) {
		return false
	}
	for _, status := range candidateTransitions(schema, resolver, task, typeDef) {
		if status.Passing {
			return true
		}
	}
	return false
}

func Blocked(schema *Schema, resolver Resolver, task TaskView) BlockedStatus {
	var result BlockedStatus
	if schema == nil || task == nil {
		return result
	}
	typeDef, ok := schema.Types[task.Type()]
	if !ok || isTerminalOrParked(typeDef, task.Status()) {
		return result
	}
	candidates := candidateTransitions(schema, resolver, task, typeDef)
	result.Transitions = candidates
	if len(candidates) == 0 {
		result.Blocked = true
		result.DeadEnd = true
		return result
	}
	for _, candidate := range candidates {
		if candidate.Passing {
			return result
		}
	}
	result.Blocked = true
	return result
}

func candidateTransitions(schema *Schema, resolver Resolver, task TaskView, typeDef TypeDef) []TransitionStatus {
	var statuses []TransitionStatus
	for _, key := range sortedTransitionKeys(typeDef.Transitions, typeDef.Pipeline) {
		transition := typeDef.Transitions[key]
		if transition.From == "*" || transition.From != task.Status() {
			continue
		}
		violations := Evaluate(schema, resolver, task, transition)
		statuses = append(statuses, TransitionStatus{
			Transition: transition,
			Passing:    len(violations) == 0,
			Violations: violations,
		})
	}
	return statuses
}

func evaluateImplicitBlocking(schema *Schema, resolver Resolver, task TaskView, typeDef TypeDef, transition Transition) []Violation {
	if !stringSetContains(typeDef.Working, transition.To) && !stringSetContains(typeDef.Terminal, transition.To) {
		return nil
	}
	ignored := makeStringSet(transition.IgnoreBlocking)
	var violations []Violation
	for _, relationName := range mapKeys(schema.Relations) {
		relation := schema.Relations[relationName]
		if !relation.Blocking || ignored[relationName] {
			continue
		}
		var offending []TaskRef
		for _, targetID := range task.Relations(relationName) {
			target, ok := resolver.Get(targetID)
			if !ok || !taskMatchesStatuses(schema, target, []string{"terminal"}) {
				offending = append(offending, taskRef(targetID, target, ok))
			}
		}
		if len(offending) > 0 {
			violations = append(violations, Violation{
				Kind:      "blocking_relation",
				Message:   fmt.Sprintf("blocking relation %q has non-terminal targets", relationName),
				Relation:  relationName,
				Offending: offending,
			})
		}
	}
	return violations
}

func evaluateGuard(schema *Schema, resolver Resolver, task TaskView, guard Guard) (Violation, bool) {
	switch guard.Kind {
	case GuardRelationStatus:
		relation, ok := stringParam(guard, "relation")
		if !ok || !relationDeclared(schema, relation) {
			return guardViolation(guard, ""), true
		}
		statuses, ok := stringSliceParam(guard, "in")
		if !ok {
			return guardViolation(guard, relation), true
		}
		return evaluateRelatedStatuses(schema, resolver, task.Relations(relation), statuses, guardViolation(guard, relation))
	case GuardRelationExists:
		relation, ok := stringParam(guard, "relation")
		if !ok || !relationDeclared(schema, relation) {
			return guardViolation(guard, ""), true
		}
		if len(task.Relations(relation)) == 0 {
			return guardViolation(guard, relation), true
		}
		return Violation{}, false
	case GuardField:
		field, ok := stringParam(guard, "field")
		if !ok {
			return guardViolation(guard, ""), true
		}
		value, exists := task.Field(field)
		if !exists || !fieldGuardMatches(guard, value) {
			violation := guardViolation(guard, "")
			violation.Field = field
			return violation, true
		}
		return Violation{}, false
	case GuardChildrenStatus:
		statuses, ok := stringSliceParam(guard, "in")
		if !ok {
			return guardViolation(guard, ""), true
		}
		return evaluateRelatedStatuses(schema, resolver, task.Children(), statuses, guardViolation(guard, ""))
	case GuardParentStatus:
		statuses, ok := stringSliceParam(guard, "in")
		if !ok {
			return guardViolation(guard, ""), true
		}
		parentID, ok := task.Parent()
		if !ok {
			return guardViolation(guard, ""), true
		}
		return evaluateRelatedStatuses(schema, resolver, []string{parentID}, statuses, guardViolation(guard, ""))
	case GuardSectionNonempty:
		section, ok := stringParam(guard, "section")
		if !ok || !task.SectionNonEmpty(section) {
			violation := guardViolation(guard, "")
			violation.Section = section
			return violation, true
		}
		return Violation{}, false
	default:
		return guardViolation(guard, ""), true
	}
}

func evaluateRelatedStatuses(schema *Schema, resolver Resolver, ids []string, statuses []string, base Violation) (Violation, bool) {
	var offending []TaskRef
	for _, id := range ids {
		task, ok := resolver.Get(id)
		if !ok || !taskMatchesStatuses(schema, task, statuses) {
			offending = append(offending, taskRef(id, task, ok))
		}
	}
	if len(offending) == 0 {
		return Violation{}, false
	}
	base.Offending = offending
	return base, true
}

func fieldGuardMatches(guard Guard, value any) bool {
	if expected, ok := guard.Params["equals"]; ok {
		return reflect.DeepEqual(value, expected)
	}
	if allowed, ok := anySliceParam(guard, "in"); ok {
		for _, candidate := range allowed {
			if reflect.DeepEqual(value, candidate) {
				return true
			}
		}
		return false
	}
	return false
}

func taskMatchesStatuses(schema *Schema, task TaskView, statuses []string) bool {
	if task == nil {
		return false
	}
	typeDef, ok := schema.Types[task.Type()]
	if !ok {
		return false
	}
	statusSet := makeStringSet(statuses)
	if statusSet["terminal"] && (task.Archived() || stringSetContains(typeDef.Terminal, task.Status())) {
		return true
	}
	for _, terminal := range typeDef.Terminal {
		if statusSet[terminal] && task.Archived() {
			return true
		}
	}
	if task.Archived() {
		return false
	}
	return statusSet[task.Status()]
}

func relationDeclared(schema *Schema, relation string) bool {
	if schema == nil {
		return false
	}
	_, ok := schema.Relations[relation]
	return ok
}

func guardViolation(guard Guard, relation string) Violation {
	message := guard.Message
	if message == "" {
		message = fmt.Sprintf("guard %q failed", guard.Kind)
	}
	return Violation{
		Kind:     string(guard.Kind),
		Message:  message,
		Hint:     guard.Hint,
		Relation: relation,
	}
}

func taskRef(id string, task TaskView, ok bool) TaskRef {
	ref := TaskRef{ID: id}
	if ok && task != nil {
		ref.Type = task.Type()
		ref.Status = task.Status()
		ref.Archived = task.Archived()
	}
	return ref
}

func isTerminalOrParked(typeDef TypeDef, status string) bool {
	return stringSetContains(typeDef.Terminal, status) || stringSetContains(typeDef.Parked, status)
}

func anySliceParam(guard Guard, name string) ([]any, bool) {
	if guard.Params == nil {
		return nil, false
	}
	value, ok := guard.Params[name]
	if !ok {
		return nil, false
	}
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []string:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, item)
		}
		return values, true
	default:
		return nil, false
	}
}

func sortTransitionStatuses(statuses []TransitionStatus, pipeline []string) {
	sort.SliceStable(statuses, func(i, j int) bool {
		left := statuses[i].Transition
		right := statuses[j].Transition
		leftTo := pipelineIndex(left.To, pipeline)
		rightTo := pipelineIndex(right.To, pipeline)
		if leftTo != rightTo {
			return leftTo < rightTo
		}
		if left.From != right.From {
			return left.From < right.From
		}
		return left.To < right.To
	})
}
