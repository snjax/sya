package cli

import "github.com/spf13/cobra"

type commandFactory func(*App) *cobra.Command

var commandRegistry []commandFactory

func registerCommand(factory commandFactory) {
	commandRegistry = append(commandRegistry, factory)
}

func registerCommands(app *App, root *cobra.Command) {
	for _, factory := range commandRegistry {
		root.AddCommand(factory(app))
	}
	root.AddCommand(app.schemaCommand())
	root.AddCommand(app.stub("board"))
	root.AddCommand(app.stub("ready"))
	root.AddCommand(app.stub("blocked"))
	root.AddCommand(app.stub("move"))
	root.AddCommand(app.stub("update"))
	root.AddCommand(app.stub("edit"))
	root.AddCommand(app.stub("claim"))
	root.AddCommand(app.stub("close"))
	root.AddCommand(app.stub("reopen"))
	root.AddCommand(app.stub("link"))
	root.AddCommand(app.stub("unlink"))
	root.AddCommand(app.epicCommand())
	root.AddCommand(app.stub("comment"))
	root.AddCommand(app.stub("archive"))
	root.AddCommand(app.stub("restore"))
	root.AddCommand(app.stub("search"))
	root.AddCommand(app.stub("events"))
	root.AddCommand(app.stub("doctor"))
	root.AddCommand(app.stub("remember"))
	root.AddCommand(app.stub("recall"))
	root.AddCommand(app.stub("forget"))
	root.AddCommand(app.wispCommand())
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
