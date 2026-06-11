package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var prefix string
		cmd := app.command("init", "Initialize a sya project", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runInit(prefix)
		})
		cmd.Flags().StringVar(&prefix, "prefix", "sya", "task id display prefix")
		return cmd
	})
}

type InitResult struct {
	Root       string   `json:"root"`
	SyaDir     string   `json:"sya_dir"`
	Created    []string `json:"created"`
	Suggestion string   `json:"suggestion"`
}

func (r InitResult) HumanText(Colorizer) string {
	return fmt.Sprintf("Initialized sya project at %s\n%s", r.SyaDir, r.Suggestion)
}

func (a *App) runInit(prefix string) (InitResult, error) {
	root := a.workDir
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return InitResult{}, err
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return InitResult{}, err
	}
	if prefix == "" {
		prefix = "sya"
	}
	syaDir := filepath.Join(root, ".sya")
	dirs := []string{
		filepath.Join(syaDir, "tasks"),
		filepath.Join(syaDir, "memory"),
		filepath.Join(syaDir, "wisps"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return InitResult{}, err
		}
	}
	config := fmt.Sprintf("project: %s\nprefix: %s\nid_length: 6\narchive:\n  after_days: 30\n", filepath.Base(root), prefix)
	files := []struct {
		name string
		data []byte
	}{
		{name: filepath.Join(syaDir, "config.yml"), data: []byte(config)},
		{name: filepath.Join(syaDir, "schema.yml"), data: DefaultSchemaBytes()},
	}
	var created []string
	for _, file := range files {
		if err := atomicWriteFile(file.name, file.data, 0o644); err != nil {
			return InitResult{}, err
		}
		created = append(created, file.name)
	}
	if err := appendGitignoreWisps(root); err != nil {
		return InitResult{}, err
	}
	return InitResult{
		Root:       root,
		SyaDir:     syaDir,
		Created:    created,
		Suggestion: "Suggested hook: run `sya doctor` from your pre-commit hook before committing .sya changes.",
	}, nil
}
