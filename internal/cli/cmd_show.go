package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/memory"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var thread bool
		cmd := app.command("show <id>", "Show a task", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runShow(args[0], thread)
		})
		cmd.Flags().BoolVar(&thread, "thread", false, "show discovered_from ancestor/discovery tree")
		return cmd
	})
}

type ShowResult struct {
	Task       TaskCard                `json:"task"`
	Relations  map[string][]string     `json:"relations,omitempty"`
	Thread     []ThreadItem            `json:"thread,omitempty"`
	Sections   []SectionCard           `json:"sections,omitempty"`
	Memory     []MemorySummary         `json:"memory,omitempty"`
	Quarantine []index.QuarantinedFile `json:"quarantine,omitempty"`
}

type TaskCard struct {
	ID                string         `json:"id"`
	Type              string         `json:"type"`
	Title             string         `json:"title"`
	Status            string         `json:"status"`
	StatusDescription string         `json:"status_description,omitempty"`
	Priority          string         `json:"priority,omitempty"`
	Parent            string         `json:"parent,omitempty"`
	Assignee          string         `json:"assignee,omitempty"`
	Labels            []string       `json:"labels,omitempty"`
	Fields            map[string]any `json:"fields,omitempty"`
	Links             []task.Link    `json:"links,omitempty"`
	Created           string         `json:"created,omitempty"`
	SchemaVersion     int            `json:"schema_version,omitempty"`
	Archived          bool           `json:"archived,omitempty"`
	File              string         `json:"file"`
}

type SectionCard struct {
	Name string `json:"name,omitempty"`
	Text string `json:"text"`
}

type ThreadItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	File      string `json:"file"`
	Depth     int    `json:"depth"`
	Direction string `json:"direction"`
	Current   bool   `json:"current,omitempty"`
}

