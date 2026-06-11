package index

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
)

const childrenRelation = "children"

type Index struct {
	tasks       map[string]*task.Task
	order       []string
	forward     map[string]map[string][]string
	reverse     ReverseEdges
	quarantine  []QuarantinedFile
	warnings    []Warning
	edgeOrigins map[CanonicalEdge][]EdgeOrigin
	schema      *schema.Schema
}

type ReverseEdges map[string]map[string][]string

type QuarantinedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type Warning struct {
	Kind    string         `json:"kind"`
	Message string         `json:"message"`
	Path    string         `json:"path,omitempty"`
	Paths   []string       `json:"paths,omitempty"`
	Edge    *CanonicalEdge `json:"edge,omitempty"`
}

type CanonicalEdge struct {
	From     string `json:"from"`
	Relation string `json:"relation"`
	To       string `json:"to"`
}

type EdgeOrigin struct {
	Path     string `json:"path"`
	TaskID   string `json:"task_id"`
	Relation string `json:"relation"`
	Target   string `json:"target"`
}

type Query struct {
	Types     []string
	Statuses  []string
	Labels    []string
	Parents   []string
	Assignees []string
	Archived  *bool
	Limit     int
}

// Load parses all Markdown task files below dir/tasks in fsys. Invalid files are
// quarantined so callers can still use the rest of the index.
func Load(fsys fs.FS, dir string, sch *schema.Schema) (*Index, error) {
	return LoadWithOptions(fsys, dir, sch, LoadOptions{})
}

func LoadWithOptions(fsys fs.FS, dir string, sch *schema.Schema, opts LoadOptions) (*Index, error) {
	if fsys == nil {
		fsys = &emptyFS{}
	}
	idx := newIndex(sch)
	tasksDir := path.Join(strings.Trim(dir, "/"), "tasks")
	if dir == "" || dir == "." {
		tasksDir = "tasks"
	}
	var taskFiles []taskFileMeta

	err := fs.WalkDir(fsys, tasksDir, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) && filePath == tasksDir {
				return fs.SkipDir
			}
			idx.quarantine = append(idx.quarantine, QuarantinedFile{Path: filePath, Reason: walkErr.Error()})
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() || path.Ext(filePath) != ".md" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			idx.quarantine = append(idx.quarantine, QuarantinedFile{Path: filePath, Reason: err.Error()})
			return nil
		}
		taskFiles = append(taskFiles, taskFileMeta{path: filePath, size: info.Size(), mtimeNS: statMtimeNS(info)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortMetas(taskFiles)

	results := loadTaskFiles(fsys, strings.Trim(dir, "/"), taskFiles, opts)
	for _, meta := range taskFiles {
		result := results[meta.path]
		if result.quarantine != nil {
			idx.quarantine = append(idx.quarantine, *result.quarantine)
			continue
		}
		idx.addTask(result.task)
	}

	idx.rebuildOrder()
	idx.buildReverseEdges()
	return idx, nil
}

func loadTaskFiles(fsys fs.FS, dir string, metas []taskFileMeta, opts LoadOptions) map[string]taskFileResult {
	results := make(map[string]taskFileResult, len(metas))
	ctx, enabled := cacheEnabled(fsys, dir, opts)
	var cache *indexCache
	if enabled {
		cache, _ = readCache(ctx)
	}

	parsePaths := make([]string, 0, len(metas))
	for _, meta := range metas {
		if cached, ok := reusableCachedTask(cache, meta); ok {
			results[meta.path] = taskFileResult{path: meta.path, task: cached}
			continue
		}
		parsePaths = append(parsePaths, meta.path)
	}
	for _, result := range loadTaskFilesParallel(fsys, parsePaths) {
		results[result.path] = result
	}
	if enabled && shouldWriteCache(cache, metas, parsePaths) {
		writeCache(ctx, metas, results)
	}
	return results
}

func shouldWriteCache(cache *indexCache, metas []taskFileMeta, parsePaths []string) bool {
	if cache == nil || len(parsePaths) > 0 || len(cache.Entries) != len(metas) {
		return true
	}
	for _, meta := range metas {
		entry, ok := cache.Entries[meta.path]
		if !ok || entry.Size != meta.size || entry.MtimeNS != meta.mtimeNS {
			return true
		}
	}
	return false
}

func newIndex(sch *schema.Schema) *Index {
	return &Index{
		tasks:       make(map[string]*task.Task),
		forward:     make(map[string]map[string][]string),
		reverse:     make(ReverseEdges),
		edgeOrigins: make(map[CanonicalEdge][]EdgeOrigin),
		schema:      sch,
	}
}

func (i *Index) loadTaskFile(fsys fs.FS, filePath string) {
	result := loadTaskFile(fsys, filePath)
	if result.quarantine != nil {
		i.quarantine = append(i.quarantine, *result.quarantine)
		return
	}
	i.addTask(result.task)
}

func (i *Index) addTask(t *task.Task) {
	if t == nil {
		return
	}
	if existing, ok := i.tasks[t.ID]; ok {
		i.warnings = append(i.warnings, Warning{
			Kind:    "duplicate_id",
			Message: fmt.Sprintf("duplicate task id %q", t.ID),
			Paths:   []string{existing.File, t.File},
		})
		return
	}
	i.tasks[t.ID] = t
}

func loadTaskFile(fsys fs.FS, filePath string) taskFileResult {
	data, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		return taskFileResult{path: filePath, quarantine: &QuarantinedFile{Path: filePath, Reason: err.Error()}}
	}
	t, err := task.ParseBytes(data)
	if err != nil {
		return taskFileResult{path: filePath, quarantine: &QuarantinedFile{Path: filePath, Reason: err.Error()}}
	}
	t.File = filePath
	return taskFileResult{path: filePath, task: t}
}

