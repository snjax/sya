package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts createOptions
		cmd := app.command("create \"Title\"", "Create a task", nil, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			if opts.FromFile == "" {
				if len(args) != 1 {
					return nil, syaerr.Usage{Message: "create requires exactly one title"}
				}
				opts.Title = args[0]
				opts.DescriptionProvided = cmd.Flags().Changed("description") || cmd.Flags().Changed("description-file")
			} else if len(args) != 0 {
				return nil, syaerr.Usage{Message: "create --from-file does not accept title args"}
			}
			return app.withProjectMutationLock(func() (any, error) {
				return app.runCreate(opts)
			})
		})
		cmd.Flags().StringVarP(&opts.Type, "type", "t", "", "task type")
		cmd.Flags().StringVarP(&opts.Priority, "priority", "p", "normal", "priority")
		cmd.Flags().StringVar(&opts.Parent, "parent", "", "parent task id")
		cmd.Flags().Var(&opts.Relations, "rel", "relation=ID")
		cmd.Flags().Var(&opts.DependsOn, "depends-on", "depends_on target id")
		cmd.Flags().Var(&opts.DiscoveredFrom, "discovered-from", "discovered_from target id")
		cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "description markdown")
		cmd.Flags().StringVar(&opts.DescriptionFile, "description-file", "", "description file path or -")
		cmd.Flags().StringVar(&opts.FromFile, "from-file", "", "YAML list batch file path or -")
		cmd.Flags().Var(&opts.Fields, "field", "field key=value")
		return cmd
	})
}

type createOptions struct {
	Title               string
	Type                string
	Priority            string
	Parent              string
	Relations           stringList
	DependsOn           stringList
	DiscoveredFrom      stringList
	Description         string
	DescriptionFile     string
	DescriptionProvided bool
	FromFile            string
	Fields              stringList
}

type batchCreateSpec struct {
	Title               string              `yaml:"title" json:"title"`
	Type                string              `yaml:"type" json:"type"`
	Priority            string              `yaml:"priority" json:"priority"`
	Parent              string              `yaml:"parent" json:"parent"`
	Relations           map[string][]string `yaml:"relations" json:"relations"`
	Description         string              `yaml:"description" json:"description"`
	DescriptionProvided bool                `yaml:"-" json:"-"`
	Fields              map[string]any      `yaml:"fields" json:"fields"`
}

type CreateResult struct {
	ID        string              `json:"id"`
	File      string              `json:"file"`
	Relations map[string][]string `json:"relations,omitempty"`
}

type CreateResults struct {
	Tasks []CreateResult `json:"tasks"`
}

func (r CreateResult) HumanText(Colorizer) string {
	return fmt.Sprintf("created %s %s", r.ID, r.File)
}

func (r CreateResults) HumanText(Colorizer) string {
	lines := make([]string, 0, len(r.Tasks))
	for _, created := range r.Tasks {
		lines = append(lines, created.HumanText(Colorizer{}))
	}
	return strings.Join(lines, "\n")
}

func (a *App) runCreate(opts createOptions) (any, error) {
	state, err := a.loadProject()
	if err != nil {
		return nil, err
	}
	if opts.FromFile != "" {
		specs, err := a.readBatchCreate(opts.FromFile)
		if err != nil {
			return nil, err
		}
		results := CreateResults{Tasks: make([]CreateResult, 0, len(specs))}
		for _, spec := range specs {
			result, err := a.createOne(state, spec)
			if err != nil {
				return nil, err
			}
			results.Tasks = append(results.Tasks, result)
		}
		return results, nil
	}
	spec, err := a.createSpecFromOptions(opts)
	if err != nil {
		return nil, err
	}
	return a.createOne(state, spec)
}

func (a *App) createSpecFromOptions(opts createOptions) (batchCreateSpec, error) {
	if opts.Description != "" && opts.DescriptionFile != "" {
		return batchCreateSpec{}, syaerr.Usage{Message: "--description and --description-file are mutually exclusive"}
	}
	description := opts.Description
	if opts.DescriptionFile != "" {
		data, err := readInputFile(opts.DescriptionFile, a.in)
		if err != nil {
			return batchCreateSpec{}, err
		}
		description = string(data)
	}
	fields := make(map[string]any)
	for _, raw := range opts.Fields {
		key, value, err := parseKeyValue(raw)
		if err != nil {
			return batchCreateSpec{}, err
		}
		if _, exists := fields[key]; exists {
			return batchCreateSpec{}, syaerr.Usage{Message: "duplicate field flag: " + key}
		}
		fields[key] = parseScalar(value)
	}
	relations, err := parseCreateRelations(opts.Relations, opts.DependsOn, opts.DiscoveredFrom)
	if err != nil {
		return batchCreateSpec{}, err
	}
	return batchCreateSpec{
		Title:               opts.Title,
		Type:                opts.Type,
		Priority:            opts.Priority,
		Parent:              opts.Parent,
		Relations:           relations,
		Description:         description,
		DescriptionProvided: opts.DescriptionProvided,
		Fields:              fields,
	}, nil
}

