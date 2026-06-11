package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/snjax/sya/internal/gitx"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts restoreOptions
		cmd := app.command("restore <id>", "Show or apply a historical task body", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			opts.ID = args[0]
			return app.runRestore(ctx, opts, gitx.ExecRunner{})
		})
		cmd.Flags().StringVar(&opts.At, "at", "", "git revision to restore from")
		cmd.Flags().BoolVar(&opts.Apply, "apply", false, "write historical body back to the current file")
		return cmd
	})
}

type restoreOptions struct {
	ID    string
	At    string
	Apply bool
}

type RestoreResult struct {
	ID      string `json:"id"`
	File    string `json:"file"`
	Rev     string `json:"rev"`
	Content string `json:"content,omitempty"`
	Applied bool   `json:"applied,omitempty"`
}

func (r RestoreResult) HumanText(Colorizer) string {
	if r.Applied {
		return fmt.Sprintf("%s: restored body from %s", r.ID, r.Rev)
	}
	return r.Content
}

func (a *App) runRestore(ctx context.Context, opts restoreOptions, runner gitx.Runner) (RestoreResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return RestoreResult{}, err
	}
	if err := gitx.RequireRepo(ctx, runner, state.Project.Root); err != nil {
		return RestoreResult{}, syaerr.GitRequired{Message: "restore requires a git repository"}
	}
	current, err := state.Index.Resolve(opts.ID)
	if err != nil {
		return RestoreResult{}, err
	}
	rev := opts.At
	var data []byte
	if rev != "" {
		data, err = gitx.Show(ctx, runner, state.Project.Root, rev, filepath.ToSlash(current.File))
		if err != nil {
			return RestoreResult{}, syaerr.Usage{Message: err.Error()}
		}
	} else {
		rev, data, err = newestUnarchivedVersion(ctx, runner, state.Project.Root, filepath.ToSlash(current.File))
		if err != nil {
			return RestoreResult{}, err
		}
	}
	historical, err := task.ParseBytes(data)
	if err != nil {
		return RestoreResult{}, syaerr.SchemaInvalid{Message: "historical task version is invalid: " + err.Error()}
	}
	if !opts.Apply {
		return RestoreResult{ID: current.ID, File: current.File, Rev: rev, Content: string(data)}, nil
	}

	current.Body = historical.Body
	current.Archived = true
	appendLogLine(current, fmt.Sprintf("- %s @%s: restored from %s", a.now().UTC().Format(time.RFC3339), a.Actor(), rev))
	if err := writeTask(state.Project.Root, current); err != nil {
		return RestoreResult{}, err
	}
	return RestoreResult{ID: current.ID, File: current.File, Rev: rev, Applied: true}, nil
}

func newestUnarchivedVersion(ctx context.Context, runner gitx.Runner, root, file string) (string, []byte, error) {
	revs, err := gitx.FollowLog(ctx, runner, root, file)
	if err != nil {
		return "", nil, syaerr.Usage{Message: err.Error()}
	}
	for _, rev := range revs {
		data, err := gitx.Show(ctx, runner, root, rev, file)
		if err != nil {
			continue
		}
		t, err := task.ParseBytes(data)
		if err == nil && !t.Archived {
			return rev, data, nil
		}
	}
	data, err := gitx.Show(ctx, runner, root, "HEAD~", file)
	if err != nil {
		return "", nil, syaerr.Usage{Message: "no unarchived historical version found for " + strings.TrimSpace(file)}
	}
	return "HEAD~", data, nil
}
