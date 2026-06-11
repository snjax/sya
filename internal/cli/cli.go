package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

type Options struct {
	Version string
	Stdout  io.Writer
	Stderr  io.Writer
}

type App struct {
	json  bool
	quiet bool
	root  *cobra.Command
	out   io.Writer
	err   io.Writer
}

func New(options Options) *App {
	app := &App{
		out: options.Stdout,
		err: options.Stderr,
	}
	if app.out == nil {
		app.out = io.Discard
	}
	if app.err == nil {
		app.err = io.Discard
	}

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
	root.SetOut(app.out)
	root.SetErr(app.err)

	root.AddCommand(app.versionCommand(options.Version))
	registerStubs(root)

	app.root = root
	return app
}

func (a *App) Execute(args []string) int {
	a.root.SetArgs(args)
	if err := a.root.Execute(); err != nil {
		if a.json {
			_ = writeJSON(a.out, syaerr.Failure(err))
		} else if !a.quiet {
			fmt.Fprintln(a.err, err)
		}
		return syaerr.ExitCode(err)
	}
	return syaerr.ExitOK
}

func (a *App) versionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.json {
				return writeJSON(cmd.OutOrStdout(), syaerr.Success(map[string]string{
					"version": version,
				}))
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "sya %s\n", version)
			return err
		},
	}
}

func registerStubs(root *cobra.Command) {
	root.AddCommand(stub("init"))
	root.AddCommand(schemaCommand())
	root.AddCommand(stub("create"))
	root.AddCommand(stub("show"))
	root.AddCommand(stub("transitions"))
	root.AddCommand(stub("list"))
	root.AddCommand(stub("board"))
	root.AddCommand(stub("ready"))
	root.AddCommand(stub("blocked"))
	root.AddCommand(stub("move"))
	root.AddCommand(stub("update"))
	root.AddCommand(stub("edit"))
	root.AddCommand(stub("claim"))
	root.AddCommand(stub("close"))
	root.AddCommand(stub("reopen"))
	root.AddCommand(stub("link"))
	root.AddCommand(stub("unlink"))
	root.AddCommand(epicCommand())
	root.AddCommand(stub("comment"))
	root.AddCommand(stub("archive"))
	root.AddCommand(stub("restore"))
	root.AddCommand(stub("search"))
	root.AddCommand(stub("events"))
	root.AddCommand(stub("doctor"))
	root.AddCommand(stub("prime"))
	root.AddCommand(stub("remember"))
	root.AddCommand(stub("recall"))
	root.AddCommand(stub("forget"))
	root.AddCommand(wispCommand())
}

func schemaCommand() *cobra.Command {
	cmd := stub("schema")
	cmd.AddCommand(stub("validate"))
	cmd.AddCommand(stub("show"))
	cmd.AddCommand(stub("graph"))
	cmd.AddCommand(stub("docs"))
	cmd.AddCommand(stub("migrate"))
	return cmd
}

func epicCommand() *cobra.Command {
	cmd := stub("epic")
	cmd.AddCommand(stub("tree"))
	cmd.AddCommand(stub("progress"))
	return cmd
}

func wispCommand() *cobra.Command {
	cmd := stub("wisp")
	cmd.AddCommand(stub("create"))
	cmd.AddCommand(stub("list"))
	cmd.AddCommand(stub("show"))
	cmd.AddCommand(stub("squash"))
	cmd.AddCommand(stub("burn"))
	return cmd
}

func stub(use string) *cobra.Command {
	return &cobra.Command{
		Use:          use,
		Short:        "Not implemented",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return syaerr.Usage{Message: "not implemented"}
		},
	}
}

func writeJSON(w io.Writer, envelope syaerr.Envelope) error {
	encoder := json.NewEncoder(w)
	return encoder.Encode(envelope)
}
