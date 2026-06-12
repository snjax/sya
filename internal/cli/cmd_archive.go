package cli

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var auto bool
		cmd := app.command("archive <id>...", "Mark terminal tasks archived", nil, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			if auto && len(args) > 0 {
				return nil, syaerr.Usage{Message: "archive accepts either ids or --auto"}
			}
			if !auto && len(args) == 0 {
				return nil, syaerr.Usage{Message: "archive requires ids or --auto"}
			}
			return app.withProjectMutationLock(func() (any, error) {
				return app.runArchive(args, auto)
			})
		})
		cmd.Flags().BoolVar(&auto, "auto", false, "archive terminal tasks older than config archive.after_days")
		return cmd
	})
}

func (a *App) runArchive(ids []string, auto bool) (MutationResults, error) {
	state, err := a.loadProject()
	if err != nil {
		return MutationResults{}, err
	}
	if auto {
		ids, err = a.autoArchiveIDs(state)
		if err != nil {
			return MutationResults{}, err
		}
	}
	selected := make([]*task.Task, 0, len(ids))
	var offenders []string
	for _, id := range ids {
		t, err := state.Index.Resolve(id)
		if err != nil {
			return MutationResults{}, err
		}
		if !isTerminalTask(state, t) {
			offenders = append(offenders, fmt.Sprintf("%s(%s)", t.ID, t.Status))
			continue
		}
		selected = append(selected, t)
	}
	if len(offenders) > 0 {
		return MutationResults{}, syaerr.Usage{Message: "cannot archive non-terminal tasks: " + strings.Join(offenders, ", ")}
	}

	results := MutationResults{Results: make([]MutationResult, 0, len(selected))}
	for _, t := range selected {
		if !t.Archived {
			t.Archived = true
			if err := appendTaskLog(t, a.now(), a.Actor(), "archived"); err != nil {
				return MutationResults{}, err
			}
			if err := writeTask(state, t); err != nil {
				return MutationResults{}, err
			}
		}
		results.Results = append(results.Results, MutationResult{ID: t.ID, File: t.File, Status: t.Status, OK: true})
	}
	return results, nil
}

func (a *App) autoArchiveIDs(state *projectState) ([]string, error) {
	cfg, err := loadConfig(state.Project)
	if err != nil {
		return nil, err
	}
	afterDays := cfg.Archive.AfterDays
	if afterDays <= 0 {
		afterDays = 30
	}
	cutoff := a.now().UTC().Add(-time.Duration(afterDays) * 24 * time.Hour)
	var ids []string
	for _, t := range state.Index.Query(index.Query{Archived: boolPtr(false)}) {
		if !isTerminalTask(state, t) {
			continue
		}
		basis := terminalEnteredAt(state, t)
		if basis.IsZero() {
			basis = t.Created
		}
		if !basis.IsZero() && basis.Before(cutoff) {
			ids = append(ids, t.ID)
		}
	}
	return ids, nil
}

var logTransitionRE = regexp.MustCompile(`^- ([0-9TZ:\-]+) @[^:]+: .* -> ([^ :↩]+)`)

func terminalEnteredAt(state *projectState, t *task.Task) time.Time {
	typeDef := state.Schema.Types[t.Type]
	terminal := make(map[string]bool, len(typeDef.Terminal))
	for _, status := range typeDef.Terminal {
		terminal[status] = true
	}
	var last time.Time
	for _, section := range t.Body.Sections {
		if section.Name != "Log" {
			continue
		}
		for _, line := range strings.Split(sectionText(section), "\n") {
			match := logTransitionRE.FindStringSubmatch(strings.TrimSpace(line))
			if len(match) != 3 || !terminal[match[2]] {
				continue
			}
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

func isTerminalTask(state *projectState, t *task.Task) bool {
	if state == nil || state.Schema == nil || t == nil {
		return false
	}
	typeDef, ok := state.Schema.Types[t.Type]
	return ok && stringIn(typeDef.Terminal, t.Status)
}
