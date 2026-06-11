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
	root.AddCommand(app.stub("board"))
	root.AddCommand(app.epicCommand())
	root.AddCommand(app.stub("comment"))
	root.AddCommand(app.stub("archive"))
	root.AddCommand(app.stub("restore"))
	root.AddCommand(app.stub("search"))
	root.AddCommand(app.stub("events"))
	root.AddCommand(app.wispCommand())
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
