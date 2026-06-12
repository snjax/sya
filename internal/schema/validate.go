package schema

import (
	"fmt"
	"sort"
)

type Diagnostic struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type ValidationResult struct {
	Violations []Diagnostic `json:"violations,omitempty"`
	Warnings   []Diagnostic `json:"warnings,omitempty"`
	schema     *Schema
	typeNames  []string
}

func (r ValidationResult) Valid() bool {
	return len(r.Violations) == 0
}

func (r ValidationResult) Err() error {
	if r.Valid() {
		return nil
	}
	return ValidationError{Violations: r.Violations}
}

type ValidationError struct {
	Violations []Diagnostic
}

func (e ValidationError) Error() string {
	if len(e.Violations) == 1 {
		return e.Violations[0].Message
	}
	return fmt.Sprintf("schema has %d violations", len(e.Violations))
}

func (s *Schema) Validate() ValidationResult {
	var result ValidationResult
	if s == nil {
		result.addViolation("schema_nil", "", "schema is nil")
		return result
	}

	typeNames := mapKeys(s.Types)
	result.schema = s
	result.typeNames = typeNames
	typeSet := makeStringSet(typeNames)
	if s.Defaults.Type != "" && !typeSet[s.Defaults.Type] {
		result.addViolation("unknown_default_type", "defaults.type", fmt.Sprintf("defaults.type %q is not declared in types", s.Defaults.Type))
	}

	for _, relationName := range mapKeys(s.Relations) {
		relation := s.Relations[relationName]
		result.validateTypeRefs("relation_type_ref", "relations."+relationName+".from", relation.From, typeSet, true)
		result.validateTypeRefs("relation_type_ref", "relations."+relationName+".to", relation.To, typeSet, true)
	}

	for _, typeName := range typeNames {
		typeDef := s.Types[typeName]
		result.validateType(typeName, typeDef, typeSet)
	}
	return result
}

func (r *ValidationResult) validateType(typeName string, typeDef TypeDef, typeSet map[string]bool) {
	prefix := "types." + typeName
	pipeline := makeStringSet(typeDef.Pipeline)
	if len(typeDef.Terminal) == 0 {
		r.addViolation("terminal_required", prefix+".terminal", fmt.Sprintf("type %q must declare terminal statuses", typeName))
	}
	r.validateStatusSubset("terminal_subset", prefix+".terminal", typeDef.Terminal, pipeline)
	r.validateStatusSubset("working_subset", prefix+".working", typeDef.Working, pipeline)
	r.validateStatusSubset("parked_subset", prefix+".parked", typeDef.Parked, pipeline)
	for _, status := range mapKeys(typeDef.Statuses) {
		if !pipeline[status] {
			r.addViolation("status_description_unknown", prefix+".statuses."+status, fmt.Sprintf("status description %q is not in pipeline for type %q", status, typeName))
		}
	}
	if len(typeDef.Children) > 0 && !typeDef.Container {
		r.addViolation("children_on_non_container", prefix+".children", fmt.Sprintf("type %q declares children but is not a container", typeName))
	}
	r.validateTypeRefs("child_type_ref", prefix+".children", typeDef.Children, typeSet, false)

	transitions, transitionDiagnostics := expandTransitions(typeDef, prefix+".transitions")
	for _, diagnostic := range transitionDiagnostics {
		r.Violations = append(r.Violations, diagnostic)
	}
	for _, transition := range transitions {
		path := prefix + ".transitions." + transitionKey(transition.From, transition.To)
		if !pipeline[transition.From] {
			r.addViolation("transition_source_unknown", path, fmt.Sprintf("transition source %q is not in pipeline for type %q", transition.From, typeName))
		}
		if !pipeline[transition.To] {
			r.addViolation("transition_target_unknown", path, fmt.Sprintf("transition target %q is not in pipeline for type %q", transition.To, typeName))
		}
		if transition.Kind != "" && transition.Kind != TransitionAdvance && transition.Kind != TransitionSetback {
			r.addViolation("transition_kind_unknown", path+".kind", fmt.Sprintf("transition kind %q is not supported", transition.Kind))
		}
	}
	r.validateReachability(typeName, typeDef, transitions, pipeline)
	r.validateOutgoing(typeName, typeDef, transitions, pipeline)
	r.validateGuards(typeName, typeDef)
}

