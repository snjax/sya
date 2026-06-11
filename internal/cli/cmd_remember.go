package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/snjax/sya/internal/memory"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts rememberOptions
		cmd := app.command("remember \"fact\"", "Store project memory", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			opts.Fact = args[0]
			return app.withProjectMutationLock(func() (any, error) {
				return app.runRemember(opts)
			})
		})
		cmd.Flags().StringVar(&opts.Key, "key", "", "memory key")
		cmd.Flags().Var(&opts.Tasks, "task", "related task id")
		return cmd
	})
	registerCommand(func(app *App) *cobra.Command {
		return app.command("recall [key]", "Read project memory", cobra.MaximumNArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			key := ""
			if len(args) > 0 {
				key = args[0]
			}
			return app.runRecall(key)
		})
	})
	registerCommand(func(app *App) *cobra.Command {
		return app.command("forget <key>", "Delete project memory", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.withProjectMutationLock(func() (any, error) {
				return app.runForget(args[0])
			})
		})
	})
}

type rememberOptions struct {
	Fact  string
	Key   string
	Tasks stringList
}

type MemorySummary struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tasks       []string `json:"tasks,omitempty"`
	File        string   `json:"file,omitempty"`
}

type MemoryWarning struct {
	Task    string `json:"task"`
	Message string `json:"message"`
}

type RememberResult struct {
	Note     MemorySummary   `json:"note"`
	Warnings []MemoryWarning `json:"warnings,omitempty"`
}

type RecallListResult struct {
	Notes []MemorySummary `json:"notes"`
}

type RecallNoteResult struct {
	Note MemorySummary `json:"note"`
	Body string        `json:"body"`
}

type ForgetResult struct {
	Name string `json:"name"`
}

func (r RememberResult) HumanText(Colorizer) string {
	lines := []string{fmt.Sprintf("remembered %s", r.Note.Name)}
	for _, warning := range r.Warnings {
		lines = append(lines, "warning: "+warning.Message)
	}
	return strings.Join(lines, "\n")
}

func (r RecallListResult) HumanText(Colorizer) string {
	if len(r.Notes) == 0 {
		return "no memory"
	}
	lines := make([]string, 0, len(r.Notes))
	for _, note := range r.Notes {
		lines = append(lines, fmt.Sprintf("%s: %s", note.Name, note.Description))
	}
	return strings.Join(lines, "\n")
}

func (r RecallNoteResult) HumanText(Colorizer) string {
	return strings.TrimRight(r.Body, "\n")
}

func (r ForgetResult) HumanText(Colorizer) string {
	return "forgot " + r.Name
}

func (a *App) runRemember(opts rememberOptions) (RememberResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return RememberResult{}, err
	}
	fact := strings.TrimSpace(opts.Fact)
	if fact == "" {
		return RememberResult{}, syaerr.Usage{Message: "memory fact is required"}
	}
	key := memory.Slug(opts.Key)
	if key == "" {
		key = autoMemoryKey(fact)
	}
	if key == "" {
		return RememberResult{}, syaerr.Usage{Message: "memory key is required"}
	}
	taskIDs, warnings := resolveMemoryTasks(state, opts.Tasks)
	note, err := memory.Load(os.DirFS(state.Project.Root), ".sya/memory", key)
	if err != nil {
		var notFound syaerr.NotFound
		if !errors.As(err, &notFound) {
			return RememberResult{}, err
		}
		note = memory.Note{
			Name:        key,
			Description: memoryDescription(fact),
		}
	}
	note.Name = key
	if strings.TrimSpace(note.Description) == "" {
		note.Description = memoryDescription(fact)
	}
	note.Body = appendMemoryBody(note.Body, fact)
	note.Tasks = mergeMemoryTasks(note.Tasks, taskIDs)
	if err := memory.SaveWith(memory.OSWriter{}, filepath.Join(state.Project.SyaDir, "memory"), note); err != nil {
		return RememberResult{}, err
	}
	saved, err := memory.Load(os.DirFS(state.Project.Root), ".sya/memory", key)
	if err != nil {
		return RememberResult{}, err
	}
	return RememberResult{Note: memorySummary(saved), Warnings: warnings}, nil
}

func (a *App) runRecall(key string) (any, error) {
	project, err := a.DiscoverProject()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		notes, err := memory.List(os.DirFS(project.Root), ".sya/memory")
		if err != nil {
			return nil, err
		}
		result := RecallListResult{Notes: make([]MemorySummary, 0, len(notes))}
		for _, note := range notes {
			result.Notes = append(result.Notes, memorySummary(note))
		}
		return result, nil
	}
	note, err := memory.Load(os.DirFS(project.Root), ".sya/memory", key)
	if err != nil {
		return nil, err
	}
	return RecallNoteResult{Note: memorySummary(note), Body: note.Body}, nil
}

func (a *App) runForget(key string) (ForgetResult, error) {
	project, err := a.DiscoverProject()
	if err != nil {
		return ForgetResult{}, err
	}
	name := memory.Slug(key)
	if err := memory.DeleteWith(memory.OSWriter{}, filepath.Join(project.SyaDir, "memory"), name); err != nil {
		return ForgetResult{}, err
	}
	return ForgetResult{Name: name}, nil
}

func resolveMemoryTasks(state *projectState, rawIDs []string) ([]string, []MemoryWarning) {
	ids := make([]string, 0, len(rawIDs))
	warnings := make([]MemoryWarning, 0)
	for _, raw := range rawIDs {
		t, err := state.Index.Resolve(raw)
		if err != nil {
			warnings = append(warnings, MemoryWarning{Task: raw, Message: "task not found: " + raw})
			continue
		}
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)
	return compactStringSlice(ids), warnings
}

func autoMemoryKey(fact string) string {
	words := strings.Fields(fact)
	if len(words) > 6 {
		words = words[:6]
	}
	return memory.Slug(strings.Join(words, " "))
}

func memoryDescription(fact string) string {
	line := strings.TrimSpace(strings.Split(fact, "\n")[0])
	if len(line) <= 100 {
		return line
	}
	return strings.TrimSpace(line[:100])
}

func appendMemoryBody(existing, fact string) string {
	existing = strings.TrimRight(existing, "\n")
	if existing == "" {
		return fact + "\n"
	}
	return existing + "\n\n" + fact + "\n"
}

func mergeMemoryTasks(existing, add []string) []string {
	out := append([]string(nil), existing...)
	out = append(out, add...)
	sort.Strings(out)
	return compactStringSlice(out)
}

func memorySummary(note memory.Note) MemorySummary {
	return MemorySummary{
		Name:        note.Name,
		Description: note.Description,
		Tasks:       append([]string(nil), note.Tasks...),
		File:        note.File,
	}
}

func compactStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:0]
	var last string
	for _, value := range values {
		if value == "" || value == last {
			continue
		}
		out = append(out, value)
		last = value
	}
	return out
}
