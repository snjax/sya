package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	syatemplate "github.com/snjax/sya/internal/template"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		cmd := &cobra.Command{Use: "template", Short: "Manage task templates", SilenceUsage: true}
		cmd.AddCommand(app.templateListCommand())
		cmd.AddCommand(app.templateShowCommand())
		cmd.AddCommand(app.templateApplyCommand())
		return cmd
	})
}

type TemplateListResult struct {
	Templates []syatemplate.Summary `json:"templates"`
}

func (r TemplateListResult) HumanText(Colorizer) string {
	if len(r.Templates) == 0 {
		return "no templates"
	}
	lines := make([]string, 0, len(r.Templates))
	for _, tmpl := range r.Templates {
		if tmpl.Description != "" {
			lines = append(lines, fmt.Sprintf("%s — %s", tmpl.Name, tmpl.Description))
		} else {
			lines = append(lines, tmpl.Name)
		}
	}
	return strings.Join(lines, "\n")
}

type TemplateShowResult struct {
	Template *syatemplate.Template `json:"template"`
}

func (r TemplateShowResult) HumanText(Colorizer) string {
	if r.Template == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", r.Template.Name)
	if r.Template.Description != "" {
		fmt.Fprintf(&b, "%s\n", r.Template.Description)
	}
	if len(r.Template.Params) > 0 {
		b.WriteString("params:\n")
		for _, param := range r.Template.Params {
			req := ""
			if param.Required {
				req = " required"
			}
			fmt.Fprintf(&b, "  %s%s", param.Name, req)
			if param.Default != nil {
				fmt.Fprintf(&b, " default=%v", param.Default)
			}
			if param.Description != "" {
				fmt.Fprintf(&b, " — %s", param.Description)
			}
			b.WriteByte('\n')
		}
	}
	b.WriteString("tasks:\n")
	for _, spec := range r.Template.Tasks {
		fmt.Fprintf(&b, "  %s: %s", spec.Key, spec.Title)
		if spec.Type != "" {
			fmt.Fprintf(&b, " (%s)", spec.Type)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

type templateApplyOptions struct {
	Name   string
	Params stringList
	Parent string
	DryRun bool
}

type TemplateApplyResult struct {
	Template string                         `json:"template"`
	DryRun   bool                           `json:"dry_run,omitempty"`
	Tasks    map[string]TemplateAppliedTask `json:"tasks"`
}

type TemplateAppliedTask struct {
	ID        string              `json:"id"`
	File      string              `json:"file"`
	Type      string              `json:"type"`
	Title     string              `json:"title"`
	Parent    string              `json:"parent,omitempty"`
	Relations map[string][]string `json:"relations,omitempty"`
}

func (r TemplateApplyResult) HumanText(Colorizer) string {
	keys := make([]string, 0, len(r.Tasks))
	for key := range r.Tasks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	prefix := "created"
	if r.DryRun {
		prefix = "would create"
	}
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		t := r.Tasks[key]
		lines = append(lines, fmt.Sprintf("%s %s -> %s %s", prefix, key, t.ID, t.File))
	}
	return strings.Join(lines, "\n")
}

type templateTaskPlan struct {
	Key          string
	ID           string
	File         string
	Type         string
	Title        string
	Parent       string
	Assignee     string
	Priority     string
	Labels       []string
	Fields       map[string]any
	Sections     map[string]string
	RawRelations map[string][]string
	Relations    map[string][]string
	Task         *task.Task
}

func (a *App) templateListCommand() *cobra.Command {
	return a.command("list", "List templates", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		project, err := a.DiscoverProject()
		if err != nil {
			return nil, err
		}
		templates, err := syatemplate.List(os.DirFS(project.Root), ".sya/templates")
		if err != nil {
			return nil, err
		}
		return TemplateListResult{Templates: templates}, nil
	})
}

func (a *App) templateShowCommand() *cobra.Command {
	return a.command("show <name>", "Show a template", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		project, err := a.DiscoverProject()
		if err != nil {
			return nil, err
		}
		tmpl, err := syatemplate.Load(os.DirFS(project.Root), ".sya/templates", args[0])
		if err != nil {
			return nil, err
		}
		return TemplateShowResult{Template: tmpl}, nil
	})
}

