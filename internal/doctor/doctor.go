package doctor

import (
	"fmt"
	"io/fs"
	"reflect"
	"sort"
	"strings"

	"github.com/snjax/sya/internal/fsutil"
	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/task"
)

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

type Options struct {
	Strict bool
}

type Report struct {
	Findings []Finding `json:"findings"`
}

type Finding struct {
	Kind     string   `json:"kind"`
	Severity string   `json:"severity"`
	Path     string   `json:"path,omitempty"`
	Paths    []string `json:"paths,omitempty"`
	TaskID   string   `json:"task_id,omitempty"`
	Message  string   `json:"message"`
	Fixable  bool     `json:"fixable,omitempty"`
}

func Run(fsys fs.FS, projectDir string, sch *schema.Schema, idx *index.Index, opts Options) (Report, error) {
	_ = opts
	if idx == nil {
		loaded, err := index.Load(fsys, projectDir, sch)
		if err != nil {
			return Report{}, err
		}
		idx = loaded
	}

	runner := runner{schema: sch, index: idx}
	runner.checkQuarantine()
	runner.checkIndexWarnings()
	runner.checkTasks()
	runner.checkDAGs()
	runner.checkRuntimeFiles(fsys, projectDir)
	runner.sort()
	return Report{Findings: runner.findings}, nil
}

type runner struct {
	schema   *schema.Schema
	index    *index.Index
	findings []Finding
}

func (r *runner) checkQuarantine() {
	for _, quarantined := range r.index.Quarantined() {
		kind := "task_parse_error"
		if strings.Contains(quarantined.Reason, "conflict markers") {
			kind = "conflict_markers"
		}
		r.add(Finding{
			Kind:     kind,
			Severity: SeverityError,
			Path:     quarantined.Path,
			Message:  quarantined.Reason,
			Fixable:  kind == "conflict_markers",
		})
	}
}

func (r *runner) checkIndexWarnings() {
	for _, warning := range r.index.Warnings() {
		switch warning.Kind {
		case "duplicate_id":
			r.add(Finding{
				Kind:     "duplicate_id",
				Severity: SeverityError,
				Path:     first(warning.Paths),
				Paths:    append([]string(nil), warning.Paths...),
				Message:  warning.Message,
				Fixable:  true,
			})
		case "duplicate_edge":
			r.add(Finding{
				Kind:     "symmetric_duplicate_edge",
				Severity: SeverityWarning,
				Path:     first(warning.Paths),
				Paths:    append([]string(nil), warning.Paths...),
				Message:  warning.Message,
				Fixable:  true,
			})
		default:
			r.add(Finding{
				Kind:     warning.Kind,
				Severity: SeverityWarning,
				Path:     warning.Path,
				Paths:    append([]string(nil), warning.Paths...),
				Message:  warning.Message,
				Fixable:  warning.Edge != nil,
			})
		}
	}
}

func (r *runner) checkTasks() {
	for _, t := range r.index.All() {
		typeDef, ok := r.schema.Types[t.Type]
		if !ok {
			r.taskFinding(t, "task_type_unknown", SeverityError, fmt.Sprintf("task type %q is not declared in schema", t.Type), false)
			continue
		}
		if !contains(typeDef.Pipeline, t.Status) {
			r.taskFinding(t, "task_status_unknown", SeverityError, fmt.Sprintf("status %q is not in pipeline for type %q", t.Status, t.Type), false)
		}
		r.checkSchemaVersion(t)
		r.checkFields(t, typeDef)
		r.checkParent(t)
		r.checkRelations(t)
		r.checkSections(t, typeDef)
	}
}

func (r *runner) checkSchemaVersion(t *task.Task) {
	if r.schema == nil {
		return
	}
	switch {
	case t.SchemaVersion > r.schema.SchemaVersion:
		r.taskFinding(t, "schema_version_future", SeverityError, fmt.Sprintf("task schema_version %d is newer than schema version %d", t.SchemaVersion, r.schema.SchemaVersion), false)
	case t.SchemaVersion < r.schema.SchemaVersion:
		r.taskFinding(t, "schema_version_drift", SeverityInfo, fmt.Sprintf("task schema_version %d is older than schema version %d", t.SchemaVersion, r.schema.SchemaVersion), true)
	}
}

func (r *runner) checkFields(t *task.Task, typeDef schema.TypeDef) {
	for name, value := range t.Fields {
		fieldDef, ok := typeDef.Fields[name]
		if !ok {
			r.taskFinding(t, "field_unknown", SeverityError, fmt.Sprintf("field %q is not declared for type %q", name, t.Type), false)
			continue
		}
		if !fieldMatches(fieldDef, value) {
			r.taskFinding(t, "field_type_invalid", SeverityError, fmt.Sprintf("field %q does not match declared type %q", name, fieldDef.Type), false)
		}
	}
}

