package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts searchOptions
		cmd := app.command("search \"query\"", "Search tasks", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			opts.Query = args[0]
			return app.runSearch(opts)
		})
		cmd.Flags().VarP(&opts.Types, "type", "t", "task type")
		cmd.Flags().IntVar(&opts.Limit, "limit", 0, "maximum number of results")
		return cmd
	})
}

type searchOptions struct {
	Query string
	Types stringList
	Limit int
}

type SearchResult struct {
	Results []SearchHit `json:"results"`
}

type SearchHit struct {
	Task  TaskSummary `json:"task"`
	Match string      `json:"match"`
}

func (r SearchResult) HumanText(Colorizer) string {
	if len(r.Results) == 0 {
		return "no matches"
	}
	lines := make([]string, 0, len(r.Results))
	for _, hit := range r.Results {
		lines = append(lines, fmt.Sprintf("%s %-10s %-12s %-5s %s", hit.Task.ID, hit.Task.Type, hit.Task.Status, hit.Match, hit.Task.Title))
	}
	return strings.Join(lines, "\n")
}

func (a *App) runSearch(opts searchOptions) (SearchResult, error) {
	query := strings.ToLower(strings.TrimSpace(opts.Query))
	if query == "" {
		return SearchResult{}, syaerr.Usage{Message: "search query is required"}
	}
	state, err := a.loadProject()
	if err != nil {
		return SearchResult{}, err
	}
	tasks := state.Index.Query(index.Query{Types: opts.Types, Archived: boolPtr(false)})
	titleHits := make([]SearchHit, 0)
	bodyHits := make([]SearchHit, 0)
	for _, t := range tasks {
		switch taskSearchMatch(t, query) {
		case "title":
			titleHits = append(titleHits, SearchHit{Task: summarizeTask(t), Match: "title"})
		case "body":
			bodyHits = append(bodyHits, SearchHit{Task: summarizeTask(t), Match: "body"})
		}
	}
	result := SearchResult{Results: append(titleHits, bodyHits...)}
	if opts.Limit > 0 && opts.Limit < len(result.Results) {
		result.Results = result.Results[:opts.Limit]
	}
	return result, nil
}

func taskSearchMatch(t *task.Task, query string) string {
	if strings.Contains(strings.ToLower(t.Title), query) {
		return "title"
	}
	for _, section := range t.Body.Sections {
		if strings.Contains(strings.ToLower(sectionText(section)), query) {
			return "body"
		}
	}
	return ""
}
