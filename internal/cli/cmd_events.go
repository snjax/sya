package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/snjax/sya/internal/events"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts eventsOptions
		cmd := app.command("events", "Show transition attempt journal", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runEvents(opts)
		})
		cmd.Flags().BoolVar(&opts.DeniedOnly, "denied", false, "show denied transitions only")
		cmd.Flags().StringVar(&opts.Task, "task", "", "filter by task id")
		cmd.Flags().StringVar(&opts.Since, "since", "", "filter by RFC3339 timestamp or duration")
		cmd.Flags().IntVar(&opts.Limit, "limit", 0, "maximum events to show")
		return cmd
	})
}

type eventsOptions struct {
	DeniedOnly bool
	Task       string
	Since      string
	Limit      int
}

type EventsResult struct {
	Events []events.Event `json:"events"`
}

func (r EventsResult) HumanText(Colorizer) string {
	if len(r.Events) == 0 {
		return "events: none"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-20s %-10s %-8s %-10s %-10s %-8s %s\n", "ts", "actor", "task", "from", "to", "result", "error")
	for _, event := range r.Events {
		fmt.Fprintf(&b, "%-20s %-10s %-8s %-10s %-10s %-8s %s\n",
			event.TS.UTC().Format(time.RFC3339),
			event.Actor,
			event.Task,
			event.From,
			event.To,
			event.Result,
			event.ErrorType,
		)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (a *App) runEvents(opts eventsOptions) (EventsResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return EventsResult{}, err
	}
	since, err := a.parseSince(opts.Since)
	if err != nil {
		return EventsResult{}, err
	}
	if opts.Limit < 0 {
		return EventsResult{}, syaerr.Usage{Message: "--limit must be non-negative"}
	}
	read, err := events.Read(state.Project.Root, events.Filters{
		DeniedOnly: opts.DeniedOnly,
		Task:       opts.Task,
		Since:      since,
		Limit:      opts.Limit,
	})
	if err != nil {
		return EventsResult{}, err
	}
	return EventsResult{Events: read}, nil
}

func (a *App) parseSince(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts, nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return time.Time{}, syaerr.Usage{Message: "--since must be RFC3339 or duration"}
	}
	return a.now().UTC().Add(-duration), nil
}
