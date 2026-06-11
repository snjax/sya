package cli

import (
	"context"
	"errors"

	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		return app.command("prime", "Print agent context", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			if _, err := app.DiscoverProject(); err != nil {
				var notFound syaerr.NotFound
				if errors.As(err, &notFound) && notFound.ID == ".sya" {
					return silent, nil
				}
				return nil, err
			}
			return nil, syaerr.Usage{Message: "not implemented"}
		})
	})
}
