package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/snjax/sya/internal/fsutil"
	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/slug"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
)

type projectState struct {
	Project Project
	Schema  *schema.Schema
	Index   *index.Index
}

type configFile struct {
	Project  string `yaml:"project"`
	Prefix   string `yaml:"prefix"`
	IDLength int    `yaml:"id_length"`
	Archive  struct {
		AfterDays int `yaml:"after_days"`
	} `yaml:"archive"`
	Alerts struct {
		DeniedTransition string `yaml:"denied_transition"`
		DoctorViolation  string `yaml:"doctor_violation"`
	} `yaml:"alerts"`
}

func (a *App) loadProject() (*projectState, error) {
	project, err := a.DiscoverProject()
	if err != nil {
		return nil, err
	}
	schemaBytes, err := os.ReadFile(filepath.Join(project.SyaDir, "schema.yml"))
	if err != nil {
		return nil, err
	}
	sch, err := schema.Parse(schemaBytes)
	if err != nil {
		return nil, err
	}
	idx, err := index.Load(os.DirFS(project.Root), ".sya", sch)
	if err != nil {
		return nil, err
	}
	a.warnQuarantined(idx)
	return &projectState{Project: project, Schema: sch, Index: idx}, nil
}

func (a *App) warnQuarantined(idx *index.Index) {
	if a == nil || idx == nil || a.quiet || a.warnedIndex {
		return
	}
	count := len(idx.Quarantined())
	if count == 0 {
		return
	}
	fmt.Fprintf(a.err, "warning: %d quarantined task file(s), run sya doctor\n", count)
	a.warnedIndex = true
}