func (a *App) readBatchCreate(path string) ([]batchCreateSpec, error) {
	data, err := readInputFile(path, a.in)
	if err != nil {
		return nil, err
	}
	var specs []batchCreateSpec
	if err := yaml.Unmarshal(data, &specs); err != nil {
		return nil, syaerr.Usage{Message: err.Error()}
	}
	for i := range specs {
		if specs[i].Priority == "" {
			specs[i].Priority = "normal"
		}
	}
	return specs, nil
}

func parseCreateRelations(rawRels, dependsOn, discoveredFrom []string) (map[string][]string, error) {
	relations := make(map[string][]string)
	seen := make(map[string]struct{})
	add := func(relation, id string) error {
		relation = strings.TrimSpace(relation)
		id = strings.TrimSpace(id)
		if relation == "" || id == "" {
			return syaerr.Usage{Message: "relation and id are required"}
		}
		key := relation + "\x00" + id
		if _, ok := seen[key]; ok {
			return syaerr.Usage{Message: "duplicate relation flag: " + relation + "=" + id}
		}
		seen[key] = struct{}{}
		relations[relation] = append(relations[relation], id)
		return nil
	}
	for _, raw := range rawRels {
		relation, id, err := parseKeyValue(raw)
		if err != nil {
			return nil, err
		}
		if err := add(relation, id); err != nil {
			return nil, err
		}
	}
	for _, id := range dependsOn {
		if err := add("depends_on", id); err != nil {
			return nil, err
		}
	}
	for _, id := range discoveredFrom {
		if err := add("discovered_from", id); err != nil {
			return nil, err
		}
	}
	for relation := range relations {
		sort.Strings(relations[relation])
	}
	return relations, nil
}

func (a *App) createOne(state *projectState, spec batchCreateSpec) (CreateResult, error) {
	if strings.TrimSpace(spec.Title) == "" {
		return CreateResult{}, syaerr.Usage{Message: "title is required"}
	}
	taskType := spec.Type
	if taskType == "" {
		taskType = defaultTaskType(state.Schema)
	}
	typeDef, ok := state.Schema.Types[taskType]
	if !ok {
		return CreateResult{}, syaerr.Usage{Message: "unknown task type: " + taskType}
	}
	status, err := initialStatus(typeDef)
	if err != nil {
		return CreateResult{}, err
	}
	if spec.Priority == "" {
		spec.Priority = "normal"
	}
	if (spec.DescriptionProvided || spec.Description != "") && !typeDeclaresSection(typeDef, "Description") {
		return CreateResult{}, syaerr.Usage{
			Message: fmt.Sprintf("type %q does not declare section %q; --description requires that section in schema types.%s.sections", taskType, "Description", taskType),
		}
	}
	fields, err := validateFields(typeDef, spec.Fields)
	if err != nil {
		return CreateResult{}, err
	}
	relations, err := validateCreateRelations(state.Index, state.Schema, taskType, spec.Relations)
	if err != nil {
		return CreateResult{}, err
	}
	id, err := a.newID(a.existingIDs(state.Index), configuredIDLength(state.Project))
	if err != nil {
		return CreateResult{}, err
	}
	parentID, err := validateParentWithChildID(state.Index, state.Schema, taskType, spec.Parent, id)
	if err != nil {
		return CreateResult{}, err
	}
	name := id
	if slug := slugify(spec.Title); slug != "" {
		name += "-" + slug
	}
	file := filepath.ToSlash(filepath.Join(".sya", "tasks", name+".md"))
	created := a.now().UTC()
	t := &task.Task{
		ID:            id,
		Type:          taskType,
		Title:         spec.Title,
		Status:        status,
		Priority:      spec.Priority,
		Parent:        parentID,
		Relations:     relations,
		Fields:        fields,
		Created:       created,
		SchemaVersion: state.Schema.SchemaVersion,
		Body:          createBody(typeDef, spec.Description),
		File:          file,
	}
	if t.Relations == nil {
		t.Relations = make(map[string][]string)
	}
	if t.Fields == nil {
		t.Fields = make(map[string]any)
	}
	if err := appendTaskLog(t, created, a.Actor(), "created"); err != nil {
		return CreateResult{}, err
	}
	if err := writeTask(state, t); err != nil {
		return CreateResult{}, err
	}
	state.Index.Add(t)
	return CreateResult{ID: id, File: file, Relations: relations}, nil
}

func validateParent(idx *index.Index, sch *schema.Schema, childType, parent string) (string, error) {
	return validateParentWithChildID(idx, sch, childType, parent, "")
}

