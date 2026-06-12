package template

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/snjax/sya/internal/syaerr"
)

type Template struct {
	Name        string     `json:"name" yaml:"name"`
	Description string     `json:"description,omitempty" yaml:"description,omitempty"`
	Params      []Param    `json:"params,omitempty" yaml:"params,omitempty"`
	Tasks       []TaskSpec `json:"tasks" yaml:"tasks"`
	Path        string     `json:"path,omitempty" yaml:"-"`
}

type Param struct {
	Name        string `json:"name" yaml:"name"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Default     any    `json:"default,omitempty" yaml:"default,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type TaskSpec struct {
	Key       string              `json:"key" yaml:"key"`
	Type      string              `json:"type,omitempty" yaml:"type,omitempty"`
	Title     string              `json:"title" yaml:"title"`
	Assignee  string              `json:"assignee,omitempty" yaml:"assignee,omitempty"`
	Priority  string              `json:"priority,omitempty" yaml:"priority,omitempty"`
	Parent    string              `json:"parent,omitempty" yaml:"parent,omitempty"`
	Sections  map[string]string   `json:"sections,omitempty" yaml:"sections,omitempty"`
	Fields    map[string]any      `json:"fields,omitempty" yaml:"fields,omitempty"`
	Relations map[string][]string `json:"relations,omitempty" yaml:"relations,omitempty"`
	Labels    []string            `json:"labels,omitempty" yaml:"labels,omitempty"`
}

type Summary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path"`
}

func List(fsys fs.FS, dir string) ([]Summary, error) {
	templates, err := loadAll(fsys, dir)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(templates))
	for _, tmpl := range templates {
		out = append(out, Summary{Name: tmpl.Name, Description: tmpl.Description, Path: tmpl.Path})
	}
	return out, nil
}

func Load(fsys fs.FS, dir, name string) (*Template, error) {
	if strings.TrimSpace(name) == "" {
		return nil, syaerr.Usage{Message: "template name is required"}
	}
	tmplPath := path.Join(strings.Trim(dir, "/"), name+".yml")
	data, err := fs.ReadFile(fsys, tmplPath)
	if err != nil {
		return nil, syaerr.NotFound{ID: name}
	}
	tmpl, err := Parse(data)
	if err != nil {
		return nil, err
	}
	tmpl.Path = tmplPath
	if tmpl.Name == "" {
		tmpl.Name = name
	}
	return tmpl, nil
}

func Parse(data []byte) (*Template, error) {
	var tmpl Template
	if err := yaml.UnmarshalWithOptions(data, &tmpl, yaml.Strict()); err != nil {
		return nil, syaerr.Usage{Message: "template parse error: " + err.Error()}
	}
	if err := validateTemplate(&tmpl); err != nil {
		return nil, err
	}
	return &tmpl, nil
}

func (t Template) Apply(raw map[string]string) (*Template, error) {
	params, err := resolveParams(t.Params, raw)
	if err != nil {
		return nil, err
	}
	applied := t
	applied.Params = append([]Param(nil), t.Params...)
	applied.Tasks = make([]TaskSpec, len(t.Tasks))
	for i, spec := range t.Tasks {
		next, err := substituteTask(spec, params)
		if err != nil {
			return nil, err
		}
		applied.Tasks[i] = next
	}
	if err := checkCycles(applied.Tasks); err != nil {
		return nil, err
	}
	return &applied, nil
}

func loadAll(fsys fs.FS, dir string) ([]*Template, error) {
	dir = strings.Trim(dir, "/")
	var templates []*Template
	err := fs.WalkDir(fsys, dir, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if filePath == dir {
				return fs.SkipDir
			}
			return walkErr
		}
		if entry.IsDir() || path.Ext(filePath) != ".yml" {
			return nil
		}
		data, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			return err
		}
		tmpl, err := Parse(data)
		if err != nil {
			return fmt.Errorf("%s: %w", filePath, err)
		}
		tmpl.Path = filePath
		if tmpl.Name == "" {
			tmpl.Name = strings.TrimSuffix(path.Base(filePath), ".yml")
		}
		templates = append(templates, tmpl)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Name < templates[j].Name
	})
	return templates, nil
}

func validateTemplate(tmpl *Template) error {
	if strings.TrimSpace(tmpl.Name) == "" {
		return syaerr.Usage{Message: "template name is required"}
	}
	if len(tmpl.Tasks) == 0 {
		return syaerr.Usage{Message: "template tasks are required"}
	}
	paramNames := make(map[string]struct{}, len(tmpl.Params))
	for _, param := range tmpl.Params {
		if strings.TrimSpace(param.Name) == "" {
			return syaerr.Usage{Message: "template param name is required"}
		}
		if _, exists := paramNames[param.Name]; exists {
			return syaerr.Usage{Message: "duplicate template param: " + param.Name}
		}
		paramNames[param.Name] = struct{}{}
	}
	keys := make(map[string]struct{}, len(tmpl.Tasks))
	for _, spec := range tmpl.Tasks {
		if strings.TrimSpace(spec.Key) == "" {
			return syaerr.Usage{Message: "template task key is required"}
		}
		if _, exists := keys[spec.Key]; exists {
			return syaerr.Usage{Message: "duplicate template task key: " + spec.Key}
		}
		if strings.TrimSpace(spec.Title) == "" {
			return syaerr.Usage{Message: "template task title is required: " + spec.Key}
		}
		keys[spec.Key] = struct{}{}
	}
	return checkCycles(tmpl.Tasks)
}