func (i *Index) rebuildOrder() {
	i.order = i.order[:0]
	for id := range i.tasks {
		i.order = append(i.order, id)
	}
	sort.Slice(i.order, func(a, b int) bool {
		left := i.tasks[i.order[a]]
		right := i.tasks[i.order[b]]
		if left.File != right.File {
			return left.File < right.File
		}
		return left.ID < right.ID
	})
}

func (i *Index) buildReverseEdges() {
	i.forward = make(map[string]map[string][]string)
	i.reverse = make(ReverseEdges)
	i.edgeOrigins = make(map[CanonicalEdge][]EdgeOrigin)

	for _, id := range i.order {
		t := i.tasks[id]
		if t.Parent != "" {
			i.addReverse(t.Parent, childrenRelation, t.ID)
		}
		for relation := range t.Relations {
			targets := append([]string(nil), t.Relations[relation]...)
			sort.Strings(targets)
			for _, target := range targets {
				i.addRelationEdge(t, relation, target)
			}
		}
	}

	for edge, origins := range i.edgeOrigins {
		if len(origins) < 2 {
			continue
		}
		i.warnings = append(i.warnings, Warning{
			Kind:    "duplicate_edge",
			Message: fmt.Sprintf("duplicate canonical edge %s %s %s", edge.From, edge.Relation, edge.To),
			Paths:   edgeOriginPaths(origins),
			Edge:    &CanonicalEdge{From: edge.From, Relation: edge.Relation, To: edge.To},
		})
	}
	sortWarnings(i.warnings)
}

func (i *Index) addRelationEdge(source *task.Task, relation, target string) {
	edge, ok := i.canonicalEdge(source.ID, relation, target)
	if !ok {
		return
	}
	i.edgeOrigins[edge] = append(i.edgeOrigins[edge], EdgeOrigin{
		Path:     source.File,
		TaskID:   source.ID,
		Relation: relation,
		Target:   target,
	})
	i.addForward(edge.From, edge.Relation, edge.To)

	relationDef := i.relationDef(edge.Relation)
	if relationDef.Symmetric {
		i.addReverse(edge.From, edge.Relation, edge.To)
		i.addReverse(edge.To, edge.Relation, edge.From)
		return
	}
	if reverse := relationDef.Reverse; reverse != "" {
		i.addReverse(edge.To, reverse, edge.From)
	}
}

func (i *Index) addForward(id, relation, target string) {
	if id == "" || target == "" {
		return
	}
	if i.forward[id] == nil {
		i.forward[id] = make(map[string][]string)
	}
	if contains(i.forward[id][relation], target) {
		return
	}
	i.forward[id][relation] = append(i.forward[id][relation], target)
	sort.Strings(i.forward[id][relation])
}