func (r *ValidationResult) validateTypeRefs(kind, path string, refs []string, typeSet map[string]bool, allowWildcard bool) {
	for _, ref := range refs {
		if ref == "*" {
			if !allowWildcard {
				r.addViolation(kind, path, fmt.Sprintf("wildcard is not allowed in %s", path))
			}
			continue
		}
		if !typeSet[ref] {
			r.addViolation(kind, path, fmt.Sprintf("type reference %q is not declared", ref))
		}
	}
}

func (r *ValidationResult) validateStatusSubset(kind, path string, statuses []string, pipeline map[string]bool) {
	for _, status := range statuses {
		if !pipeline[status] {
			r.addViolation(kind, path, fmt.Sprintf("status %q is not in pipeline", status))
		}
	}
}

func (r *ValidationResult) validateReachability(typeName string, typeDef TypeDef, transitions []Transition, pipeline map[string]bool) {
	if len(typeDef.Pipeline) == 0 {
		return
	}
	initial := typeDef.Pipeline[0]
	if !pipeline[initial] {
		return
	}
	graph := make(map[string][]string)
	for _, transition := range transitions {
		if pipeline[transition.From] && pipeline[transition.To] {
			graph[transition.From] = append(graph[transition.From], transition.To)
		}
	}
	seen := map[string]bool{initial: true}
	queue := []string{initial}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range graph[current] {
			if seen[next] {
				continue
			}
			seen[next] = true
			queue = append(queue, next)
		}
	}
	for _, status := range typeDef.Pipeline {
		if !seen[status] {
			r.addViolation("status_unreachable", "types."+typeName+".pipeline", fmt.Sprintf("status %q is not reachable from initial status %q for type %q", status, initial, typeName))
		}
	}
}

func (r *ValidationResult) validateOutgoing(typeName string, typeDef TypeDef, transitions []Transition, pipeline map[string]bool) {
	terminal := makeStringSet(typeDef.Terminal)
	parked := makeStringSet(typeDef.Parked)
	outgoing := make(map[string]bool)
	explicitOutgoing := make(map[string]bool)
	for _, transition := range transitions {
		if pipeline[transition.From] && pipeline[transition.To] {
			outgoing[transition.From] = true
		}
	}
	for _, transition := range typeDef.Transitions {
		if transition.From != "*" && pipeline[transition.From] && pipeline[transition.To] {
			explicitOutgoing[transition.From] = true
		}
	}
	for _, status := range typeDef.Pipeline {
		if terminal[status] || parked[status] {
			continue
		}
		if !outgoing[status] {
			r.addWarning("status_dead_end", "types."+typeName+".transitions", fmt.Sprintf("active status %q has no outgoing transition for type %q", status, typeName))
			continue
		}
		if !explicitOutgoing[status] {
			r.addWarning("status_dead_end", "types."+typeName+".transitions", fmt.Sprintf("active status %q has only wildcard-derived outgoing transitions for type %q", status, typeName))
		}
	}
}

func (r *ValidationResult) validateGuards(typeName string, typeDef TypeDef) {
	for _, key := range sortedTransitionKeys(typeDef.Transitions, typeDef.Pipeline) {
		transition := typeDef.Transitions[key]
		path := "types." + typeName + ".transitions." + transitionKey(transition.From, transition.To)
		for guardIndex, guard := range transition.Guards {
			guardPath := fmt.Sprintf("%s.guards[%d]", path, guardIndex)
			def, ok := guardKindRegistry[guard.Kind]
			if !ok {
				r.addViolation("guard_kind_unknown", guardPath+".kind", fmt.Sprintf("guard kind %q is not supported", guard.Kind))
				continue
			}
			r.Violations = append(r.Violations, def.ValidateDecl(guardValidateContext{
				Result:    r,
				TypeName:  typeName,
				TypeDef:   typeDef,
				GuardPath: guardPath,
				Guard:     guard,
			})...)
		}
	}
}