func resolveParams(decls []Param, raw map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(decls))
	declared := make(map[string]Param, len(decls))
	for _, param := range decls {
		declared[param.Name] = param
		if value, ok := raw[param.Name]; ok {
			out[param.Name] = value
			continue
		}
		if param.Default != nil {
			out[param.Name] = fmt.Sprint(param.Default)
			continue
		}
		if param.Required {
			return nil, syaerr.Usage{Message: "missing required template param: " + param.Name}
		}
		out[param.Name] = ""
	}
	for name := range raw {
		if _, ok := declared[name]; !ok {
			return nil, syaerr.Usage{Message: "unknown template param: " + name}
		}
	}
	return out, nil
}

var placeholderRE = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

func substituteTask(spec TaskSpec, params map[string]string) (TaskSpec, error) {
	var err error
	spec.Title, err = substituteString(spec.Title, params)
	if err != nil {
		return spec, err
	}
	spec.Type, err = substituteString(spec.Type, params)
	if err != nil {
		return spec, err
	}
	spec.Assignee, err = substituteString(spec.Assignee, params)
	if err != nil {
		return spec, err
	}
	spec.Priority, err = substituteString(spec.Priority, params)
	if err != nil {
		return spec, err
	}
	spec.Parent, err = substituteString(spec.Parent, params)
	if err != nil {
		return spec, err
	}
	for i := range spec.Labels {
		spec.Labels[i], err = substituteString(spec.Labels[i], params)
		if err != nil {
			return spec, err
		}
	}
	spec.Sections, err = substituteStringMap(spec.Sections, params)
	if err != nil {
		return spec, err
	}
	spec.Fields, err = substituteAnyMap(spec.Fields, params)
	if err != nil {
		return spec, err
	}
	for rel, targets := range spec.Relations {
		for i := range targets {
			targets[i], err = substituteString(targets[i], params)
			if err != nil {
				return spec, err
			}
		}
		spec.Relations[rel] = targets
	}
	return spec, nil
}

func substituteStringMap(values map[string]string, params map[string]string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		next, err := substituteString(value, params)
		if err != nil {
			return nil, err
		}
		out[key] = next
	}
	return out, nil
}

func substituteAnyMap(values map[string]any, params map[string]string) (map[string]any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		next, err := substituteAny(value, params)
		if err != nil {
			return nil, err
		}
		out[key] = next
	}
	return out, nil
}

func substituteAny(value any, params map[string]string) (any, error) {
	switch typed := value.(type) {
	case string:
		return substituteString(typed, params)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			next, err := substituteAny(item, params)
			if err != nil {
				return nil, err
			}
			out[i] = next
		}
		return out, nil
	case map[string]any:
		return substituteAnyMap(typed, params)
	default:
		return value, nil
	}
}

func substituteString(value string, params map[string]string) (string, error) {
	var err error
	out := placeholderRE.ReplaceAllStringFunc(value, func(match string) string {
		if err != nil {
			return match
		}
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}"))
		reMatch := placeholderRE.FindStringSubmatch(match)
		if len(reMatch) == 2 {
			name = reMatch[1]
		}
		replacement, ok := params[name]
		if !ok {
			err = syaerr.Usage{Message: "unknown template placeholder: " + name}
			return match
		}
		return replacement
	})
	if err == nil && strings.Contains(out, "{{") && strings.Contains(out, "}}") {
		return "", syaerr.Usage{Message: "unknown template placeholder"}
	}
	return out, err
}

func checkCycles(tasks []TaskSpec) error {
	keys := make(map[string]struct{}, len(tasks))
	for _, spec := range tasks {
		keys[spec.Key] = struct{}{}
	}
	graph := make(map[string][]string)
	for _, spec := range tasks {
		if _, ok := keys[spec.Parent]; ok {
			graph[spec.Key] = append(graph[spec.Key], spec.Parent)
		}
		for _, targets := range spec.Relations {
			for _, target := range targets {
				if _, ok := keys[target]; ok {
					graph[spec.Key] = append(graph[spec.Key], target)
				}
			}
		}
	}
	visiting := make(map[string]bool)
	visited := make(map[string]bool)
	var visit func(string) error
	visit = func(key string) error {
		if visiting[key] {
			return syaerr.Usage{Message: "template contains cycle involving key: " + key}
		}
		if visited[key] {
			return nil
		}
		visiting[key] = true
		for _, next := range graph[key] {
			if err := visit(next); err != nil {
				return err
			}
		}
		visiting[key] = false
		visited[key] = true
		return nil
	}
	for key := range keys {
		if err := visit(key); err != nil {
			return err
		}
	}
	return nil
}
