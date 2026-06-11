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
}