func (r *ValidationResult) schemaRelations(typeName string) map[string]RelationDef {
	_ = typeName
	if r.schema == nil {
		return nil
	}
	return r.schema.Relations
}

func (r *ValidationResult) relationTargetTypes(typeName, relationName string) []string {
	if r.schema == nil {
		return nil
	}
	relation, ok := r.schema.Relations[relationName]
	if !ok || !typeAllowed(typeName, relation.From) {
		return nil
	}
	return r.expandTypeRefs(relation.To)
}

func (r *ValidationResult) childrenTargetTypes(typeDef TypeDef) []string {
	return append([]string(nil), typeDef.Children...)
}

func (r *ValidationResult) parentTargetTypes(childType string) []string {
	var parents []string
	if r.schema == nil {
		return parents
	}
	for _, typeName := range r.typeNames {
		typeDef := r.schema.Types[typeName]
		if typeDef.Container && stringSetContains(typeDef.Children, childType) {
			parents = append(parents, typeName)
		}
	}
	return parents
}

func (r *ValidationResult) warnUnknownInStatuses(path string, guard Guard, possibleTypes []string) {
	statuses, ok := stringSliceParam(guard, "in")
	if !ok {
		r.addViolation("guard_param_missing", path, "status guard must declare in list")
		return
	}
	for _, status := range statuses {
		if status == "terminal" {
			continue
		}
		if !r.statusExistsInAnyType(status, possibleTypes) {
			r.Warnings = append(r.Warnings, Diagnostic{
				Kind:    "guard_status_unknown",
				Path:    path,
				Message: fmt.Sprintf("literal status %q does not exist in any possible target type", status),
			})
		}
	}
}

func (r *ValidationResult) addViolation(kind, path, message string) {
	r.Violations = append(r.Violations, Diagnostic{Kind: kind, Path: path, Message: message})
}

func (r *ValidationResult) addWarning(kind, path, message string) {
	r.Warnings = append(r.Warnings, Diagnostic{Kind: kind, Path: path, Message: message})
}

func ExpandTransitions(typeDef TypeDef) ([]Transition, error) {
	transitions, diagnostics := expandTransitions(typeDef, "transitions")
	if len(diagnostics) > 0 {
		return nil, ValidationError{Violations: diagnostics}
	}
	return transitions, nil
}

func expandTransitions(typeDef TypeDef, path string) ([]Transition, []Diagnostic) {
	var diagnostics []Diagnostic
	terminal := makeStringSet(typeDef.Terminal)
	explicitBySourceTarget := make(map[string]map[string]Transition)
	wildcards := make(map[string]Transition)
	for key, transition := range typeDef.Transitions {
		if transition.From == "" || transition.To == "" {
			var err error
			transition.From, transition.To, err = parseTransitionKey(key)
			if err != nil {
				diagnostics = append(diagnostics, Diagnostic{Kind: "transition_key_invalid", Path: path + "." + key, Message: err.Error()})
				continue
			}
		}
		if transition.Kind == "" {
			if transition.From == "*" {
				transition.Kind = TransitionSetback
			} else {
				transition.Kind = TransitionAdvance
			}
		}
		if transition.From == transition.To {
			diagnostics = append(diagnostics, Diagnostic{
				Kind:    "transition_self",
				Path:    path + "." + transitionKey(transition.From, transition.To),
				Message: fmt.Sprintf("self-transition %q is forbidden", transitionKey(transition.From, transition.To)),
			})
			continue
		}
		if transition.From == "*" {
			wildcards[transition.To] = transition
			continue
		}
		if explicitBySourceTarget[transition.From] == nil {
			explicitBySourceTarget[transition.From] = make(map[string]Transition)
		}
		explicitBySourceTarget[transition.From][transition.To] = transition
	}

	var expanded []Transition
	for _, from := range sortedSourceKeys(explicitBySourceTarget, typeDef.Pipeline) {
		for _, to := range sortedTransitionTargets(explicitBySourceTarget[from], typeDef.Pipeline) {
			expanded = append(expanded, explicitBySourceTarget[from][to])
		}
	}
	for _, to := range sortedWildcardTargets(wildcards, typeDef.Pipeline) {
		wildcard := wildcards[to]
		for _, from := range typeDef.Pipeline {
			if terminal[from] || from == to {
				continue
			}
			if explicitBySourceTarget[from] != nil {
				if _, exists := explicitBySourceTarget[from][to]; exists {
					continue
				}
			}
			transition := wildcard
			transition.From = from
			transition.To = to
			expanded = append(expanded, transition)
		}
	}
	return expanded, diagnostics
}

func mapKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func makeStringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func stringSetContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func typeAllowed(typeName string, refs []string) bool {
	if len(refs) == 0 {
		return true
	}
	for _, ref := range refs {
		if ref == "*" || ref == typeName {
			return true
		}
	}
	return false
}

func (r *ValidationResult) expandTypeRefs(refs []string) []string {
	if len(refs) == 0 || stringSetContains(refs, "*") {
		return append([]string(nil), r.typeNames...)
	}
	expanded := append([]string(nil), refs...)
	sort.Strings(expanded)
	return expanded
}

func (r *ValidationResult) statusExistsInAnyType(status string, typeNames []string) bool {
	if r.schema == nil {
		return false
	}
	for _, typeName := range typeNames {
		typeDef, ok := r.schema.Types[typeName]
		if !ok {
			continue
		}
		if stringSetContains(typeDef.Pipeline, status) {
			return true
		}
	}
	return false
}

func stringParam(guard Guard, name string) (string, bool) {
	if guard.Params == nil {
		return "", false
	}
	value, ok := guard.Params[name]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok && text != ""
}

func stringSliceParam(guard Guard, name string) ([]string, bool) {
	if guard.Params == nil {
		return nil, false
	}
	value, ok := guard.Params[name]
	if !ok {
		return nil, false
	}
	switch typed := value.(type) {
	case []string:
		return typed, true
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, text)
		}
		return out, true
	default:
		return nil, false
	}
}

func sortedSourceKeys(transitions map[string]map[string]Transition, pipeline []string) []string {
	seen := make(map[string]bool, len(transitions))
	var keys []string
	for _, status := range pipeline {
		if transitions[status] != nil {
			keys = append(keys, status)
			seen[status] = true
		}
	}
	for key := range transitions {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		left := pipelineIndex(keys[i], pipeline)
		right := pipelineIndex(keys[j], pipeline)
		if left == right {
			return keys[i] < keys[j]
		}
		return left < right
	})
	return keys
}

func sortedTransitionTargets(transitions map[string]Transition, pipeline []string) []string {
	keys := make([]string, 0, len(transitions))
	for key := range transitions {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		left := pipelineIndex(keys[i], pipeline)
		right := pipelineIndex(keys[j], pipeline)
		if left == right {
			return keys[i] < keys[j]
		}
		return left < right
	})
	return keys
}

func sortedWildcardTargets(transitions map[string]Transition, pipeline []string) []string {
	keys := make([]string, 0, len(transitions))
	for key := range transitions {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		left := pipelineIndex(keys[i], pipeline)
		right := pipelineIndex(keys[j], pipeline)
		if left == right {
			return keys[i] < keys[j]
		}
		return left < right
	})
	return keys
}

func sortedTransitionKeys(transitions map[string]Transition, pipeline []string) []string {
	keys := make([]string, 0, len(transitions))
	for key := range transitions {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		left := transitions[keys[i]]
		right := transitions[keys[j]]
		leftFrom := pipelineIndex(left.From, pipeline)
		rightFrom := pipelineIndex(right.From, pipeline)
		if leftFrom != rightFrom {
			return leftFrom < rightFrom
		}
		leftTo := pipelineIndex(left.To, pipeline)
		rightTo := pipelineIndex(right.To, pipeline)
		if leftTo != rightTo {
			return leftTo < rightTo
		}
		return keys[i] < keys[j]
	})
	return keys
}

func pipelineIndex(status string, pipeline []string) int {
	for index, candidate := range pipeline {
		if candidate == status {
			return index
		}
	}
	return len(pipeline)
}
