package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/snjax/sya/internal/fsutil"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts roadmapOptions
		cmd := app.command("roadmap", "Render project roadmap", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runRoadmap(opts)
		})
		cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "write markdown roadmap to file")
		return cmd
	})
}

type roadmapOptions struct {
	Output string
}

type RoadmapResult struct {
	Project       string         `json:"project"`
	Epics         []RoadmapEpic  `json:"epics"`
	Groups        []RoadmapGroup `json:"groups"`
	ArchivedCount int            `json:"archived_count"`
	Markdown      string         `json:"markdown,omitempty"`
	Written       string         `json:"written,omitempty"`
}

type RoadmapEpic struct {
	Task        TaskSummary   `json:"task"`
	Description string        `json:"description,omitempty"`
	Closed      int           `json:"closed"`
	Total       int           `json:"total"`
	Bar         string        `json:"bar"`
	Children    []RoadmapItem `json:"children,omitempty"`
}

type RoadmapGroup struct {
	Type  string        `json:"type"`
	Tasks []RoadmapItem `json:"tasks"`
}

type RoadmapItem struct {
	Task          TaskSummary `json:"task"`
	Icon          string      `json:"icon"`
	Blocked       bool        `json:"blocked,omitempty"`
	BlockedReason string      `json:"blocked_reason,omitempty"`
}

func (r RoadmapResult) HumanText(Colorizer) string {
	if r.Written != "" {
		return "wrote " + r.Written
	}
	return r.Markdown
}

func (a *App) runRoadmap(opts roadmapOptions) (RoadmapResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return RoadmapResult{}, err
	}
	cfg, err := loadConfig(state.Project)
	if err != nil {
		return RoadmapResult{}, err
	}
	result := buildRoadmap(state, cfg.Project)
	result.Markdown = renderRoadmap(result)
	if opts.Output != "" {
		path := opts.Output
		if !filepath.IsAbs(path) {
			path = filepath.Join(state.Project.Root, path)
		}
		if err := fsutil.AtomicWriteFile(path, []byte(result.Markdown+"\n"), 0o644); err != nil {
			return RoadmapResult{}, err
		}
		result.Written = opts.Output
	}
	return result, nil
}

func buildRoadmap(state *projectState, projectName string) RoadmapResult {
	result := RoadmapResult{Project: projectName}
	for _, t := range state.Index.All() {
		if t.Archived {
			result.ArchivedCount++
			continue
		}
		if t.Type == "epic" {
			result.Epics = append(result.Epics, roadmapEpic(state, t))
		}
	}
	sort.Slice(result.Epics, func(i, j int) bool {
		leftClosed := taskClosed(state.Schema, taskFromSummary(result.Epics[i].Task))
		rightClosed := taskClosed(state.Schema, taskFromSummary(result.Epics[j].Task))
		if leftClosed != rightClosed {
			return !leftClosed
		}
		leftOrder := statusOrder(state.Schema, result.Epics[i].Task.Type, result.Epics[i].Task.Status)
		rightOrder := statusOrder(state.Schema, result.Epics[j].Task.Type, result.Epics[j].Task.Status)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if result.Epics[i].Task.Title != result.Epics[j].Task.Title {
			return result.Epics[i].Task.Title < result.Epics[j].Task.Title
		}
		return result.Epics[i].Task.ID < result.Epics[j].Task.ID
	})

	groups := make(map[string][]RoadmapItem)
	for _, t := range state.Index.All() {
		if t.Archived || t.Type == "epic" || t.Parent != "" {
			continue
		}
		groups[t.Type] = append(groups[t.Type], roadmapItem(state, t))
	}
	types := make([]string, 0, len(groups))
	for typ := range groups {
		types = append(types, typ)
		sortRoadmapItems(state.Schema, typ, groups[typ])
	}
	sort.Strings(types)
	for _, typ := range types {
		result.Groups = append(result.Groups, RoadmapGroup{Type: typ, Tasks: groups[typ]})
	}
	return result
}

func roadmapEpic(state *projectState, t *task.Task) RoadmapEpic {
	closed, total := roadmapProgress(state, t.ID, make(map[string]bool))
	epic := RoadmapEpic{
		Task:        summarizeTask(t),
		Description: firstDescriptionLine(t),
		Closed:      closed,
		Total:       total,
		Bar:         progressBar(closed, total),
	}
	for _, childID := range state.Index.Children(t.ID) {
		child, err := state.Index.Resolve(childID)
		if err != nil || child.Archived {
			continue
		}
		epic.Children = append(epic.Children, roadmapItem(state, child))
	}
	sortRoadmapItems(state.Schema, "", epic.Children)
	return epic
}

