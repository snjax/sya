package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/snjax/sya/internal/fsutil"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var prefix string
		var noAgentsMD bool
		cmd := app.command("init", "Initialize a sya project", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runInit(prefix, !noAgentsMD)
		})
		cmd.Flags().StringVar(&prefix, "prefix", "sya", "task id display prefix")
		cmd.Flags().BoolVar(&noAgentsMD, "no-agents-md", false, "do not create or update AGENTS.md/CLAUDE.md")
		return cmd
	})
}

type InitResult struct {
	Root       string           `json:"root"`
	SyaDir     string           `json:"sya_dir"`
	Created    []string         `json:"created"`
	AgentDocs  []AgentDocChange `json:"agent_docs,omitempty"`
	Suggestion string           `json:"suggestion"`
}

func (r InitResult) HumanText(Colorizer) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Initialized sya project at %s\n", r.SyaDir))
	if len(r.AgentDocs) > 0 {
		b.WriteString("Agent docs:\n")
		for _, change := range r.AgentDocs {
			b.WriteString(fmt.Sprintf("- %s %s", change.Action, change.Path))
			if change.Message != "" {
				b.WriteString(" (" + change.Message + ")")
			}
			b.WriteByte('\n')
		}
	}
	b.WriteString(r.Suggestion)
	return b.String()
}

func (a *App) runInit(prefix string, manageAgentDocs bool) (InitResult, error) {
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
	if _, err := os.Stat(syaDir); err == nil {
		return InitResult{}, syaerr.Usage{Message: "sya project already initialized"}
	} else if err != nil && !os.IsNotExist(err) {
		return InitResult{}, err
	}
	dirs := []string{
		filepath.Join(syaDir, "tasks"),
		filepath.Join(syaDir, "memory"),
		filepath.Join(syaDir, "templates"),
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
	for _, name := range fsutil.SearchIgnoreFiles {
		files = append(files, struct {
			name string
			data []byte
		}{
			name: filepath.Join(syaDir, name),
			data: []byte(fsutil.SearchIgnoreContent),
		})
	}
	var created []string
	for _, file := range files {
		if err := fsutil.AtomicWriteFile(file.name, file.data, 0o644); err != nil {
			return InitResult{}, err
		}
		created = append(created, file.name)
	}
	if err := appendGitignoreRuntime(root); err != nil {
		return InitResult{}, err
	}
	var agentDocs []AgentDocChange
	if manageAgentDocs {
		agentDocs, err = EnsureAgentDocs(root, AgentDocOptions{})
		if err != nil {
			return InitResult{}, err
		}
	}
	return InitResult{
		Root:       root,
		SyaDir:     syaDir,
		Created:    created,
		AgentDocs:  agentDocs,
		Suggestion: "Suggested hook: run `sya doctor` from your pre-commit hook before committing .sya changes.",
	}, nil
}