func (a *App) templateApplyCommand() *cobra.Command {
	var opts templateApplyOptions
	cmd := a.command("apply <name>", "Apply a template", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		opts.Name = args[0]
		if opts.DryRun {
			return a.runTemplateApply(opts)
		}
		return a.withProjectMutationLock(func() (any, error) {
			return a.runTemplateApply(opts)
		})
	})
	cmd.Flags().VarP(&opts.Params, "param", "p", "template param key=value")
	cmd.Flags().StringVar(&opts.Parent, "parent", "", "default parent for template tasks")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "print plan without writing tasks")
	return cmd
}

func (a *App) runTemplateApply(opts templateApplyOptions) (TemplateApplyResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return TemplateApplyResult{}, err
	}
	rawParams, err := templateParams(opts.Params)
	if err != nil {
		return TemplateApplyResult{}, err
	}
	tmpl, err := syatemplate.Load(os.DirFS(state.Project.Root), ".sya/templates", opts.Name)
	if err != nil {
		return TemplateApplyResult{}, err
	}
	applied, err := tmpl.Apply(rawParams)
	if err != nil {
		return TemplateApplyResult{}, err
	}
	plans, externalUpdates, err := a.buildTemplatePlan(state, applied, opts.Parent)
	if err != nil {
		return TemplateApplyResult{}, err
	}
	result := templateApplyResult(applied.Name, opts.DryRun, plans)
	if opts.DryRun {
		return result, nil
	}
	for _, plan := range plans {
		if err := writeTask(state, plan.Task); err != nil {
			return TemplateApplyResult{}, err
		}
		state.Index.Add(plan.Task)
	}
	for _, t := range externalUpdates {
		if err := writeTask(state, t); err != nil {
			return TemplateApplyResult{}, err
		}
	}
	return result, nil
}

func templateParams(values []string) (map[string]string, error) {
	out := make(map[string]string, len(values))
	for _, raw := range values {
		key, value, err := parseKeyValue(raw)
		if err != nil {
			return nil, err
		}
		if _, exists := out[key]; exists {
			return nil, syaerr.Usage{Message: "duplicate template param: " + key}
		}
		out[key] = value
	}
	return out, nil
}

func (a *App) buildTemplatePlan(state *projectState, tmpl *syatemplate.Template, defaultParent string) ([]templateTaskPlan, []*task.Task, error) {
	keyToID := make(map[string]string, len(tmpl.Tasks))
	keyToPlan := make(map[string]*templateTaskPlan, len(tmpl.Tasks))
	existing := a.existingIDs(state.Index)
	for _, spec := range tmpl.Tasks {
		id, err := a.newID(existing, configuredIDLength(state.Project))
		if err != nil {
			return nil, nil, err
		}
		existing[id] = struct{}{}
		keyToID[spec.Key] = id
	}

	plans := make([]templateTaskPlan, 0, len(tmpl.Tasks))
	for _, spec := range tmpl.Tasks {
		plan, err := buildTemplateTaskBase(state, spec, keyToID[spec.Key], defaultParent)
		if err != nil {
			return nil, nil, err
		}
		keyToPlan[plan.Key] = &plan
		plans = append(plans, plan)
	}
	for i := range plans {
		plan := &plans[i]
		keyToPlan[plan.Key] = plan
	}
	if err := resolveTemplateParents(state, plans, keyToPlan, keyToID); err != nil {
		return nil, nil, err
	}
	externalUpdates, err := resolveTemplateRelations(state, plans, keyToPlan, keyToID)
	if err != nil {
		return nil, nil, err
	}
	for i := range plans {
		plans[i].Task = templateTask(state, tmpl.Name, plans[i], a.Actor(), a.now().UTC())
	}
	return plans, externalUpdates, nil
}

