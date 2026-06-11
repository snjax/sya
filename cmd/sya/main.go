package main

import (
	"os"

	"github.com/snjax/sya/internal/cli"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	app := cli.New(cli.Options{
		Version: version,
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	os.Exit(app.Execute(os.Args[1:]))
}
