package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		return app.command("stats", "Show project statistics", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runStats()
		})
	})
}

type StatsResult struct {
	Types      []TypeStats `json:"types"`
	Totals     StatsTotals `json:"totals"`
	Quarantine int         `json:"quarantined"`
	Wisps      int         `json:"wisps"`
	Age        AgeStats    `json:"age"`
}

type TypeStats struct {
	Type     string        `json:"type"`
	Statuses []StatusCount `json:"statuses"`
	Total    int           `json:"total"`
}

type StatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type StatsTotals struct {
	Active   int `json:"active"`
	Terminal int `json:"terminal"`
	Archived int `json:"archived"`
	Ready    int `json:"ready"`
	Blocked  int `json:"blocked"`
	DeadEnd  int `json:"dead_end"`
	Tasks    int `json:"tasks"`
}

type AgeStats struct {
	ActiveAverageDays float64 `json:"active_average_days"`
	ActiveMaxDays     int     `json:"active_max_days"`
}

func (r StatsResult) HumanText(Colorizer) string {
	var b strings.Builder
	b.WriteString("type       status       count\n")
	for _, typ := range r.Types {
		for _, status := range typ.Statuses {
			fmt.Fprintf(&b, "%-10s %-12s %d\n", typ.Type, status.Status, status.Count)
		}
	}
	fmt.Fprintf(&b, "\ntotals: tasks=%d active=%d terminal=%d archived=%d ready=%d blocked=%d dead_end=%d quarantined=%d wisps=%d\n",
		r.Totals.Tasks,
		r.Totals.Active,
		r.Totals.Terminal,
		r.Totals.Archived,
		r.Totals.Ready,
		r.Totals.Blocked,
		r.Totals.DeadEnd,
		r.Quarantine,
		r.Wisps,
	)
	fmt.Fprintf(&b, "active age: avg=%.1fd max=%dd", r.Age.ActiveAverageDays, r.Age.ActiveMaxDays)
	return strings.TrimRight(b.String(), "\n")
}

func (a *App) runStats() (StatsResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return StatsResult{}, err
	}
	result := StatsResult{Quarantine: len(state.Index.Quarantined())}
	wisps, err := loadWisps(state.Project)
	if err != nil {
		return StatsResult{}, err
	}
	result.Wisps = len(wisps)
	counts := make(map[string]map[string]int)
	now := a.now().UTC()
	var activeAgeTotal time.Duration
	var activeAgeMax time.Duration
	var activeAgeCount int
	resolver := state.Index.Resolver()
	for _, t := range state.Index.All() {
		result.Totals.Tasks++
		if counts[t.Type] == nil {
			counts[t.Type] = make(map[string]int)
		}
		counts[t.Type][t.Status]++
		if t.Archived {
			result.Totals.Archived++
		}
		typeDef, ok := state.Schema.Types[t.Type]
		terminal := ok && stringIn(typeDef.Terminal, t.Status)
		if terminal {
			result.Totals.Terminal++
		}
		active := !t.Archived && !terminal
		if active {
			result.Totals.Active++
			if !t.Created.IsZero() && now.After(t.Created) {
				age := now.Sub(t.Created)
				activeAgeTotal += age
				activeAgeCount++
				if age > activeAgeMax {
					activeAgeMax = age
				}
			}
		}
		view, ok := resolver.Get(t.ID)
		if !ok || t.Archived {
			continue
		}
		if schema.Ready(state.Schema, resolver, view) {
			result.Totals.Ready++
		}
		blocked := schema.Blocked(state.Schema, resolver, view)
		if blocked.Blocked {
			result.Totals.Blocked++
		}
		if blocked.DeadEnd {
			result.Totals.DeadEnd++
		}
	}
	for _, typeName := range sortedTypeNames(state.Schema.Types) {
		typeDef := state.Schema.Types[typeName]
		typeStats := TypeStats{Type: typeName}
		for _, status := range typeDef.Pipeline {
			count := counts[typeName][status]
			typeStats.Statuses = append(typeStats.Statuses, StatusCount{Status: status, Count: count})
			typeStats.Total += count
		}
		result.Types = append(result.Types, typeStats)
	}
	if activeAgeCount > 0 {
		result.Age.ActiveAverageDays = activeAgeTotal.Hours() / 24 / float64(activeAgeCount)
		result.Age.ActiveMaxDays = int(activeAgeMax.Hours() / 24)
	}
	return result, nil
}

func sortedTypeNames(types map[string]schema.TypeDef) []string {
	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func activeTasks(idx *index.Index, sch *schema.Schema) []*task.Task {
	var out []*task.Task
	for _, t := range idx.All() {
		if t.Archived {
			continue
		}
		typeDef, ok := sch.Types[t.Type]
		if ok && stringIn(typeDef.Terminal, t.Status) {
			continue
		}
		out = append(out, t)
	}
	return out
}
