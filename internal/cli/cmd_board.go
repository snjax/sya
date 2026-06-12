package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var taskType string
		cmd := app.command("board", "Show kanban boards", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runBoard(taskType)
		})
		cmd.Flags().StringVarP(&taskType, "type", "t", "", "task type")
		return cmd
	})
}

type BoardResult struct {
	Boards             map[string]map[string][]TaskSummary `json:"boards"`
	Order              []string                            `json:"-"`
	StatusOrder        map[string][]string                 `json:"-"`
	StatusDescriptions map[string]map[string]string        `json:"-"`
}

func (r BoardResult) HumanText(Colorizer) string {
	if len(r.Order) == 0 {
		return "no boards"
	}
	var b strings.Builder
	for boardIndex, typeName := range r.Order {
		if boardIndex > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s\n", typeName)
		for _, status := range r.StatusOrder[typeName] {
			fmt.Fprintf(&b, "  %s:\n", formatBoardStatusHeader(status, r.StatusDescriptions[typeName][status]))
			tasks := r.Boards[typeName][status]
			if len(tasks) == 0 {
				b.WriteString("    -\n")
				continue
			}
			for _, t := range tasks {
				cell := fmt.Sprintf("%s %s", t.ID, t.Title)
				if t.Assignee != "" {
					cell += " (" + t.Assignee + ")"
				}
				fmt.Fprintf(&b, "    %s\n", cell)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (a *App) runBoard(taskType string) (BoardResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return BoardResult{}, err
	}
	if taskType != "" {
		typeDef, ok := state.Schema.Types[taskType]
		if !ok {
			return BoardResult{}, syaerr.Usage{Message: "unknown task type: " + taskType}
		}
		if !typeBoardVisible(typeDef) {
			return BoardResult{Boards: map[string]map[string][]TaskSummary{}}, nil
		}
		return boardForTypes(state.Index, state.Schema, []string{taskType}), nil
	}
	typeNames := make([]string, 0, len(state.Schema.Types))
	for name, typeDef := range state.Schema.Types {
		if !typeBoardVisible(typeDef) {
			continue
		}
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)
	return boardForTypes(state.Index, state.Schema, typeNames), nil
}

func boardForTypes(idx *index.Index, sch *schema.Schema, typeNames []string) BoardResult {
	result := BoardResult{
		Boards:             make(map[string]map[string][]TaskSummary, len(typeNames)),
		Order:              append([]string(nil), typeNames...),
		StatusOrder:        make(map[string][]string, len(typeNames)),
		StatusDescriptions: make(map[string]map[string]string, len(typeNames)),
	}
	for _, typeName := range typeNames {
		typeDef := sch.Types[typeName]
		result.StatusOrder[typeName] = append([]string(nil), typeDef.Pipeline...)
		result.StatusDescriptions[typeName] = make(map[string]string, len(typeDef.Statuses))
		for status, description := range typeDef.Statuses {
			result.StatusDescriptions[typeName][status] = description
		}
		result.Boards[typeName] = make(map[string][]TaskSummary, len(typeDef.Pipeline))
		for _, status := range typeDef.Pipeline {
			result.Boards[typeName][status] = []TaskSummary{}
		}
		tasks := idx.Query(index.Query{Types: []string{typeName}, Archived: boolPtr(false)})
		for _, t := range tasks {
			if _, ok := result.Boards[typeName][t.Status]; !ok {
				result.Boards[typeName][t.Status] = []TaskSummary{}
				result.StatusOrder[typeName] = append(result.StatusOrder[typeName], t.Status)
			}
			result.Boards[typeName][t.Status] = append(result.Boards[typeName][t.Status], summarizeTask(t))
		}
	}
	return result
}

func formatBoardStatusHeader(status, description string) string {
	description = truncateStatusDescription(description)
	if description == "" {
		return status
	}
	return status + " — " + description
}

func typeBoardVisible(typeDef schema.TypeDef) bool {
	return typeDef.Board == nil || *typeDef.Board
}