func (i *Index) canonicalEdge(source, relation, target string) (CanonicalEdge, bool) {
	relationDef, canonical, reversed, ok := i.resolveRelation(relation)
	if !ok {
		return CanonicalEdge{}, false
	}
	if relationDef.Symmetric {
		from, to := source, target
		if to < from {
			from, to = to, from
		}
		return CanonicalEdge{From: from, Relation: canonical, To: to}, true
	}
	if reversed {
		return CanonicalEdge{From: target, Relation: canonical, To: source}, true
	}
	return CanonicalEdge{From: source, Relation: canonical, To: target}, true
}

func (i *Index) resolveRelation(name string) (schema.RelationDef, string, bool, bool) {
	if i.schema == nil {
		return schema.RelationDef{}, name, false, true
	}
	if relationDef, ok := i.schema.Relations[name]; ok {
		return relationDef, name, false, true
	}
	for canonical, relationDef := range i.schema.Relations {
		if relationDef.Reverse == name {
			return relationDef, canonical, true, true
		}
	}
	return schema.RelationDef{}, "", false, false
}

func (i *Index) relationDef(name string) schema.RelationDef {
	if i.schema == nil {
		return schema.RelationDef{}
	}
	return i.schema.Relations[name]
}

func (i *Index) addReverse(id, relation, target string) {
	if id == "" || target == "" {
		return
	}
	if i.reverse[id] == nil {
		i.reverse[id] = make(map[string][]string)
	}
	if contains(i.reverse[id][relation], target) {
		return
	}
	i.reverse[id][relation] = append(i.reverse[id][relation], target)
	sort.Strings(i.reverse[id][relation])
}

func (i *Index) Get(id string) (*task.Task, error) {
	t, ok := i.tasks[id]
	if !ok {
		return nil, syaerr.NotFound{ID: id}
	}
	return t, nil
}

func (i *Index) Resolve(idOrPrefix string) (*task.Task, error) {
	if t, ok := i.tasks[idOrPrefix]; ok {
		return t, nil
	}
	matches := i.prefixMatches(idOrPrefix)
	switch len(matches) {
	case 0:
		return nil, syaerr.NotFound{ID: idOrPrefix}
	case 1:
		return i.tasks[matches[0]], nil
	default:
		return nil, syaerr.Ambiguous{Prefix: idOrPrefix, Candidates: i.candidates(matches)}
	}
}

func (i *Index) ResolvePrefix(prefix string) (*task.Task, []task.Summary, error) {
	t, err := i.Resolve(prefix)
	if err == nil {
		return t, nil, nil
	}
	matches := i.prefixMatches(prefix)
	if len(matches) == 0 {
		return nil, nil, err
	}
	summaries := make([]task.Summary, 0, len(matches))
	for _, id := range matches {
		summaries = append(summaries, i.tasks[id].Summary())
	}
	return nil, summaries, err
}

func (i *Index) prefixMatches(prefix string) []string {
	var matches []string
	for _, id := range i.order {
		if strings.HasPrefix(id, prefix) {
			matches = append(matches, id)
		}
	}
	sort.Strings(matches)
	return matches
}

func (i *Index) candidates(ids []string) []syaerr.Candidate {
	out := make([]syaerr.Candidate, 0, len(ids))
	for _, id := range ids {
		summary := i.tasks[id].Summary()
		out = append(out, syaerr.Candidate{
			ID:     summary.ID,
			Title:  summary.Title,
			Type:   summary.Type,
			Status: summary.Status,
			File:   summary.File,
		})
	}
	return out
}

func (i *Index) All() []*task.Task {
	return i.Query(Query{})
}

func (i *Index) ByType(taskType string) []*task.Task {
	return i.Query(Query{Types: []string{taskType}})
}

func (i *Index) ByStatus(status string) []*task.Task {
	return i.Query(Query{Statuses: []string{status}})
}

func (i *Index) ByLabel(label string) []*task.Task {
	return i.Query(Query{Labels: []string{label}})
}

func (i *Index) ByParent(parent string) []*task.Task {
	return i.Query(Query{Parents: []string{parent}})
}

func (i *Index) ByAssignee(assignee string) []*task.Task {
	return i.Query(Query{Assignees: []string{assignee}})
}

func (i *Index) Archived(archived bool) []*task.Task {
	return i.Query(Query{Archived: &archived})
}

