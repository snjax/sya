package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		return app.schemaCommand()
	})
}

func (a *App) schemaCommand() *cobra.Command {
	cmd := a.command("schema", "Schema commands", nil, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		if err := cmd.Help(); err != nil {
			return nil, err
		}
		return silent, nil
	})
	cmd.AddCommand(a.schemaValidateCommand())
	cmd.AddCommand(a.schemaShowCommand())
	cmd.AddCommand(a.schemaGraphCommand())
	cmd.AddCommand(a.schemaDocsCommand())
	cmd.AddCommand(a.schemaMigrateCommand())
	return cmd
}

func (a *App) schemaValidateCommand() *cobra.Command {
	return a.command("validate", "Validate schema", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		return a.runSchemaValidate()
	})
}

func (a *App) schemaShowCommand() *cobra.Command {
	return a.command("show", "Show schema", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		return a.runSchemaShow()
	})
}

func (a *App) schemaGraphCommand() *cobra.Command {
	var typeName string
	cmd := a.command("graph", "Render schema graph", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		return a.runSchemaGraph(typeName)
	})
	cmd.Flags().StringVar(&typeName, "type", "", "task type")
	return cmd
}

func (a *App) schemaDocsCommand() *cobra.Command {
	var typeName string
	cmd := a.command("docs", "Render schema docs", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		return a.runSchemaDocs(typeName)
	})
	cmd.Flags().StringVar(&typeName, "type", "", "task type")
	return cmd
}

func (a *App) schemaMigrateCommand() *cobra.Command {
	var opts schemaMigrateOptions
	cmd := a.command("migrate", "Migrate task files after schema changes", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		if opts.DryRun {
			return a.runSchemaMigrate(opts)
		}
		return a.withProjectMutationLock(func() (any, error) {
			return a.runSchemaMigrate(opts)
		})
	})
	cmd.Flags().StringVar(&opts.RenameStatus, "rename-status", "", "rename status old=new")
	cmd.Flags().StringVar(&opts.Type, "type", "", "task type")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "list affected tasks without writing")
	return cmd
}

type SchemaValidateResult struct {
	Valid      bool               `json:"valid"`
	Violations []SchemaDiagnostic `json:"violations,omitempty"`
	Warnings   []SchemaDiagnostic `json:"warnings,omitempty"`
}

type SchemaDiagnostic struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type SchemaShowResult struct {
	Schema *schema.Schema `json:"schema"`
}

type SchemaGraphResult struct {
	Type    string `json:"type,omitempty"`
	Mermaid string `json:"mermaid"`
}

type SchemaDocsResult struct {
	Type     string `json:"type,omitempty"`
	Markdown string `json:"markdown"`
}

type schemaMigrateOptions struct {
	RenameStatus string
	Type         string
	DryRun       bool
}

type SchemaMigrateResult struct {
	Changes []SchemaMigrateChange `json:"changes"`
	DryRun  bool                  `json:"dry_run,omitempty"`
}

type SchemaMigrateChange struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	File   string `json:"file"`
	From   string `json:"from"`
	To     string `json:"to"`
	Status string `json:"status"`
}

func (r SchemaValidateResult) HumanText(Colorizer) string {
	var b strings.Builder
	if r.Valid {
		b.WriteString("schema valid")
	} else {
		b.WriteString("schema invalid")
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(&b, "\nwarning %s: %s", warning.Path, warning.Message)
	}
	for _, violation := range r.Violations {
		fmt.Fprintf(&b, "\nerror %s: %s", violation.Path, violation.Message)
	}
	return b.String()
}

func (r SchemaShowResult) HumanText(Colorizer) string {
	typeCount := 0
	relationCount := 0
	if r.Schema != nil {
		typeCount = len(r.Schema.Types)
		relationCount = len(r.Schema.Relations)
	}
	return fmt.Sprintf("schema_version: %d\ntypes: %d\nrelations: %d\ndefault_type: %s", r.Schema.SchemaVersion, typeCount, relationCount, r.Schema.Defaults.Type)
}

