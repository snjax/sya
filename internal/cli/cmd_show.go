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
		return app.command("show <id>", "Show a task", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runShow(args[0])
		})
	})
}

type ShowResult struct {
	Task       TaskCard                `json:"task"`
	Relations  map[string][]string     `json:"relations,omitempty"`
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
	Created           string         `json:"created,omitempty"`
	SchemaVersion     int            `json:"schema_version,omitempty"`
	Archived          bool           `json:"archived,omitempty"`
	File              string         `json:"file"`
}

type SectionCard struct {
	Name string `json:"name,omitempty"`
	Text string `json:"text"`
}

func (r ShowResult) HumanText(Colorizer) string {
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

func (a *App) runShow(id string) (ShowResult, error) {
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
	return ShowResult{
		Task:       taskCard(t, statusDescription(state.Schema, t.Type, t.Status)),
		Relations:  relations,
		Sections:   sectionCards(t.Body.Sections),
		Memory:     memoryForTask(notes, t.ID),
		Quarantine: state.Index.Quarantined(),
	}, nil
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
		Created:           t.Created.UTC().Format("2006-01-02T15:04:05Z07:00"),
		SchemaVersion:     t.SchemaVersion,
		Archived:          t.Archived,
		File:              t.File,
	}
}

func relationView(t *task.Task, reverse map[string][]string) map[string][]string {
	relations := make(map[string][]string)
	for relation, ids := range t.Relations {
		relations[relation] = append([]string(nil), ids...)
		sort.Strings(relations[relation])
	}
	for relation, ids := range reverse {
		relations[relation] = append(relations[relation], ids...)
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
