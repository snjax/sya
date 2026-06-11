package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/memory"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		return app.command("prime", "Print agent context", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runPrime()
		})
	})
}

type PrimeResult struct {
	Project    PrimeProject            `json:"project"`
	Schema     PrimeSchema             `json:"schema"`
	Ready      []TaskSummary           `json:"ready"`
	InProgress []TaskSummary           `json:"in_progress"`
	Memory     []MemorySummary         `json:"memory"`
	Warnings   []index.Warning         `json:"warnings,omitempty"`
	Quarantine []index.QuarantinedFile `json:"quarantine,omitempty"`
}

type PrimeProject struct {
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
	Root   string `json:"root,omitempty"`
}

type PrimeSchema struct {
	Types     []PrimeType     `json:"types"`
	Relations []PrimeRelation `json:"relations"`
}

type PrimeType struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Pipeline    []string `json:"pipeline"`
}

type PrimeRelation struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Reverse     string `json:"reverse,omitempty"`
	Symmetric   bool   `json:"symmetric,omitempty"`
	Graph       string `json:"graph,omitempty"`
	Blocking    bool   `json:"blocking,omitempty"`
}

func (r PrimeResult) HumanText(Colorizer) string {
	var b strings.Builder
	fmt.Fprintf(&b, "project: %s", r.Project.Name)
	if r.Project.Prefix != "" {
		fmt.Fprintf(&b, " (%s)", r.Project.Prefix)
	}
	b.WriteString("\n\nschema:\n")
	b.WriteString("(* = working/in-progress, ! = terminal)\n")
	for _, typ := range r.Schema.Types {
		fmt.Fprintf(&b, "- %s", typ.Name)
		if typ.Description != "" {
			fmt.Fprintf(&b, ": %s", typ.Description)
		}
		if len(typ.Pipeline) > 0 {
			fmt.Fprintf(&b, " | %s", strings.Join(typ.Pipeline, " -> "))
		}
		b.WriteByte('\n')
	}
	if len(r.Schema.Relations) > 0 {
		b.WriteString("\nrelations:\n")
		for _, relation := range r.Schema.Relations {
			parts := []string{relation.Name}
			if relation.Reverse != "" {
				parts = append(parts, "reverse "+relation.Reverse)
			}
			if relation.Symmetric {
				parts = append(parts, "symmetric")
			}
			if relation.Graph != "" {
				parts = append(parts, relation.Graph)
			}
			if relation.Blocking {
				parts = append(parts, "blocking")
			}
			line := strings.Join(parts, " | ")
			if relation.Description != "" {
				line += ": " + relation.Description
			}
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}
	b.WriteString("\nready:\n")
	if len(r.Ready) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, t := range r.Ready {
			fmt.Fprintf(&b, "- %s [%s/%s/%s] %s\n", t.ID, t.Type, t.Status, t.Priority, t.Title)
		}
	}
	b.WriteString("\nin-progress:\n")
	if len(r.InProgress) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, t := range r.InProgress {
			assignee := t.Assignee
			if assignee == "" {
				assignee = "unassigned"
			}
			fmt.Fprintf(&b, "- %s [%s/%s] @%s %s\n", t.ID, t.Type, t.Status, assignee, t.Title)
		}
	}
	b.WriteString("\nmemory:\n")
	if len(r.Memory) == 0 {
		b.WriteString("- none")
	} else {
		for _, note := range r.Memory {
			fmt.Fprintf(&b, "- %s: %s\n", note.Name, note.Description)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (a *App) runPrime() (any, error) {
	project, err := a.DiscoverProject()
	if err != nil {
		var notFound syaerr.NotFound
		if errors.As(err, &notFound) && notFound.ID == ".sya" {
			return silent, nil
		}
		return nil, err
	}
	state, err := a.loadProject()
	if err != nil {
		return nil, err
	}
	cfg, err := loadConfig(project)
	if err != nil {
		return nil, err
	}
	notes, err := memory.List(os.DirFS(project.Root), ".sya/memory")
	if err != nil {
		return nil, err
	}
	return PrimeResult{
		Project:    PrimeProject{Name: cfg.Project, Prefix: cfg.Prefix, Root: project.Root},
		Schema:     primeSchema(state.Schema),
		Ready:      primeReady(state, 10),
		InProgress: primeInProgress(state.Index, state.Schema),
		Memory:     primeMemory(notes),
		Warnings:   state.Index.Warnings(),
		Quarantine: state.Index.Quarantined(),
	}, nil
}

func primeSchema(sch *schema.Schema) PrimeSchema {
	var result PrimeSchema
	typeNames := make([]string, 0, len(sch.Types))
	for name := range sch.Types {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)
	for _, name := range typeNames {
		typeDef := sch.Types[name]
		result.Types = append(result.Types, PrimeType{
			Name:        name,
			Description: oneLine(typeDef.Description),
			Pipeline:    markedPipeline(typeDef),
		})
	}
	relationNames := make([]string, 0, len(sch.Relations))
	for name := range sch.Relations {
		relationNames = append(relationNames, name)
	}
	sort.Strings(relationNames)
	for _, name := range relationNames {
		relation := sch.Relations[name]
		result.Relations = append(result.Relations, PrimeRelation{
			Name:        name,
			Description: oneLine(relation.Description),
			Reverse:     relation.Reverse,
			Symmetric:   relation.Symmetric,
			Graph:       relation.Graph,
			Blocking:    relation.Blocking,
		})
	}
	return result
}

func markedPipeline(typeDef schema.TypeDef) []string {
	out := make([]string, 0, len(typeDef.Pipeline))
	for _, status := range typeDef.Pipeline {
		marker := status
		if stringIn(typeDef.Working, status) {
			marker += "*"
		}
		if stringIn(typeDef.Terminal, status) {
			marker += "!"
		}
		out = append(out, marker)
	}
	return out
}

func primeReady(state *projectState, limit int) []TaskSummary {
	resolver := state.Index.Resolver()
	tasks := state.Index.Query(index.Query{Archived: boolPtr(false)})
	result := make([]TaskSummary, 0, limit)
	for _, t := range tasks {
		view, ok := resolver.Get(t.ID)
		if !ok || !schema.Ready(state.Schema, resolver, view) {
			continue
		}
		result = append(result, summarizeTask(t))
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

func primeInProgress(idx *index.Index, sch *schema.Schema) []TaskSummary {
	var statuses []string
	for _, typeDef := range sch.Types {
		statuses = append(statuses, typeDef.Working...)
	}
	statuses = compactStringSlice(sortedStrings(statuses))
	tasks := idx.Query(index.Query{Statuses: statuses, Archived: boolPtr(false)})
	out := make([]TaskSummary, 0, len(tasks))
	for _, t := range tasks {
		typeDef := sch.Types[t.Type]
		if !stringIn(typeDef.Working, t.Status) {
			continue
		}
		out = append(out, summarizeTask(t))
	}
	return out
}

func primeMemory(notes []memory.Note) []MemorySummary {
	out := make([]MemorySummary, 0, len(notes))
	for _, note := range notes {
		out = append(out, memorySummary(note))
	}
	return out
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func boolPtr(value bool) *bool {
	return &value
}
