package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts graphOptions
		cmd := app.command("graph", "Render task instance graph", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runGraph(opts)
		})
		cmd.Flags().Var(&opts.Relations, "rel", "relation to include")
		cmd.Flags().StringVar(&opts.Epic, "epic", "", "limit to epic subtree")
		cmd.Flags().VarP(&opts.Types, "type", "t", "task type filter")
		cmd.Flags().BoolVar(&opts.AllRelations, "all-relations", false, "include all relation edges")
		cmd.Flags().StringVar(&opts.Format, "format", "mermaid", "output format: mermaid or dot")
		return cmd
	})
}

type graphOptions struct {
	Relations    stringList
	Epic         string
	Types        stringList
	AllRelations bool
	Format       string
}

type GraphResult struct {
	Format  string      `json:"format"`
	Content string      `json:"content"`
	Nodes   []GraphNode `json:"nodes"`
	Edges   []GraphEdge `json:"edges"`
}

type GraphNode struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Parent   string `json:"parent,omitempty"`
	Archived bool   `json:"archived,omitempty"`
}

type GraphEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
}

func (r GraphResult) HumanText(Colorizer) string { return r.Content }

func (a *App) runGraph(opts graphOptions) (GraphResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return GraphResult{}, err
	}
	if opts.Format == "" {
		opts.Format = "mermaid"
	}
	if opts.Format != "mermaid" && opts.Format != "dot" {
		return GraphResult{}, syaerr.Usage{Message: "graph --format must be mermaid or dot"}
	}
	rels, err := graphRelations(state.Schema, opts)
	if err != nil {
		return GraphResult{}, err
	}
	selected, err := graphSelectedTasks(state, opts)
	if err != nil {
		return GraphResult{}, err
	}
	nodes, edges := graphModel(state.Index, selected, rels)
	result := GraphResult{Format: opts.Format, Nodes: nodes, Edges: edges}
	if opts.Format == "dot" {
		result.Content = renderDOTGraph(nodes, edges)
	} else {
		result.Content = renderMermaidGraph(nodes, edges)
	}
	return result, nil
}

func graphRelations(sch *schema.Schema, opts graphOptions) (map[string]bool, error) {
	out := make(map[string]bool)
	if opts.AllRelations {
		for name := range sch.Relations {
			out[name] = true
		}
		return out, nil
	}
	if len(opts.Relations) > 0 {
		for _, relation := range opts.Relations {
			if _, ok := sch.Relations[relation]; !ok {
				return nil, syaerr.Usage{Message: "unknown relation: " + relation}
			}
			out[relation] = true
		}
		return out, nil
	}
	for name, def := range sch.Relations {
		if def.Blocking {
			out[name] = true
		}
	}
	return out, nil
}

func graphSelectedTasks(state *projectState, opts graphOptions) (map[string]*task.Task, error) {
	allowed := make(map[string]bool)
	if opts.Epic != "" {
		epic, err := state.Index.Resolve(opts.Epic)
		if err != nil {
			return nil, err
		}
		allowed[epic.ID] = true
		addDescendants(state.Index, epic.ID, allowed)
	}
	typeFilter := make(map[string]bool, len(opts.Types))
	for _, typ := range opts.Types {
		if _, ok := state.Schema.Types[typ]; !ok {
			return nil, syaerr.Usage{Message: "unknown task type: " + typ}
		}
		typeFilter[typ] = true
	}
	selected := make(map[string]*task.Task)
	for _, t := range state.Index.All() {
		if len(allowed) > 0 && !allowed[t.ID] {
			continue
		}
		if len(typeFilter) > 0 && !typeFilter[t.Type] {
			continue
		}
		selected[t.ID] = t
	}
	return selected, nil
}

func addDescendants(idx *index.Index, id string, out map[string]bool) {
	for _, childID := range idx.ReverseEdges()[id]["children"] {
		if out[childID] {
			continue
		}
		out[childID] = true
		addDescendants(idx, childID, out)
	}
}

