package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/snjax/sya/internal/events"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

type Options struct {
	Version     string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	WorkDir     string
	Env         func(string) string
	GitUser     func(context.Context) (string, error)
	Now         func() time.Time
	NewID       func(map[string]struct{}, int) (string, error)
	AppendEvent func(string, events.Event) error
}

type App struct {
	json        bool
	quiet       bool
	actor       string
	root        *cobra.Command
	in          io.Reader
	out         io.Writer
	err         io.Writer
	workDir     string
	env         func(string) string
	gitUser     func(context.Context) (string, error)
	now         func() time.Time
	newID       func(map[string]struct{}, int) (string, error)
	appendEvent func(string, events.Event) error
	result      any
	colorize    Colorizer
}

func New(options Options) *App {
	app := &App{
		in:          options.Stdin,
		out:         options.Stdout,
		err:         options.Stderr,
		workDir:     options.WorkDir,
		env:         options.Env,
		gitUser:     options.GitUser,
		now:         options.Now,
		newID:       options.NewID,
		appendEvent: options.AppendEvent,
	}
	if app.in == nil {
		app.in = os.Stdin
	}
	if app.out == nil {
		app.out = io.Discard
	}
	if app.err == nil {
		app.err = io.Discard
	}
	if app.env == nil {
		app.env = os.Getenv
	}
	if app.gitUser == nil {
		app.gitUser = gitConfigUserName
	}
	if app.now == nil {
		app.now = func() time.Time { return time.Now().UTC() }
	}
	if app.newID == nil {
		app.newID = task.NewID
	}
	if app.appendEvent == nil {
		app.appendEvent = events.Append
	}
	app.colorize = NewColorizer(app.env)

	root := &cobra.Command{
		Use:           "sya",
		Short:         "git-native issue tracker for AI-agent workflows",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.PersistentFlags().BoolVar(&app.json, "json", false, "emit JSON envelope")
	root.PersistentFlags().BoolVar(&app.quiet, "quiet", false, "suppress human-readable diagnostics")
	root.PersistentFlags().StringVar(&app.actor, "actor", "", "actor name for log entries")
	root.SetIn(app.in)
	root.SetOut(app.out)
	root.SetErr(app.err)

	root.AddCommand(app.versionCommand(options.Version))
	registerCommands(app, root)

	app.root = root
	return app
}

func (a *App) Execute(args []string) int {
	a.result = nil
	a.root.SetArgs(args)
	if err := a.root.Execute(); err != nil {
		var partial partialError
		if errors.As(err, &partial) {
			if emitErr := a.Emit(partial.data); emitErr != nil {
				return a.EmitError(emitErr)
			}
			return partial.code
		}
		return a.EmitError(err)
	}
	if a.result == silent {
		return syaerr.ExitOK
	}
	if err := a.Emit(a.result); err != nil {
		return a.EmitError(err)
	}
	if result, ok := a.result.(interface{ ResultExitCode() int }); ok {
		return result.ResultExitCode()
	}
	return syaerr.ExitOK
}

func (a *App) versionCommand(version string) *cobra.Command {
	return a.command("version", "Print version", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		return Version{Version: version}, nil
	})
}

func (a *App) stub(use string) *cobra.Command {
	return a.command(use, "Not implemented", nil, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		return nil, syaerr.Usage{Message: "not implemented"}
	})
}

type Handler func(context.Context, *cobra.Command, []string) (any, error)

func (a *App) command(use, short string, args cobra.PositionalArgs, handler Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:          use,
		Short:        short,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, argv []string) error {
			if args != nil {
				if err := args(cmd, argv); err != nil {
					return syaerr.Usage{Message: err.Error()}
				}
			}
			data, err := handler(cmd.Context(), cmd, argv)
			if err != nil {
				return err
			}
			a.result = data
			return nil
		},
	}
	return cmd
}

func (a *App) Emit(data any) error {
	if a.json {
		return writeJSON(a.out, syaerr.Success(data))
	}
	if data == nil {
		return nil
	}
	text, err := humanText(data, a.colorize)
	if err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	_, err = fmt.Fprintln(a.out, text)
	return err
}

func (a *App) EmitError(err error) int {
	if a.json {
		_ = writeJSON(a.out, syaerr.Failure(err))
	} else if !a.quiet {
		fmt.Fprintln(a.err, a.colorize.Red("error:")+" "+syaerr.ErrorMessage(err))
	}
	return syaerr.ExitCode(err)
}

func (a *App) Actor() string {
	return ActorResolver{
		Env:     a.env,
		GitUser: a.gitUser,
	}.Resolve(context.Background(), a.actor)
}

func (a *App) DiscoverProject() (Project, error) {
	return FindProject(a.workDir)
}

type Version struct {
	Version string `json:"version"`
}

func (v Version) HumanText(Colorizer) string {
	return fmt.Sprintf("sya %s", v.Version)
}

type Project struct {
	Root   string `json:"root"`
	SyaDir string `json:"sya_dir"`
}

func FindProject(cwd string) (Project, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return Project{}, err
		}
	}
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return Project{}, err
	}
	for {
		syaDir := filepath.Join(dir, ".sya")
		info, err := os.Stat(syaDir)
		if err == nil && info.IsDir() {
			return Project{Root: dir, SyaDir: syaDir}, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return Project{}, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Project{}, syaerr.NotFound{ID: ".sya"}
		}
		dir = parent
	}
}

type ActorResolver struct {
	Env     func(string) string
	GitUser func(context.Context) (string, error)
}

func (r ActorResolver) Resolve(ctx context.Context, flag string) string {
	if actor := strings.TrimSpace(flag); actor != "" {
		return actor
	}
	env := r.Env
	if env == nil {
		env = os.Getenv
	}
	if actor := strings.TrimSpace(env("SYA_ACTOR")); actor != "" {
		return actor
	}
	gitUser := r.GitUser
	if gitUser == nil {
		gitUser = gitConfigUserName
	}
	if actor, err := gitUser(ctx); err == nil {
		if actor = strings.TrimSpace(actor); actor != "" {
			return actor
		}
	}
	return "unknown"
}

func gitConfigUserName(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "config", "user.name")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(out)), nil
}

type Colorizer struct {
	enabled bool
}

func NewColorizer(env func(string) string) Colorizer {
	if env == nil {
		env = os.Getenv
	}
	return Colorizer{enabled: env("NO_COLOR") == ""}
}

func (c Colorizer) Red(s string) string {
	if !c.enabled {
		return s
	}
	return "\x1b[31m" + s + "\x1b[0m"
}

type humanRenderer interface {
	HumanText(Colorizer) string
}

func humanText(data any, colorize Colorizer) (string, error) {
	switch v := data.(type) {
	case humanRenderer:
		return v.HumanText(colorize), nil
	case string:
		return v, nil
	case []byte:
		return strings.TrimRight(string(v), "\n"), nil
	case fmt.Stringer:
		return v.String(), nil
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

type silentSuccess struct{}

var silent = silentSuccess{}

func writeJSON(w io.Writer, envelope syaerr.Envelope) error {
	encoder := json.NewEncoder(w)
	return encoder.Encode(envelope)
}
