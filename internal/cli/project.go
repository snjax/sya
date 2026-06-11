package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/goccy/go-yaml"
	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/schema"
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

func atomicWriteFile(name string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(name)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, name)
}

func appendGitignoreWisps(root string) error {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		if strings.TrimSpace(string(line)) == ".sya/wisps/" || strings.TrimSpace(string(line)) == ".sya/wisps" {
			return nil
		}
	}
	var out []byte
	out = append(out, data...)
	if len(out) > 0 && !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	out = append(out, []byte(".sya/wisps/\n")...)
	return atomicWriteFile(path, out, 0o644)
}

func slugify(title string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r > unicode.MaxASCII:
			continue
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if b.Len() > 0 && !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
		if b.Len() >= 40 {
			break
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "task"
	}
	return slug
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
	return atomicWriteFile(filepath.Join(root, t.File), data, 0o644)
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