func graphModel(idx *index.Index, selected map[string]*task.Task, rels map[string]bool) ([]GraphNode, []GraphEdge) {
	nodes := make([]GraphNode, 0, len(selected))
	for _, t := range selected {
		nodes = append(nodes, GraphNode{ID: t.ID, Title: t.Title, Type: t.Type, Status: t.Status, Parent: t.Parent, Archived: t.Archived})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	var edges []GraphEdge
	for edge := range idx.CanonicalOrigins() {
		if !rels[edge.Relation] || selected[edge.From] == nil || selected[edge.To] == nil {
			continue
		}
		edges = append(edges, GraphEdge{From: edge.From, To: edge.To, Relation: edge.Relation})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].Relation != edges[j].Relation {
			return edges[i].Relation < edges[j].Relation
		}
		return edges[i].To < edges[j].To
	})
	return nodes, edges
}

func renderMermaidGraph(nodes []GraphNode, edges []GraphEdge) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	rendered := make(map[string]bool)
	children := graphChildren(nodes)
	nodeByID := graphNodeMap(nodes)
	for _, node := range nodes {
		if node.Type == "epic" && len(children[node.ID]) > 0 {
			fmt.Fprintf(&b, "  subgraph %s[%q]\n", graphNodeID(node.ID), graphNodeLabel(node))
			renderMermaidNode(&b, node, rendered, "    ")
			for _, childID := range children[node.ID] {
				if child, ok := nodeByID[childID]; ok {
					renderMermaidNode(&b, child, rendered, "    ")
				}
			}
			b.WriteString("  end\n")
		}
	}
	for _, node := range nodes {
		renderMermaidNode(&b, node, rendered, "  ")
	}
	for _, edge := range edges {
		fmt.Fprintf(&b, "  %s -->|%s| %s\n", graphNodeID(edge.From), edge.Relation, graphNodeID(edge.To))
	}
	if hasArchived(nodes) {
		b.WriteString("  classDef archived stroke-dasharray: 5 5\n")
		for _, node := range nodes {
			if node.Archived {
				fmt.Fprintf(&b, "  class %s archived\n", graphNodeID(node.ID))
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderMermaidNode(b *strings.Builder, node GraphNode, rendered map[string]bool, indent string) {
	if rendered[node.ID] {
		return
	}
	rendered[node.ID] = true
	fmt.Fprintf(b, "%s%s[%q]\n", indent, graphNodeID(node.ID), graphNodeLabel(node))
}

func renderDOTGraph(nodes []GraphNode, edges []GraphEdge) string {
	var b strings.Builder
	b.WriteString("digraph sya {\n")
	for _, node := range nodes {
		style := ""
		if node.Archived {
			style = `, style="dashed"`
		}
		fmt.Fprintf(&b, "  %q [label=%q%s];\n", node.ID, graphNodeLabel(node), style)
	}
	for _, edge := range edges {
		fmt.Fprintf(&b, "  %q -> %q [label=%q];\n", edge.From, edge.To, edge.Relation)
	}
	b.WriteString("}")
	return b.String()
}

func graphChildren(nodes []GraphNode) map[string][]string {
	children := make(map[string][]string)
	for _, node := range nodes {
		if node.Parent != "" {
			children[node.Parent] = append(children[node.Parent], node.ID)
		}
	}
	for parent := range children {
		sort.Strings(children[parent])
	}
	return children
}

func graphNodeMap(nodes []GraphNode) map[string]GraphNode {
	out := make(map[string]GraphNode, len(nodes))
	for _, node := range nodes {
		out[node.ID] = node
	}
	return out
}

func graphNodeID(id string) string {
	return "n_" + strings.NewReplacer("-", "_").Replace(id)
}

func graphNodeLabel(node GraphNode) string {
	return fmt.Sprintf("%s %s [%s]", node.ID, truncateTitle(node.Title, 30), node.Status)
}

func truncateTitle(title string, limit int) string {
	runes := []rune(title)
	if len(runes) <= limit {
		return title
	}
	return string(runes[:limit]) + "..."
}

func hasArchived(nodes []GraphNode) bool {
	for _, node := range nodes {
		if node.Archived {
			return true
		}
	}
	return false
}