func validateParentWithChildID(idx *index.Index, sch *schema.Schema, childType, parent, childID string) (string, error) {
	if parent == "" {
		return "", nil
	}
	parentTask, err := idx.Resolve(parent)
	if err != nil {
		return "", err
	}
	if childID != "" && parentTask.ID == childID {
		return "", parentCycle{Task: childID, Parent: parentTask.ID}
	}
	parentType := sch.Types[parentTask.Type]
	if !parentType.Container {
		return "", syaerr.Usage{Message: "parent is not a container: " + parentTask.ID}
	}
	if !stringIn(parentType.Children, childType) {
		return "", syaerr.Usage{Message: "parent type does not allow child type: " + childType}
	}
	if childID != "" {
		for ancestorID := parentTask.Parent; ancestorID != ""; {
			if ancestorID == childID {
				return "", parentCycle{Task: childID, Parent: parentTask.ID}
			}
			ancestor, err := idx.Get(ancestorID)
			if err != nil {
				break
			}
			ancestorID = ancestor.Parent
		}
	}
	return parentTask.ID, nil
}

func validateFields(typeDef schema.TypeDef, raw map[string]any) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	fields := make(map[string]any, len(raw))
	for key, value := range raw {
		field, ok := declaredField(typeDef, key)
		if !ok {
			return nil, syaerr.Usage{Message: "field is not declared for type: " + key}
		}
		parsed, err := coerceCreateFieldValue(field, value)
		if err != nil {
			return nil, err
		}
		fields[key] = parsed
	}
	return fields, nil
}

func coerceCreateFieldValue(field schema.FieldDef, value any) (any, error) {
	switch field.Type {
	case "", "string":
		return fmt.Sprint(value), nil
	case "bool":
		parsed, ok := value.(bool)
		if !ok {
			return nil, syaerr.Usage{Message: "field expects bool"}
		}
		return parsed, nil
	case "int":
		parsed, ok := value.(int)
		if !ok {
			return nil, syaerr.Usage{Message: "field expects int"}
		}
		return parsed, nil
	case "enum":
		str := fmt.Sprint(value)
		if !stringIn(field.Values, str) {
			return nil, syaerr.Usage{Message: "field value not in enum: " + str}
		}
		return str, nil
	default:
		return value, nil
	}
}

func configuredIDLength(project Project) int {
	cfg, err := loadConfig(project)
	if err != nil {
		return task.DefaultIDLength
	}
	return task.ClampIDLength(cfg.IDLength)
}

func validateCreateRelations(idx *index.Index, sch *schema.Schema, sourceType string, raw map[string][]string) (map[string][]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	relations := make(map[string][]string, len(raw))
	for relation, ids := range raw {
		relationDef, ok := sch.Relations[relation]
		if !ok {
			return nil, syaerr.Usage{Message: "unknown relation: " + relation}
		}
		if len(relationDef.From) > 0 && !typeAllowed(sourceType, relationDef.From) {
			return nil, syaerr.Usage{Message: "relation " + relation + " cannot originate from type " + sourceType}
		}
		for _, rawID := range ids {
			target, err := idx.Resolve(rawID)
			if err != nil {
				return nil, err
			}
			if len(relationDef.To) > 0 && !typeAllowed(target.Type, relationDef.To) {
				return nil, syaerr.Usage{Message: "relation " + relation + " cannot target type " + target.Type}
			}
			if !stringIn(relations[relation], target.ID) {
				relations[relation] = append(relations[relation], target.ID)
			}
		}
		sort.Strings(relations[relation])
	}
	return relations, nil
}

func typeDeclaresSection(typeDef schema.TypeDef, name string) bool {
	for _, section := range typeDef.Sections {
		if section == name {
			return true
		}
	}
	return false
}

func createBody(typeDef schema.TypeDef, description string) task.Body {
	raw := make([]byte, 0)
	sections := make([]task.Section, 0, len(typeDef.Sections))
	for i, name := range typeDef.Sections {
		content := ""
		if name == "Description" && description != "" {
			content = strings.TrimRight(description, "\n")
		}
		sectionRaw := []byte(fmt.Sprintf("## %s\n%s", name, content))
		if content != "" {
			sectionRaw = append(sectionRaw, '\n')
		}
		if i+1 < len(typeDef.Sections) {
			sectionRaw = append(sectionRaw, '\n')
		}
		raw = append(raw, sectionRaw...)
		sections = append(sections, task.Section{Name: name, Raw: sectionRaw})
	}
	return task.NewBody(raw, sections)
}

func readInputFile(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return readAll(stdin)
	}
	return os.ReadFile(path)
}

func typeAllowed(taskType string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		if item == "*" || item == taskType {
			return true
		}
	}
	return false
}

func stringIn(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
