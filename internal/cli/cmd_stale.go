package cli

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts staleOptions
		cmd := app.command("stale", "List stale active tasks", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runStale(opts)
		})
		cmd.Flags().IntVar(&opts.Days, "days", 14, "minimum days since last Log entry")
		cmd.Flags().VarP(&opts.Types, "type", "t", "task type")
		cmd.Flags().IntVar(&opts.Limit, "limit", 0, "maximum number of tasks")
		return cmd
	})
}

type staleOptions struct {
	Days  int
	Types stringList
	Limit int
}

type StaleResult struct {
	Tasks []StaleTask `json:"tasks"`
}

type StaleTask struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	DaysStale int    `json:"days_stale"`
	File      string `json:"file"`
}

func (r StaleResult) HumanText(Colorizer) string {
	if len(r.Tasks) == 0 {
		return "no stale tasks"
	}
	lines := make([]string, 0, len(r.Tasks))
	for _, t := range r.Tasks {
		lines = append(lines, fmt.Sprintf("%s %-10s %-12s %4dd %s", t.ID, t.Type, t.Status, t.DaysStale, t.Title))
	}
	return strings.Join(lines, "\n")
}

func (a *App) runStale(opts staleOptions) (StaleResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return StaleResult{}, err
	}
	if opts.Days <= 0 {
		opts.Days = 14
	}
	now := a.now().UTC()
	cutoff := now.Add(-time.Duration(opts.Days) * 24 * time.Hour)
	var stale []struct {
		task *task.Task
		last time.Time
	}
	for _, t := range activeTasks(state.Index, state.Schema) {
		if len(opts.Types) > 0 && !stringIn(opts.Types, t.Type) {
			continue
		}
		last := lastLogTime(t)
		if last.IsZero() {
			last = t.Created
		}
		if last.IsZero() || last.After(cutoff) || last.Equal(cutoff) {
			continue
		}
		stale = append(stale, struct {
			task *task.Task
			last time.Time
		}{task: t, last: last})
	}
	sort.Slice(stale, func(i, j int) bool {
		if !stale[i].last.Equal(stale[j].last) {
			return stale[i].last.Before(stale[j].last)
		}
		return stale[i].task.ID < stale[j].task.ID
	})
	result := StaleResult{Tasks: make([]StaleTask, 0, len(stale))}
	for _, item := range stale {
		result.Tasks = append(result.Tasks, StaleTask{
			ID:        item.task.ID,
			Type:      item.task.Type,
			Status:    item.task.Status,
			Title:     item.task.Title,
			DaysStale: int(now.Sub(item.last).Hours() / 24),
			File:      item.task.File,
		})
		if opts.Limit > 0 && len(result.Tasks) >= opts.Limit {
			break
		}
	}
	return result, nil
}

var logEntryTimeRE = regexp.MustCompile(`(?m)^- ([0-9]{4}-[0-9]{2}-[0-9]{2}T[^ ]+) @`)

func lastLogTime(t *task.Task) time.Time {
	var last time.Time
	for _, section := range t.Body.Sections {
		if section.Name != "Log" {
			continue
		}
		for _, match := range logEntryTimeRE.FindAllStringSubmatch(sectionText(section), -1) {
			ts, err := time.Parse(time.RFC3339, match[1])
			if err != nil {
				continue
			}
			if ts.After(last) {
				last = ts
			}
		}
	}
	return last
}