func (i *Index) Query(q Query) []*task.Task {
	matches := make([]*task.Task, 0, len(i.tasks))
	for _, id := range i.order {
		t := i.tasks[id]
		if !q.matches(t) {
			continue
		}
		matches = append(matches, t)
	}
	sortTasks(matches)
	if q.Limit > 0 && q.Limit < len(matches) {
		matches = matches[:q.Limit]
	}
	return matches
}

func (q Query) matches(t *task.Task) bool {
	if !matchesAny(t.Type, q.Types) {
		return false
	}
	if !matchesAny(t.Status, q.Statuses) {
		return false
	}
	if !matchesAny(t.Parent, q.Parents) {
		return false
	}
	if !matchesAny(t.Assignee, q.Assignees) {
		return false
	}
	if q.Archived != nil && t.Archived != *q.Archived {
		return false
	}
	for _, label := range q.Labels {
		if !contains(t.Labels, label) {
			return false
		}
	}
	return true
}

func (i *Index) ReverseEdges() ReverseEdges {
	out := make(ReverseEdges, len(i.reverse))
	for id, relations := range i.reverse {
		out[id] = make(map[string][]string, len(relations))
		for relation, targets := range relations {
			out[id][relation] = append([]string(nil), targets...)
		}
	}
	return out
}

func (i *Index) Related(id, relation string) []string {
	var out []string
	out = append(out, i.forward[id][relation]...)
	out = append(out, i.reverse[id][relation]...)
	sort.Strings(out)
	return compactStrings(out)
}

func edgeOriginPaths(origins []EdgeOrigin) []string {
	paths := make([]string, 0, len(origins))
	for _, origin := range origins {
		paths = append(paths, origin.Path)
	}
	sort.Strings(paths)
	return compactStrings(paths)
}

func (i *Index) Children(id string) []string {
	return append([]string(nil), i.reverse[id][childrenRelation]...)
}

func (i *Index) Parent(id string) (string, bool) {
	t, ok := i.tasks[id]
	if !ok || t.Parent == "" {
		return "", false
	}
	return t.Parent, true
}

func (i *Index) Quarantined() []QuarantinedFile {
	return append([]QuarantinedFile(nil), i.quarantine...)
}

func (i *Index) Warnings() []Warning {
	return append([]Warning(nil), i.warnings...)
}

func (i *Index) CanonicalOrigins() map[CanonicalEdge][]EdgeOrigin {
	out := make(map[CanonicalEdge][]EdgeOrigin, len(i.edgeOrigins))
	for edge, origins := range i.edgeOrigins {
		out[edge] = append([]EdgeOrigin(nil), origins...)
	}
	return out
}

type emptyFS struct{}

func (*emptyFS) Open(string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

func sortTasks(tasks []*task.Task) {
	sort.Slice(tasks, func(a, b int) bool {
		left, right := tasks[a], tasks[b]
		if priorityRank(left.Priority) != priorityRank(right.Priority) {
			return priorityRank(left.Priority) > priorityRank(right.Priority)
		}
		if !left.Created.Equal(right.Created) {
			return left.Created.Before(right.Created)
		}
		if left.ID != right.ID {
			return left.ID < right.ID
		}
		return left.File < right.File
	})
}

func priorityRank(priority string) int {
	switch strings.ToLower(priority) {
	case "critical":
		return 5
	case "high":
		return 4
	case "normal", "medium", "":
		return 3
	case "low":
		return 2
	case "deferred", "backlog":
		return 1
	default:
		return 0
	}
}

func matchesAny(value string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	return contains(allowed, value)
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func sortWarnings(warnings []Warning) {
	sort.Slice(warnings, func(a, b int) bool {
		left, right := warnings[a], warnings[b]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if strings.Join(left.Paths, "\x00") != strings.Join(right.Paths, "\x00") {
			return strings.Join(left.Paths, "\x00") < strings.Join(right.Paths, "\x00")
		}
		if left.Edge == nil || right.Edge == nil {
			return left.Message < right.Message
		}
		if left.Edge.From != right.Edge.From {
			return left.Edge.From < right.Edge.From
		}
		if left.Edge.Relation != right.Edge.Relation {
			return left.Edge.Relation < right.Edge.Relation
		}
		return left.Edge.To < right.Edge.To
	})
}
