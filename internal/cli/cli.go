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

	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

type Options struct {
	Version string
	Stdout  io.Writer
	Stderr  io.Writer
	WorkDir string
	Env     func(string) string
	GitUser func(context.Context) (string, error)
}

type App struct {
	json     bool
	quiet    bool
	actor    string
	root     *cobra.Command
	out      io.Writer
	err      io.Writer
	workDir  string
	env      func(string) string
	gitUser  func(context.Context) (string, error)
	result   any
	colorize Colorizer
}

func New(options Options) *App {
	app := &App{
		out:     options.Stdout,
		err:     options.Stderr,
		workDir: options.WorkDir,
		env:     options.Env,
		gitUser: options.GitUser,
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
	root.SetOut(app.out)
	root.SetErr(app.err)

	root.AddCommand(app.versionCommand(options.Version))
	app.registerStubs(root)

	app.root = root
	return app
}

func (a *App) Execute(args []string) int {
	a.result = nil
	a.root.SetArgs(args)
	if err := a.root.Execute(); err != nil {
		return a.EmitError(err)
	}
	if a.result == silent {
		return syaerr.ExitOK
	}
	if err := a.Emit(a.result); err != nil {
		return a.EmitError(err)
	}
	return syaerr.ExitOK
}

func (a *App) versionCommand(version string) *cobra.Command {
	return a.command("version", "Print version", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		return Version{Version: version}, nil
	})
}

func (a *App) registerStubs(root *cobra.Command) {
	root.AddCommand(a.stub("init"))
	root.AddCommand(a.schemaCommand())
	root.AddCommand(a.stub("create"))
	root.AddCommand(a.stub("show"))
	root.AddCommand(a.stub("transitions"))
	root.AddCommand(a.stub("list"))
	root.AddCommand(a.stub("board"))
	root.AddCommand(a.stub("ready"))
	root.AddCommand(a.stub("blocked"))
	root.AddCommand(a.stub("move"))
	root.AddCommand(a.stub("update"))
	root.AddCommand(a.stub("edit"))
	root.AddCommand(a.stub("claim"))
	root.AddCommand(a.stub("close"))
	root.AddCommand(a.stub("reopen"))
	root.AddCommand(a.stub("link"))
	root.AddCommand(a.stub("unlink"))
	root.AddCommand(a.epicCommand())
	root.AddCommand(a.stub("comment"))
	root.AddCommand(a.stub("archive"))
	root.AddCommand(a.stub("restore"))
	root.AddCommand(a.stub("search"))
	root.AddCommand(a.stub("events"))
	root.AddCommand(a.stub("doctor"))
	root.AddCommand(a.primeCommand())
	root.AddCommand(a.stub("remember"))
	root.AddCommand(a.stub("recall"))
	root.AddCommand(a.stub("forget"))
	root.AddCommand(a.wispCommand())
}

func (a *App) schemaCommand() *cobra.Command {
	cmd := a.stub("schema")
	cmd.AddCommand(a.stub("validate"))
	cmd.AddCommand(a.stub("show"))
	cmd.AddCommand(a.stub("graph"))
	cmd.AddCommand(a.stub("docs"))
	cmd.AddCommand(a.stub("migrate"))
	return cmd
}

func (a *App) epicCommand() *cobra.Command {
	cmd := a.stub("epic")
	cmd.AddCommand(a.stub("tree"))
	cmd.AddCommand(a.stub("progress"))
	return cmd
}

func (a *App) wispCommand() *cobra.Command {
	cmd := a.stub("wisp")
	cmd.AddCommand(a.stub("create"))
	cmd.AddCommand(a.stub("list"))
	cmd.AddCommand(a.stub("show"))
	cmd.AddCommand(a.stub("squash"))
	cmd.AddCommand(a.stub("burn"))
	return cmd
}

func (a *App) primeCommand() *cobra.Command {
	return a.command("prime", "Print agent context", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		if _, err := a.DiscoverProject(); err != nil {
			var notFound syaerr.NotFound
			if errors.As(err, &notFound) && notFound.ID == ".sya" {
				return silent, nil
			}
			return nil, err
		}
		return nil, syaerr.Usage{Message: "not implemented"}
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
