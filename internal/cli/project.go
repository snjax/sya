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
	return &projectState{Project: project, Schema: sch, Index: idx}, nil
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

func writeTask(root string, t *task.Task) error {
	data, err := task.Serialize(t)
	if err != nil {
		return err
	}
	return fsutil.AtomicWriteFile(filepath.Join(root, t.File), data, 0o644)
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