func (r SchemaGraphResult) HumanText(Colorizer) string {
	return r.Mermaid
}

func (r SchemaDocsResult) HumanText(Colorizer) string {
	return r.Markdown
}

func (r SchemaMigrateResult) HumanText(Colorizer) string {
	if len(r.Changes) == 0 {
		if r.DryRun {
			return "no matching tasks"
		}
		return "schema migrate: no changes"
	}
	verb := "migrated"
	if r.DryRun {
		verb = "would migrate"
	}
	lines := make([]string, 0, len(r.Changes))
	for _, change := range r.Changes {
		lines = append(lines, fmt.Sprintf("%s %s %s->%s %s", verb, change.ID, change.From, change.To, change.File))
	}
	return strings.Join(lines, "\n")
}

func (a *App) runSchemaValidate() (SchemaValidateResult, error) {
	sch, err := a.loadProjectSchema()
	if err != nil {
		return SchemaValidateResult{}, err
	}
	result := sch.Validate()
	out := SchemaValidateResult{
		Valid:      result.Valid(),
		Violations: convertSchemaDiagnostics(result.Violations),
		Warnings:   convertSchemaDiagnostics(result.Warnings),
	}
	if !result.Valid() {
		return out, syaerr.SchemaInvalid{
			Message:    "schema invalid",
			Violations: diagnosticsAsViolations(result.Violations),
		}
	}
	return out, nil
}

func (a *App) runSchemaShow() (SchemaShowResult, error) {
	sch, err := a.loadProjectSchema()
	if err != nil {
		return SchemaShowResult{}, err
	}
	return SchemaShowResult{Schema: sch}, nil
}

func (a *App) runSchemaGraph(typeName string) (SchemaGraphResult, error) {
	sch, err := a.loadProjectSchema()
	if err != nil {
		return SchemaGraphResult{}, err
	}
	mermaid, err := renderSchemaGraph(sch, typeName)
	if err != nil {
		return SchemaGraphResult{}, err
	}
	return SchemaGraphResult{Type: typeName, Mermaid: mermaid}, nil
}

func (a *App) runSchemaDocs(typeName string) (SchemaDocsResult, error) {
	sch, err := a.loadProjectSchema()
	if err != nil {
		return SchemaDocsResult{}, err
	}
	markdown, err := renderSchemaDocs(sch, typeName)
	if err != nil {
		return SchemaDocsResult{}, err
	}
	return SchemaDocsResult{Type: typeName, Markdown: markdown}, nil
}