func (r ShowResult) HumanText(Colorizer) string {
	if len(r.Thread) > 0 {
		return renderThread(r.Thread)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s [%s] %s\n", r.Task.ID, r.Task.Status, r.Task.Title)
	fmt.Fprintf(&b, "type: %s\n", r.Task.Type)
	fmt.Fprintf(&b, "status: %s\n", formatStatusWithDescription(r.Task.Status, r.Task.StatusDescription))
	if r.Task.Priority != "" {
		fmt.Fprintf(&b, "priority: %s\n", r.Task.Priority)
	}
	if r.Task.Parent != "" {
		fmt.Fprintf(&b, "parent: %s\n", r.Task.Parent)
	}
	if len(r.Relations) > 0 {
		b.WriteString("relations:\n")
		for _, relation := range sortedRelationKeys(r.Relations) {
			fmt.Fprintf(&b, "  %s: %s\n", relation, strings.Join(r.Relations[relation], ", "))
		}
	}
	if len(r.Task.Links) > 0 {
		b.WriteString("links:\n")
		for _, link := range r.Task.Links {
			target := link.URL
			if target == "" {
				target = link.Path
			}
			if link.Title != "" {
				fmt.Fprintf(&b, "  - %s: %s\n", link.Title, target)
			} else {
				fmt.Fprintf(&b, "  - %s\n", target)
			}
		}
	}
	for _, section := range r.Sections {
		if section.Name == "" {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n%s\n", section.Name, section.Text)
	}
	if len(r.Memory) > 0 {
		b.WriteString("\n## Memory\n")
		for _, note := range r.Memory {
			fmt.Fprintf(&b, "- %s: %s\n", note.Name, note.Description)
		}
	}
	if len(r.Quarantine) > 0 {
		fmt.Fprintf(&b, "\nwarning: %d quarantined task file(s)\n", len(r.Quarantine))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (a *App) runShow(id string, includeThread bool) (ShowResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return ShowResult{}, err
	}
	t, err := state.Index.Resolve(id)
	if err != nil {
		return ShowResult{}, err
	}
	notes, err := memory.List(os.DirFS(state.Project.Root), ".sya/memory")
	if err != nil {
		return ShowResult{}, err
	}
	relations := relationView(t, state.Index.ReverseEdges()[t.ID])
	result := ShowResult{
		Task:       taskCard(t, statusDescription(state.Schema, t.Type, t.Status)),
		Relations:  relations,
		Sections:   sectionCards(t.Body.Sections),
		Memory:     memoryForTask(notes, t.ID),
		Quarantine: state.Index.Quarantined(),
	}
	if includeThread {
		result.Thread = discoveryThread(state.Index, t)
	}
	return result, nil
}

func taskCard(t *task.Task, statusDescription string) TaskCard {
	return TaskCard{
		ID:                t.ID,
		Type:              t.Type,
		Title:             t.Title,
		Status:            t.Status,
		StatusDescription: statusDescription,
		Priority:          t.Priority,
		Parent:            t.Parent,
		Assignee:          t.Assignee,
		Labels:            append([]string(nil), t.Labels...),
		Fields:            t.Fields,
		Links:             append([]task.Link(nil), t.Links...),
		Created:           t.Created.UTC().Format("2006-01-02T15:04:05Z07:00"),
		SchemaVersion:     t.SchemaVersion,
		Archived:          t.Archived,
		File:              t.File,
	}
}

func discoveryThread(idx *index.Index, t *task.Task) []ThreadItem {
	ancestors := discoveryAncestors(idx, t, nil)
	items := make([]ThreadItem, 0, len(ancestors)+1)
	for i, ancestor := range ancestors {
		items = append(items, threadItem(ancestor, i, "ancestor", false))
	}
	items = append(items, threadItem(t, len(ancestors), "current", true))
	visited := map[string]bool{t.ID: true}
	for _, ancestor := range ancestors {
		visited[ancestor.ID] = true
	}
	items = append(items, discoveryDescendants(idx, t.ID, len(ancestors)+1, visited)...)
	return items
}

func discoveryAncestors(idx *index.Index, t *task.Task, seen map[string]bool) []*task.Task {
	if seen == nil {
		seen = make(map[string]bool)
	}
	if t == nil || seen[t.ID] {
		return nil
	}
	seen[t.ID] = true
	parents := append([]string(nil), t.Relations["discovered_from"]...)
	sort.Strings(parents)
	if len(parents) == 0 {
		return nil
	}
	parent, err := idx.Resolve(parents[0])
	if err != nil {
		return nil
	}
	return append(discoveryAncestors(idx, parent, seen), parent)
}

func discoveryDescendants(idx *index.Index, id string, depth int, seen map[string]bool) []ThreadItem {
	reverse := idx.ReverseEdges()[id]
	children := append([]string(nil), reverse["discovered"]...)
	sort.Strings(children)
	items := make([]ThreadItem, 0, len(children))
	for _, childID := range children {
		if seen[childID] {
			continue
		}
		child, err := idx.Resolve(childID)
		if err != nil {
			continue
		}
		seen[child.ID] = true
		items = append(items, threadItem(child, depth, "discovery", false))
		items = append(items, discoveryDescendants(idx, child.ID, depth+1, seen)...)
	}
	return items
}

func threadItem(t *task.Task, depth int, direction string, current bool) ThreadItem {
	return ThreadItem{
		ID:        t.ID,
		Type:      t.Type,
		Title:     t.Title,
		Status:    t.Status,
		File:      t.File,
		Depth:     depth,
		Direction: direction,
		Current:   current,
	}
}

func renderThread(items []ThreadItem) string {
	lines := make([]string, 0, len(items)+1)
	lines = append(lines, "thread:")
	for _, item := range items {
		marker := "-"
		switch item.Direction {
		case "ancestor":
			marker = "^"
		case "current":
			marker = "*"
		case "discovery":
			marker = "v"
		}
		lines = append(lines, fmt.Sprintf("%s%s %s [%s] %s", strings.Repeat("  ", item.Depth), marker, item.ID, item.Status, item.Title))
	}
	return strings.Join(lines, "\n")
}

func relationView(t *task.Task, reverse map[string][]string) map[string][]string {
	sets := make(map[string]map[string]struct{})
	for relation, ids := range t.Relations {
		if sets[relation] == nil {
			sets[relation] = make(map[string]struct{})
		}
		for _, id := range ids {
			sets[relation][id] = struct{}{}
		}
	}
	for relation, ids := range reverse {
		if sets[relation] == nil {
			sets[relation] = make(map[string]struct{})
		}
		for _, id := range ids {
			sets[relation][id] = struct{}{}
		}
	}
	relations := make(map[string][]string, len(sets))
	for relation, ids := range sets {
		relations[relation] = make([]string, 0, len(ids))
		for id := range ids {
			relations[relation] = append(relations[relation], id)
		}
		sort.Strings(relations[relation])
	}
	if len(relations) == 0 {
		return nil
	}
	return relations
}

func sectionCards(sections []task.Section) []SectionCard {
	cards := make([]SectionCard, 0, len(sections))
	for _, section := range sections {
		cards = append(cards, SectionCard{Name: section.Name, Text: sectionText(section)})
	}
	return cards
}

func memoryForTask(notes []memory.Note, id string) []MemorySummary {
	out := make([]MemorySummary, 0)
	for _, note := range notes {
		if !stringIn(note.Tasks, id) {
			continue
		}
		out = append(out, memorySummary(note))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}