func buildTemplateTaskBase(state *projectState, spec syatemplate.TaskSpec, id, defaultParent string) (templateTaskPlan, error) {
	taskType := spec.Type
	if taskType == "" {
		taskType = defaultTaskType(state.Schema)
	}
	typeDef, ok := state.Schema.Types[taskType]
	if !ok {
		return templateTaskPlan{}, syaerr.Usage{Message: "unknown task type: " + taskType}
	}
	if _, err := initialStatus(typeDef); err != nil {
		return templateTaskPlan{}, err
	}
	if strings.TrimSpace(spec.Title) == "" {
		return templateTaskPlan{}, syaerr.Usage{Message: "title is required for template key: " + spec.Key}
	}
	for section := range spec.Sections {
		if !typeDeclaresSection(typeDef, section) {
			return templateTaskPlan{}, syaerr.Usage{Message: fmt.Sprintf("type %q does not declare section %q", taskType, section)}
		}
	}
	fields, err := validateFields(typeDef, spec.Fields)
	if err != nil {
		return templateTaskPlan{}, err
	}
	priority := spec.Priority
	if priority == "" {
		priority = "normal"
	}
	parent := spec.Parent
	if parent == "" {
		parent = defaultParent
	}
	name := id
	if slug := slugify(spec.Title); slug != "" {
		name += "-" + slug
	}
	return templateTaskPlan{
		Key:          spec.Key,
		ID:           id,
		File:         filepath.ToSlash(filepath.Join(".sya", "tasks", name+".md")),
		Type:         taskType,
		Title:        spec.Title,
		Assignee:     spec.Assignee,
		Priority:     priority,
		Parent:       parent,
		Labels:       append([]string(nil), spec.Labels...),
		Fields:       fields,
		Sections:     spec.Sections,
		RawRelations: spec.Relations,
		Relations:    make(map[string][]string),
	}, nil
}

func resolveTemplateParents(state *projectState, plans []templateTaskPlan, keyToPlan map[string]*templateTaskPlan, keyToID map[string]string) error {
	for i := range plans {
		parent := plans[i].Parent
		if parent == "" {
			continue
		}
		if parentID, ok := keyToID[parent]; ok {
			parentPlan := keyToPlan[parent]
			parentType := state.Schema.Types[parentPlan.Type]
			if !parentType.Container {
				return syaerr.Usage{Message: "parent is not a container: " + parent}
			}
			if !stringIn(parentType.Children, plans[i].Type) {
				return syaerr.Usage{Message: "parent type does not allow child type: " + plans[i].Type}
			}
			plans[i].Parent = parentID
			continue
		}
		parentID, err := validateParentWithChildID(state.Index, state.Schema, plans[i].Type, parent, plans[i].ID)
		if err != nil {
			return err
		}
		plans[i].Parent = parentID
	}
	return nil
}