func loadConfig(project Project) (configFile, error) {
	var cfg configFile
	data, err := os.ReadFile(filepath.Join(project.SyaDir, "config.yml"))
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (a *App) existingIDs(idx *index.Index) map[string]struct{} {
	existing := make(map[string]struct{})
	for _, t := range idx.All() {
		existing[t.ID] = struct{}{}
	}
	return existing
}

func initialStatus(typeDef schema.TypeDef) (string, error) {
	if len(typeDef.Pipeline) == 0 {
		return "", syaerr.SchemaInvalid{Message: "type pipeline is empty"}
	}
	return typeDef.Pipeline[0], nil
}

func defaultTaskType(sch *schema.Schema) string {
	if sch != nil && sch.Defaults.Type != "" {
		return sch.Defaults.Type
	}
	return "task"
}

func appendGitignoreRuntime(root string) error {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	required := []string{".sya/events.jsonl", ".sya/wisps/", ".sya/.lock"}
	present := make(map[string]bool, len(required))
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := strings.TrimSpace(string(line))
		if trimmed == ".sya/wisps" {
			trimmed = ".sya/wisps/"
		}
		present[trimmed] = true
	}
	var missing []string
	for _, entry := range required {
		if !present[entry] {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	var out []byte
	out = append(out, data...)
	if len(out) > 0 && !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	for _, entry := range missing {
		out = append(out, []byte(entry+"\n")...)
	}
	return fsutil.AtomicWriteFile(path, out, 0o644)
}

func slugify(title string) string {
	return slug.Make(title)
}

func parseKeyValue(raw string) (string, string, error) {
	key, value, ok := strings.Cut(raw, "=")
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if !ok || key == "" || value == "" {
		return "", "", syaerr.Usage{Message: "expected key=value"}
	}
	return key, value, nil
}

func parseScalar(value string) any {
	switch strings.ToLower(value) {
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}
	return value
}

func writeTask(state *projectState, t *task.Task) error {
	if state == nil || state.Schema == nil {
		return syaerr.SchemaConformance{Task: taskID(t), Path: taskFile(t)}
	}
	if violations := taskConformanceViolations(state.Schema, t); len(violations) > 0 {
		return syaerr.SchemaConformance{Task: t.ID, Path: t.File, Violations: violations}
	}
	t.SchemaVersion = state.Schema.SchemaVersion
	return writeTaskRaw(state.Project.Root, t)
}

func writeTaskRaw(root string, t *task.Task) error {
	data, err := task.Serialize(t)
	if err != nil {
		return err
	}
	return fsutil.AtomicWriteFile(filepath.Join(root, t.File), data, 0o644)
}

func taskConformanceViolations(sch *schema.Schema, t *task.Task) []syaerr.Violation {
	var violations []syaerr.Violation
	if t == nil {
		return []syaerr.Violation{{Kind: "task_nil", Message: "task is nil", Hint: "run sya doctor"}}
	}
	typeDef, ok := sch.Types[t.Type]
	if !ok {
		return []syaerr.Violation{{
			Kind:    "task_type_unknown",
			Message: fmt.Sprintf("task type %q is not declared in schema", t.Type),
			Hint:    "run sya doctor or sya schema migrate",
		}}
	}
	if !stringIn(typeDef.Pipeline, t.Status) {
		violations = append(violations, syaerr.Violation{
			Kind:    "task_status_unknown",
			Message: fmt.Sprintf("status %q is not in pipeline for type %q", t.Status, t.Type),
			Hint:    "run sya doctor or sya schema migrate",
		})
	}
	for name, value := range t.Fields {
		fieldDef, ok := typeDef.Fields[name]
		if !ok {
			violations = append(violations, syaerr.Violation{
				Kind:    "field_unknown",
				Field:   name,
				Message: fmt.Sprintf("field %q is not declared for type %q", name, t.Type),
				Hint:    "run sya doctor or sya schema migrate",
			})
			continue
		}
		if !storedFieldMatches(fieldDef, value) {
			violations = append(violations, syaerr.Violation{
				Kind:    "field_type_invalid",
				Field:   name,
				Message: fmt.Sprintf("field %q does not match declared type %q", name, fieldDef.Type),
				Hint:    "run sya doctor or sya schema migrate",
			})
		}
	}
	allowedSections := make(map[string]bool, len(typeDef.Sections)+1)
	for _, section := range typeDef.Sections {
		allowedSections[section] = true
	}
	allowedSections["Log"] = true
	for _, section := range t.Body.Sections {
		if section.Name == "" || allowedSections[section.Name] {
			continue
		}
		violations = append(violations, syaerr.Violation{
			Kind:    "section_unknown",
			Section: section.Name,
			Message: fmt.Sprintf("section %q is not declared for type %q", section.Name, t.Type),
			Hint:    "run sya doctor or sya schema migrate",
		})
	}
	return violations
}

func storedFieldMatches(field schema.FieldDef, value any) bool {
	switch field.Type {
	case "", "any":
		return true
	case "bool":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "enum":
		text, ok := value.(string)
		return ok && stringIn(field.Values, text)
	case "int":
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return true
		default:
			return false
		}
	default:
		if len(field.Values) > 0 {
			return storedFieldMatches(schema.FieldDef{Type: "enum", Values: field.Values}, value)
		}
		return true
	}
}

func (a *App) withProjectMutationLock(fn func() (any, error)) (any, error) {
	project, err := a.DiscoverProject()
	if err != nil {
		return nil, err
	}
	var data any
	err = fsutil.WithProjectLock(project.Root, func() error {
		var innerErr error
		data, innerErr = fn()
		return innerErr
	})
	if err != nil {
		var timeout fsutil.LockTimeout
		if errors.As(err, &timeout) {
			return nil, syaerr.ProjectLocked{Path: timeout.Path}
		}
		return nil, err
	}
	return data, nil
}

func readAll(r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	return io.ReadAll(r)
}

func sectionText(section task.Section) string {
	raw := string(section.Raw)
	if section.Name == "" {
		return strings.TrimSpace(raw)
	}
	firstNewline := strings.IndexByte(raw, '\n')
	if firstNewline < 0 {
		return ""
	}
	return strings.TrimSpace(raw[firstNewline+1:])
}

func sortedRelationKeys(relations map[string][]string) []string {
	keys := make([]string, 0, len(relations))
	for key := range relations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type stringList []string

func (s *stringList) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func (s *stringList) Type() string {
	return "string"
}

func mustYAML(out any) string {
	data, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Sprint(out)
	}
	return strings.TrimSpace(string(data))
}