func (a *App) runSchemaMigrate(opts schemaMigrateOptions) (SchemaMigrateResult, error) {
	if strings.TrimSpace(opts.RenameStatus) == "" {
		return SchemaMigrateResult{}, syaerr.Usage{Message: "schema migrate requires --rename-status old=new"}
	}
	oldStatus, newStatus, err := parseRenameStatus(opts.RenameStatus)
	if err != nil {
		return SchemaMigrateResult{}, err
	}
	state, err := a.loadProject()
	if err != nil {
		return SchemaMigrateResult{}, err
	}
	if opts.Type != "" {
		typeDef, ok := state.Schema.Types[opts.Type]
		if !ok {
			return SchemaMigrateResult{}, syaerr.Usage{Message: "unknown task type: " + opts.Type}
		}
		if !stringIn(typeDef.Pipeline, newStatus) {
			return SchemaMigrateResult{}, syaerr.Usage{Message: "new status is not in pipeline for type " + opts.Type + ": " + newStatus}
		}
	}
	var changes []SchemaMigrateChange
	var affected []*task.Task
	for _, t := range state.Index.All() {
		if opts.Type != "" && t.Type != opts.Type {
			continue
		}
		if t.Status != oldStatus {
			continue
		}
		typeDef, ok := state.Schema.Types[t.Type]
		if !ok {
			return SchemaMigrateResult{}, syaerr.SchemaInvalid{Message: "unknown task type: " + t.Type}
		}
		if !stringIn(typeDef.Pipeline, newStatus) {
			return SchemaMigrateResult{}, syaerr.Usage{Message: "new status is not in pipeline for type " + t.Type + ": " + newStatus}
		}
		changes = append(changes, SchemaMigrateChange{
			ID:     t.ID,
			Type:   t.Type,
			File:   t.File,
			From:   oldStatus,
			To:     newStatus,
			Status: newStatus,
		})
		affected = append(affected, t)
	}
	if opts.DryRun {
		return SchemaMigrateResult{Changes: changes, DryRun: opts.DryRun}, nil
	}
	for _, t := range affected {
		t.Status = newStatus
		t.SchemaVersion = state.Schema.SchemaVersion
		if err := appendTaskLog(t, a.now(), a.Actor(), fmt.Sprintf("migrated: status %s->%s", oldStatus, newStatus)); err != nil {
			return SchemaMigrateResult{}, err
		}
		if err := writeTaskRaw(state.Project.Root, t); err != nil {
			return SchemaMigrateResult{}, err
		}
	}
	return SchemaMigrateResult{Changes: changes, DryRun: opts.DryRun}, nil
}

func parseRenameStatus(raw string) (string, string, error) {
	left, right, ok := strings.Cut(raw, "=")
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if !ok || left == "" || right == "" {
		return "", "", syaerr.Usage{Message: "expected --rename-status old=new"}
	}
	if left == right {
		return "", "", syaerr.Usage{Message: "old and new status must differ"}
	}
	return left, right, nil
}

func (a *App) loadProjectSchema() (*schema.Schema, error) {
	project, err := a.DiscoverProject()
	if err != nil {
		return nil, err
	}
	schemaBytes, err := os.ReadFile(filepath.Join(project.SyaDir, "schema.yml"))
	if err != nil {
		return nil, err
	}
	return schema.Parse(schemaBytes)
}

func convertSchemaDiagnostics(diagnostics []schema.Diagnostic) []SchemaDiagnostic {
	out := make([]SchemaDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		out = append(out, SchemaDiagnostic{
			Kind:    diagnostic.Kind,
			Path:    diagnostic.Path,
			Message: diagnostic.Message,
			Hint:    diagnostic.Hint,
		})
	}
	return out
}

func diagnosticsAsViolations(diagnostics []schema.Diagnostic) []syaerr.Violation {
	out := make([]syaerr.Violation, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		out = append(out, syaerr.Violation{
			Kind:    diagnostic.Kind,
			File:    diagnostic.Path,
			Message: diagnostic.Message,
			Hint:    diagnostic.Hint,
		})
	}
	return out
}