func resolveTemplateRelations(state *projectState, plans []templateTaskPlan, keyToPlan map[string]*templateTaskPlan, keyToID map[string]string) ([]*task.Task, error) {
	byID := make(map[string]*templateTaskPlan, len(plans))
	for i := range plans {
		byID[plans[i].ID] = &plans[i]
	}
	var pending []canonicalRelationEdge
	externalUpdates := make(map[string]*task.Task)
	for _, plan := range plans {
		for relation, refs := range plan.RawRelations {
			if _, ok := state.Schema.Relations[relation]; !ok {
				if _, _, _, err := canonicalRelation(state.Schema, plan.ID, relation, plan.ID); err != nil {
					return nil, err
				}
			}
			for _, ref := range refs {
				targetID, err := templateTargetID(state.Index, keyToID, ref)
				if err != nil {
					return nil, err
				}
				from, canonical, to, err := canonicalRelation(state.Schema, plan.ID, relation, targetID)
				if err != nil {
					return nil, err
				}
				if err := templateRelationTypeCheck(state, byID, from, canonical, to); err != nil {
					return nil, err
				}
				if state.Schema.Relations[canonical].Graph == "dag" && wouldCreateCycleWithPending(state.Index, pending, from, canonical, to) {
					return nil, syaerr.Usage{Message: "relation would create a cycle"}
				}
				pending = append(pending, canonicalRelationEdge{From: from, Relation: canonical, To: to})
				if source := byID[from]; source != nil {
					if !stringIn(source.Relations[canonical], to) {
						source.Relations[canonical] = append(source.Relations[canonical], to)
						sort.Strings(source.Relations[canonical])
					}
					continue
				}
				external, err := state.Index.Resolve(from)
				if err != nil {
					return nil, err
				}
				if external.Relations == nil {
					external.Relations = make(map[string][]string)
				}
				if !stringIn(external.Relations[canonical], to) {
					external.Relations[canonical] = append(external.Relations[canonical], to)
					sort.Strings(external.Relations[canonical])
					externalUpdates[external.ID] = external
				}
			}
		}
	}
	out := make([]*task.Task, 0, len(externalUpdates))
	for _, t := range externalUpdates {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func templateTargetID(idx *index.Index, keyToID map[string]string, ref string) (string, error) {
	if id, ok := keyToID[ref]; ok {
		return id, nil
	}
	t, err := idx.Resolve(ref)
	if err != nil {
		return "", err
	}
	return t.ID, nil
}

func templateRelationTypeCheck(state *projectState, planned map[string]*templateTaskPlan, from, relation, to string) error {
	def := state.Schema.Relations[relation]
	sourceType, err := templateTaskType(state.Index, planned, from)
	if err != nil {
		return err
	}
	targetType, err := templateTaskType(state.Index, planned, to)
	if err != nil {
		return err
	}
	if len(def.From) > 0 && !typeAllowed(sourceType, def.From) {
		return syaerr.Usage{Message: "relation " + relation + " cannot originate from type " + sourceType}
	}
	if len(def.To) > 0 && !typeAllowed(targetType, def.To) {
		return syaerr.Usage{Message: "relation " + relation + " cannot target type " + targetType}
	}
	return nil
}

func templateTaskType(idx *index.Index, planned map[string]*templateTaskPlan, id string) (string, error) {
	if plan := planned[id]; plan != nil {
		return plan.Type, nil
	}
	t, err := idx.Resolve(id)
	if err != nil {
		return "", err
	}
	return t.Type, nil
}

func templateTask(state *projectState, templateName string, plan templateTaskPlan, actor string, now time.Time) *task.Task {
	typeDef := state.Schema.Types[plan.Type]
	status, _ := initialStatus(typeDef)
	t := &task.Task{
		ID:            plan.ID,
		Type:          plan.Type,
		Title:         plan.Title,
		Status:        status,
		Priority:      plan.Priority,
		Parent:        plan.Parent,
		Assignee:      plan.Assignee,
		Labels:        append([]string(nil), plan.Labels...),
		Relations:     plan.Relations,
		Fields:        plan.Fields,
		Created:       now,
		SchemaVersion: state.Schema.SchemaVersion,
		Body:          templateBody(typeDef, plan.Sections),
		File:          plan.File,
	}
	_ = appendTaskLog(t, now, actor, "created from template "+templateName)
	return t
}

func templateBody(typeDef schema.TypeDef, contents map[string]string) task.Body {
	raw := make([]byte, 0)
	sections := make([]task.Section, 0, len(typeDef.Sections))
	for i, name := range typeDef.Sections {
		content := strings.TrimRight(contents[name], "\n")
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

func templateApplyResult(name string, dryRun bool, plans []templateTaskPlan) TemplateApplyResult {
	result := TemplateApplyResult{Template: name, DryRun: dryRun, Tasks: make(map[string]TemplateAppliedTask, len(plans))}
	for _, plan := range plans {
		result.Tasks[plan.Key] = TemplateAppliedTask{
			ID:        plan.ID,
			File:      plan.File,
			Type:      plan.Type,
			Title:     plan.Title,
			Parent:    plan.Parent,
			Relations: plan.Relations,
		}
	}
	return result
}