func roadmapProgress(state *projectState, id string, seen map[string]bool) (int, int) {
	if seen[id] {
		return 0, 0
	}
	seen[id] = true
	t, err := state.Index.Resolve(id)
	if err != nil || t.Archived {
		return 0, 0
	}
	closed, total := 0, 1
	if taskClosed(state.Schema, t) {
		closed = 1
	}
	for _, childID := range state.Index.Children(id) {
		childClosed, childTotal := roadmapProgress(state, childID, seen)
		closed += childClosed
		total += childTotal
	}
	return closed, total
}

func roadmapItem(state *projectState, t *task.Task) RoadmapItem {
	blocked := roadmapBlocked(state, t)
	return RoadmapItem{
		Task:          summarizeTask(t),
		Icon:          roadmapIcon(state.Schema, t, blocked != ""),
		Blocked:       blocked != "",
		BlockedReason: blocked,
	}
}

func roadmapBlocked(state *projectState, t *task.Task) string {
	view, ok := state.Index.Resolver().Get(t.ID)
	if !ok {
		return ""
	}
	blocked := schema.Blocked(state.Schema, state.Index.Resolver(), view)
	if !blocked.Blocked {
		return ""
	}
	if blocked.DeadEnd {
		return "dead-end"
	}
	for _, transition := range blocked.Transitions {
		if transition.Passing {
			continue
		}
		for _, violation := range transition.Violations {
			if violation.Message != "" {
				return violation.Message
			}
		}
	}
	return "blocked"
}

func roadmapIcon(sch *schema.Schema, t *task.Task, blocked bool) string {
	typeDef, ok := sch.Types[t.Type]
	if ok && stringIn(typeDef.Terminal, t.Status) {
		return "✓"
	}
	if blocked {
		return "⛔"
	}
	if ok && stringIn(typeDef.Working, t.Status) {
		return "▸"
	}
	if ok && stringIn(typeDef.Parked, t.Status) {
		return "◇"
	}
	return "·"
}

func renderRoadmap(result RoadmapResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", result.Project)
	for _, epic := range result.Epics {
		fmt.Fprintf(&b, "\n## %s (%d/%d closed) %s\n", epic.Task.Title, epic.Closed, epic.Total, epic.Bar)
		if epic.Description != "" {
			fmt.Fprintf(&b, "%s\n", epic.Description)
		}
		if len(epic.Children) == 0 {
			b.WriteString("\n- · no children\n")
			continue
		}
		b.WriteByte('\n')
		for _, child := range epic.Children {
			writeRoadmapItem(&b, child)
		}
	}
	if len(result.Groups) > 0 {
		b.WriteString("\n## Top-level tasks\n")
		for _, group := range result.Groups {
			fmt.Fprintf(&b, "\n### %s\n", group.Type)
			for _, item := range group.Tasks {
				writeRoadmapItem(&b, item)
			}
		}
	}
	fmt.Fprintf(&b, "\nArchived tasks: %d", result.ArchivedCount)
	return b.String()
}

func writeRoadmapItem(b *strings.Builder, item RoadmapItem) {
	fmt.Fprintf(b, "- %s %s [%s] %s", item.Icon, item.Task.ID, item.Task.Status, item.Task.Title)
	if item.Task.Assignee != "" {
		fmt.Fprintf(b, " @%s", item.Task.Assignee)
	}
	if item.BlockedReason != "" {
		fmt.Fprintf(b, " — blocked: %s", item.BlockedReason)
	}
	b.WriteByte('\n')
}

func sortRoadmapItems(sch *schema.Schema, typ string, items []RoadmapItem) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i].Task
		right := items[j].Task
		leftType, rightType := typ, typ
		if leftType == "" {
			leftType = left.Type
		}
		if rightType == "" {
			rightType = right.Type
		}
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		leftOrder := statusOrder(sch, leftType, left.Status)
		rightOrder := statusOrder(sch, rightType, right.Status)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if left.Title != right.Title {
			return left.Title < right.Title
		}
		return left.ID < right.ID
	})
}

func statusOrder(sch *schema.Schema, typeName, status string) int {
	typeDef, ok := sch.Types[typeName]
	if !ok {
		return 1 << 20
	}
	for i, pipelineStatus := range typeDef.Pipeline {
		if pipelineStatus == status {
			return i
		}
	}
	return len(typeDef.Pipeline) + 1
}

func firstDescriptionLine(t *task.Task) string {
	for _, section := range t.Body.Sections {
		if section.Name != "Description" {
			continue
		}
		for _, line := range strings.Split(sectionText(section), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				return line
			}
		}
	}
	return ""
}

func progressBar(closed, total int) string {
	if total <= 0 {
		return "[----------]"
	}
	done := closed * 10 / total
	if done > 10 {
		done = 10
	}
	return "[" + strings.Repeat("█", done) + strings.Repeat("░", 10-done) + "]"
}

func taskFromSummary(summary TaskSummary) *task.Task {
	return &task.Task{ID: summary.ID, Type: summary.Type, Title: summary.Title, Status: summary.Status}
}