func renderSchemaGraph(sch *schema.Schema, onlyType string) (string, error) {
	var b strings.Builder
	b.WriteString("stateDiagram-v2\n")
	for _, typeName := range selectedTypeNames(sch, onlyType) {
		typeDef := sch.Types[typeName]
		if len(sch.Types) > 1 || onlyType == "" {
			fmt.Fprintf(&b, "  state %s {\n", typeName)
			writeTypeGraph(&b, typeDef, "    ")
			b.WriteString("  }\n")
		} else {
			writeTypeGraph(&b, typeDef, "  ")
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func writeTypeGraph(b *strings.Builder, typeDef schema.TypeDef, indent string) {
	if len(typeDef.Pipeline) > 0 {
		fmt.Fprintf(b, "%s[*] --> %s\n", indent, typeDef.Pipeline[0])
	}
	transitions, err := schema.ExpandTransitions(typeDef)
	if err != nil {
		return
	}
	for _, transition := range transitions {
		arrow := "-->"
		if transition.Kind == schema.TransitionSetback {
			arrow = "-.->"
		}
		label := string(transition.Kind)
		if transition.Description != "" {
			label += ": " + transition.Description
		}
		fmt.Fprintf(b, "%s%s %s %s: %s\n", indent, transition.From, arrow, transition.To, label)
	}
	for _, status := range typeDef.Terminal {
		fmt.Fprintf(b, "%s%s --> [*]\n", indent, status)
	}
}

func renderSchemaDocs(sch *schema.Schema, onlyType string) (string, error) {
	var b strings.Builder
	b.WriteString("# Schema\n\n")
	if strings.TrimSpace(sch.Description) != "" {
		b.WriteString(strings.TrimSpace(sch.Description))
		b.WriteString("\n\n")
	}
	writeRelationsDocs(&b, sch)
	for _, typeName := range selectedTypeNames(sch, onlyType) {
		typeDef := sch.Types[typeName]
		fmt.Fprintf(&b, "\n## %s\n\n", typeName)
		if strings.TrimSpace(typeDef.Description) != "" {
			b.WriteString(strings.TrimSpace(typeDef.Description))
			b.WriteString("\n\n")
		}
		writeStatusDocs(&b, typeDef)
		writeTransitionDocs(&b, typeDef)
		graph, err := renderSchemaGraph(&schema.Schema{Types: map[string]schema.TypeDef{typeName: typeDef}}, typeName)
		if err != nil {
			return "", err
		}
		b.WriteString("\n```mermaid\n")
		b.WriteString(graph)
		b.WriteString("\n```\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func writeRelationsDocs(b *strings.Builder, sch *schema.Schema) {
	if len(sch.Relations) == 0 {
		return
	}
	b.WriteString("## Relations\n\n")
	b.WriteString("| relation | reverse | graph | blocking | from | to | description |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, name := range sortedSchemaMapKeys(sch.Relations) {
		relation := sch.Relations[name]
		fmt.Fprintf(b, "| %s | %s | %s | %t | %s | %s | %s |\n",
			name, relation.Reverse, relation.Graph, relation.Blocking, strings.Join(relation.From, ", "), strings.Join(relation.To, ", "), tableCell(relation.Description))
	}
}

func writeStatusDocs(b *strings.Builder, typeDef schema.TypeDef) {
	b.WriteString("| status | description | flags |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, status := range typeDef.Pipeline {
		var flags []string
		if stringIn(typeDef.Terminal, status) {
			flags = append(flags, "terminal")
		}
		if stringIn(typeDef.Working, status) {
			flags = append(flags, "working")
		}
		if stringIn(typeDef.Parked, status) {
			flags = append(flags, "parked")
		}
		fmt.Fprintf(b, "| %s | %s | %s |\n", status, tableCell(typeDef.Statuses[status]), strings.Join(flags, ", "))
	}
}

func writeTransitionDocs(b *strings.Builder, typeDef schema.TypeDef) {
	b.WriteString("\n| from | to | kind | guards | description |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	transitions, err := schema.ExpandTransitions(typeDef)
	if err != nil {
		return
	}
	for _, transition := range transitions {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n",
			transition.From, transition.To, transition.Kind, tableCell(guardsSummary(transition.Guards)), tableCell(transition.Description))
	}
}

func guardsSummary(guards []schema.Guard) string {
	if len(guards) == 0 {
		return ""
	}
	parts := make([]string, 0, len(guards))
	for _, guard := range guards {
		part := string(guard.Kind)
		if guard.Message != "" {
			part += ": " + guard.Message
		}
		if guard.Hint != "" {
			part += " (hint: " + guard.Hint + ")"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}

func selectedTypeNames(sch *schema.Schema, onlyType string) []string {
	if onlyType != "" {
		if _, ok := sch.Types[onlyType]; !ok {
			return nil
		}
		return []string{onlyType}
	}
	return sortedSchemaMapKeys(sch.Types)
}

func sortedSchemaMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func tableCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}
