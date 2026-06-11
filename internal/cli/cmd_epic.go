package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		cmd := &cobra.Command{
			Use:          "epic",
			Short:        "Epic commands",
			SilenceUsage: true,
		}
		cmd.AddCommand(app.command("tree <id>", "Show recursive epic tree", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runEpicTree(args[0])
		}))
		cmd.AddCommand(app.command("progress <id>", "Show epic progress numbers", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runEpicProgress(args[0])
		}))
		return cmd
	})
}

type EpicTreeResult struct {
	Root EpicNode `json:"root"`
}

type EpicProgressResult struct {
	ID     string `json:"id"`
	Closed int    `json:"closed"`
	Total  int    `json:"total"`
}

type EpicNode struct {
	Task     TaskSummary `json:"task"`
	Closed   int         `json:"closed"`
	Total    int         `json:"total"`
	Children []EpicNode  `json:"children,omitempty"`
}

func (r EpicTreeResult) HumanText(Colorizer) string {
	var b strings.Builder
	writeEpicNode(&b, r.Root, 0)
	return strings.TrimRight(b.String(), "\n")
}

func (r EpicProgressResult) HumanText(Colorizer) string {
	return fmt.Sprintf("%s closed=%d total=%d", r.ID, r.Closed, r.Total)
}

func (a *App) runEpicTree(id string) (EpicTreeResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return EpicTreeResult{}, err
	}
	root, err := state.Index.Resolve(id)
	if err != nil {
		return EpicTreeResult{}, err
	}
	if !state.Schema.Types[root.Type].Container {
		return EpicTreeResult{}, syaerr.Usage{Message: "task is not an epic/container: " + root.ID}
	}
	node, err := buildEpicNode(state, root, make(map[string]struct{}))
	if err != nil {
		return EpicTreeResult{}, err
	}
	return EpicTreeResult{Root: node}, nil
}

func (a *App) runEpicProgress(id string) (EpicProgressResult, error) {
	tree, err := a.runEpicTree(id)
	if err != nil {
		return EpicProgressResult{}, err
	}
	return EpicProgressResult{ID: tree.Root.Task.ID, Closed: tree.Root.Closed, Total: tree.Root.Total}, nil
}

func buildEpicNode(state *projectState, t *task.Task, visited map[string]struct{}) (EpicNode, error) {
	if _, ok := visited[t.ID]; ok {
		return EpicNode{}, syaerr.SchemaInvalid{Message: "parent cycle detected at " + t.ID}
	}
	visited[t.ID] = struct{}{}
	node := EpicNode{Task: summarizeTask(t), Total: 1}
	if taskClosed(state.Schema, t) {
		node.Closed = 1
	}
	for _, childID := range state.Index.Children(t.ID) {
		child, err := state.Index.Resolve(childID)
		if err != nil {
			continue
		}
		childNode, err := buildEpicNode(state, child, visited)
		if err != nil {
			return EpicNode{}, err
		}
		node.Children = append(node.Children, childNode)
		node.Closed += childNode.Closed
		node.Total += childNode.Total
	}
	delete(visited, t.ID)
	return node, nil
}

func taskClosed(sch *schema.Schema, t *task.Task) bool {
	typeDef := sch.Types[t.Type]
	return stringIn(typeDef.Terminal, t.Status)
}

func writeEpicNode(b *strings.Builder, node EpicNode, depth int) {
	indent := strings.Repeat("  ", depth)
	marker := statusMarker(node.Task.Status, node.Closed == node.Total)
	fmt.Fprintf(b, "%s%s %s [%s] %s (%d/%d)\n", indent, marker, node.Task.ID, node.Task.Status, node.Task.Title, node.Closed, node.Total)
	for _, child := range node.Children {
		writeEpicNode(b, child, depth+1)
	}
}

func statusMarker(status string, closed bool) string {
	if closed {
		return "[x]"
	}
	if status == "" {
		return "[ ]"
	}
	return "[ ]"
}
