package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/task"
	"github.com/snjax/sya/internal/textsim"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts duplicateOptions
		cmd := app.command("duplicates", "Find similar tasks", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runDuplicates(opts)
		})
		cmd.Flags().Float64Var(&opts.Threshold, "threshold", 0.6, "minimum similarity score")
		cmd.Flags().BoolVar(&opts.All, "all", false, "include terminal and archived tasks")
		cmd.Flags().IntVar(&opts.Limit, "limit", 0, "maximum number of pairs")
		return cmd
	})
}

type duplicateOptions struct {
	Threshold float64
	All       bool
	Limit     int
}

type DuplicatesResult struct {
	Pairs []DuplicatePair `json:"pairs"`
}

type DuplicatePair struct {
	A     TaskSummary `json:"a"`
	B     TaskSummary `json:"b"`
	Score float64     `json:"score"`
	Hint  string      `json:"hint"`
}

func (r DuplicatesResult) HumanText(Colorizer) string {
	if len(r.Pairs) == 0 {
		return "no duplicates"
	}
	lines := make([]string, 0, len(r.Pairs))
	for _, pair := range r.Pairs {
		lines = append(lines, fmt.Sprintf("%.3f  %s  %s  %s", pair.Score, pair.A.ID, pair.B.ID, pair.Hint))
	}
	return strings.Join(lines, "\n")
}

func (a *App) runDuplicates(opts duplicateOptions) (DuplicatesResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return DuplicatesResult{}, err
	}
	tasks := duplicateCandidateTasks(state, opts.All)
	docs := make([]textsim.Doc, 0, len(tasks))
	summaries := make(map[string]TaskSummary, len(tasks))
	for _, t := range tasks {
		docText := t.Title + "\n" + taskDescriptionText(t)
		docs = append(docs, textsim.Doc{ID: t.ID, Text: docText})
		summaries[t.ID] = summarizeTask(t)
	}
	similar := textsim.Similar(docs, opts.Threshold)
	if opts.Limit > 0 && len(similar) > opts.Limit {
		similar = similar[:opts.Limit]
	}
	result := DuplicatesResult{Pairs: make([]DuplicatePair, 0, len(similar))}
	for _, pair := range similar {
		result.Pairs = append(result.Pairs, DuplicatePair{
			A:     summaries[pair.A],
			B:     summaries[pair.B],
			Score: pair.Score,
			Hint:  fmt.Sprintf("sya link %s duplicates %s", pair.A, pair.B),
		})
	}
	return result, nil
}

func duplicateCandidateTasks(state *projectState, all bool) []*task.Task {
	if all {
		return state.Index.All()
	}
	archived := false
	candidates := state.Index.Query(index.Query{Archived: &archived})
	out := candidates[:0]
	for _, t := range candidates {
		typeDef, ok := state.Schema.Types[t.Type]
		if ok && stringIn(typeDef.Terminal, t.Status) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func taskDescriptionText(t *task.Task) string {
	for _, section := range t.Body.Sections {
		if section.Name == "Description" {
			return sectionText(section)
		}
	}
	return ""
}