func (r *runner) checkParent(t *task.Task) {
	if t.Parent == "" {
		return
	}
	parent, err := r.index.Get(t.Parent)
	if err != nil {
		r.taskFinding(t, "dangling_parent", SeverityError, fmt.Sprintf("parent %q does not resolve", t.Parent), false)
		return
	}
	parentType, ok := r.schema.Types[parent.Type]
	if !ok {
		return
	}
	if !parentType.Container {
		r.taskFinding(t, "parent_not_container", SeverityError, fmt.Sprintf("parent %q has non-container type %q", parent.ID, parent.Type), false)
		return
	}
	if !contains(parentType.Children, t.Type) {
		r.taskFinding(t, "parent_child_type_invalid", SeverityError, fmt.Sprintf("parent type %q does not allow child type %q", parent.Type, t.Type), false)
	}
}

func (r *runner) checkRelations(t *task.Task) {
	for relation, targets := range t.Relations {
		relationDef, ok := r.schema.Relations[relation]
		if !ok {
			r.taskFinding(t, "relation_unknown", SeverityError, fmt.Sprintf("relation %q is not declared in schema", relation), false)
			continue
		}
		if !typeAllowed(t.Type, relationDef.From) {
			r.taskFinding(t, "relation_from_type_invalid", SeverityError, fmt.Sprintf("relation %q cannot start from type %q", relation, t.Type), false)
		}
		for _, targetID := range targets {
			target, err := r.index.Get(targetID)
			if err != nil {
				r.taskFinding(t, "dangling_relation", SeverityError, fmt.Sprintf("relation %q target %q does not resolve", relation, targetID), false)
				continue
			}
			if !typeAllowed(target.Type, relationDef.To) {
				r.taskFinding(t, "relation_to_type_invalid", SeverityError, fmt.Sprintf("relation %q cannot target type %q", relation, target.Type), false)
			}
		}
	}
}

func (r *runner) checkSections(t *task.Task, typeDef schema.TypeDef) {
	allowed := make(map[string]bool, len(typeDef.Sections)+1)
	for _, section := range typeDef.Sections {
		allowed[section] = true
	}
	allowed["Log"] = true
	for _, section := range t.Body.Sections {
		if section.Name == "" || allowed[section.Name] {
			continue
		}
		r.taskFinding(t, "section_unknown", SeverityError, fmt.Sprintf("section %q is not declared for type %q", section.Name, t.Type), false)
	}
}

func (r *runner) checkDAGs() {
	r.checkParentCycles()
	for relation, relationDef := range r.schema.Relations {
		if relationDef.Graph != "dag" {
			continue
		}
		r.checkRelationCycle(relation)
	}
}

func (r *runner) checkParentCycles() {
	for _, cycle := range cycles(r.index.All(), func(t *task.Task) []string {
		if t.Parent == "" {
			return nil
		}
		return []string{t.Parent}
	}) {
		r.add(Finding{
			Kind:     "parent_cycle",
			Severity: SeverityError,
			Path:     taskPath(r.index, cycle[0]),
			TaskID:   cycle[0],
			Message:  "parent cycle detected: " + strings.Join(cycle, " -> "),
		})
	}
}

func (r *runner) checkRelationCycle(relation string) {
	for _, cycle := range cycles(r.index.All(), func(t *task.Task) []string {
		return t.Relations[relation]
	}) {
		r.add(Finding{
			Kind:     "relation_cycle",
			Severity: SeverityError,
			Path:     taskPath(r.index, cycle[0]),
			TaskID:   cycle[0],
			Message:  fmt.Sprintf("graph:dag relation %q has cycle: %s", relation, strings.Join(cycle, " -> ")),
		})
	}
}

func (r *runner) checkRuntimeFiles(fsys fs.FS, projectDir string) {
	for _, name := range fsutil.SearchIgnoreFiles {
		filePath := runtimePath(projectDir, name)
		if _, err := fs.Stat(fsys, filePath); err != nil {
			r.add(Finding{
				Kind:     "search_ignore_missing",
				Severity: SeverityInfo,
				Path:     filePath,
				Message:  filePath + " should exist to keep search tools from indexing raw sya data",
				Fixable:  true,
			})
		}
	}

	data, err := fs.ReadFile(fsys, ".gitignore")
	if _, statErr := fs.Stat(fsys, strings.Trim(projectDir, "/")+"/events.jsonl"); statErr == nil {
		if err != nil || !gitignoreContains(data, ".sya/events.jsonl") {
			r.add(Finding{
				Kind:     "events_not_ignored",
				Severity: SeverityWarning,
				Path:     ".gitignore",
				Message:  ".sya/events.jsonl should be ignored; run sya init in new projects or add .sya/events.jsonl to .gitignore",
				Fixable:  false,
			})
		}
	}
	if _, statErr := fs.Stat(fsys, strings.Trim(projectDir, "/")+"/.lock"); statErr == nil {
		if err != nil || !gitignoreContains(data, ".sya/.lock") {
			r.add(Finding{
				Kind:     "lock_not_ignored",
				Severity: SeverityWarning,
				Path:     ".gitignore",
				Message:  ".sya/.lock should be ignored; run sya init in new projects or add .sya/.lock to .gitignore",
				Fixable:  false,
			})
		}
	}
}

func runtimePath(projectDir, name string) string {
	dir := strings.Trim(projectDir, "/")
	if dir == "" || dir == "." {
		return name
	}
	return dir + "/" + name
}

func gitignoreContains(data []byte, entry string) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}

func (r *runner) taskFinding(t *task.Task, kind, severity, message string, fixable bool) {
	r.add(Finding{
		Kind:     kind,
		Severity: severity,
		Path:     t.File,
		TaskID:   t.ID,
		Message:  message,
		Fixable:  fixable,
	})
}

func (r *runner) add(f Finding) {
	r.findings = append(r.findings, f)
}

func (r *runner) sort() {
	sort.SliceStable(r.findings, func(i, j int) bool {
		left, right := r.findings[i], r.findings[j]
		if left.Severity != right.Severity {
			return severityRank(left.Severity) > severityRank(right.Severity)
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.TaskID != right.TaskID {
			return left.TaskID < right.TaskID
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.Message < right.Message
	})
}

func fieldMatches(field schema.FieldDef, value any) bool {
	switch field.Type {
	case "", "any":
		return true
	case "bool":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "enum":
		text, ok := value.(string)
		return ok && contains(field.Values, text)
	case "int":
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return true
		default:
			return false
		}
	default:
		if len(field.Values) > 0 {
			return fieldMatches(schema.FieldDef{Type: "enum", Values: field.Values}, value)
		}
		return true
	}
}

func cycles(tasks []*task.Task, edges func(*task.Task) []string) [][]string {
	taskByID := make(map[string]*task.Task, len(tasks))
	for _, t := range tasks {
		taskByID[t.ID] = t
	}
	var found [][]string
	seenCycles := make(map[string]bool)
	visiting := make(map[string]int)
	visited := make(map[string]bool)
	var stack []string
	var visit func(string)
	visit = func(id string) {
		if visited[id] {
			return
		}
		if start, ok := visiting[id]; ok {
			cycle := append([]string(nil), stack[start:]...)
			cycle = append(cycle, id)
			key := canonicalCycleKey(cycle)
			if !seenCycles[key] {
				seenCycles[key] = true
				found = append(found, cycle)
			}
			return
		}
		t := taskByID[id]
		if t == nil {
			return
		}
		visiting[id] = len(stack)
		stack = append(stack, id)
		for _, next := range edges(t) {
			if taskByID[next] != nil {
				visit(next)
			}
		}
		stack = stack[:len(stack)-1]
		delete(visiting, id)
		visited[id] = true
	}
	for _, t := range tasks {
		visit(t.ID)
	}
	return found
}

func canonicalCycleKey(cycle []string) string {
	if len(cycle) <= 1 {
		return strings.Join(cycle, "->")
	}
	nodes := append([]string(nil), cycle[:len(cycle)-1]...)
	min := 0
	for i := range nodes {
		if nodes[i] < nodes[min] {
			min = i
		}
	}
	rotated := append(nodes[min:], nodes[:min]...)
	return strings.Join(rotated, "->")
}

func taskPath(idx *index.Index, id string) string {
	t, err := idx.Get(id)
	if err != nil {
		return ""
	}
	return t.File
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

func contains[T comparable](values []T, needle T) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func severityRank(severity string) int {
	switch severity {
	case SeverityError:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

func taskFrontmatterEqual(left, right *task.Task) bool {
	leftCopy := *left
	rightCopy := *right
	leftCopy.Body, rightCopy.Body = task.Body{}, task.Body{}
	leftCopy.File, rightCopy.File = "", ""
	return reflect.DeepEqual(leftCopy, rightCopy)
}
