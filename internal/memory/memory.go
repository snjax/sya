package memory

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/goccy/go-yaml"
	"github.com/snjax/sya/internal/syaerr"
)

type Note struct {
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Tasks       []string `json:"tasks,omitempty" yaml:"tasks,omitempty"`
	Body        string   `json:"body,omitempty" yaml:"-"`
	File        string   `json:"file,omitempty" yaml:"-"`
}

type frontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tasks       []string `yaml:"tasks"`
}

func Slug(text string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(text) {
		if repl, ok := cyrillicSlug[r]; ok {
			if repl == "" {
				continue
			}
			if b.Len()+len(repl) > 40 {
				break
			}
			b.WriteString(repl)
			lastHyphen = false
			continue
		}
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
	return strings.Trim(b.String(), "-")
}

func Load(fsys fs.FS, dir, name string) (Note, error) {
	name = Slug(name)
	if name == "" {
		return Note{}, syaerr.Usage{Message: "memory key is required"}
	}
	filePath := path.Join(strings.Trim(dir, "/"), name+".md")
	data, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Note{}, syaerr.NotFound{ID: name}
		}
		return Note{}, err
	}
	note, err := ParseBytes(data)
	if err != nil {
		return Note{}, err
	}
	note.File = filePath
	return note, nil
}

func List(fsys fs.FS, dir string) ([]Note, error) {
	dir = strings.Trim(dir, "/")
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var notes []Note
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".md" {
			continue
		}
		filePath := path.Join(dir, entry.Name())
		data, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			return nil, err
		}
		note, err := ParseBytes(data)
		if err != nil {
			return nil, err
		}
		note.File = filePath
		notes = append(notes, note)
	}
	sort.Slice(notes, func(i, j int) bool {
		if notes[i].Name != notes[j].Name {
			return notes[i].Name < notes[j].Name
		}
		return notes[i].File < notes[j].File
	})
	return notes, nil
}

func Save(dir string, note Note) error {
	note.Name = Slug(note.Name)
	if note.Name == "" {
		return syaerr.Usage{Message: "memory key is required"}
	}
	sort.Strings(note.Tasks)
	note.Tasks = compact(note.Tasks)
	data, err := Serialize(note)
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(dir, note.Name+".md"), data, 0o644)
}

func Delete(dir, name string) error {
	name = Slug(name)
	if name == "" {
		return syaerr.Usage{Message: "memory key is required"}
	}
	err := os.Remove(filepath.Join(dir, name+".md"))
	if err != nil {
		if os.IsNotExist(err) {
			return syaerr.NotFound{ID: name}
		}
		return err
	}
	return nil
}

func ParseBytes(data []byte) (Note, error) {
	yml, body, err := splitFrontmatter(data)
	if err != nil {
		return Note{}, syaerr.SchemaInvalid{Message: err.Error()}
	}
	var fm frontmatter
	if err := yaml.UnmarshalWithOptions(yml, &fm, yaml.DisallowUnknownField()); err != nil {
		return Note{}, syaerr.SchemaInvalid{Message: err.Error()}
	}
	fm.Name = Slug(fm.Name)
	if fm.Name == "" {
		return Note{}, syaerr.SchemaInvalid{Message: "memory note missing name"}
	}
	sort.Strings(fm.Tasks)
	return Note{
		Name:        fm.Name,
		Description: fm.Description,
		Tasks:       compact(fm.Tasks),
		Body:        string(body),
	}, nil
}

func Serialize(note Note) ([]byte, error) {
	note.Name = Slug(note.Name)
	if note.Name == "" {
		return nil, syaerr.Usage{Message: "memory key is required"}
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	if err := appendYAMLField(&buf, "name", note.Name); err != nil {
		return nil, err
	}
	if err := appendYAMLField(&buf, "description", note.Description); err != nil {
		return nil, err
	}
	if len(note.Tasks) > 0 {
		if err := appendYAMLField(&buf, "tasks", note.Tasks); err != nil {
			return nil, err
		}
	}
	buf.WriteString("---\n")
	body := strings.TrimRight(note.Body, "\n")
	if body != "" {
		buf.WriteString(body)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func splitFrontmatter(data []byte) ([]byte, []byte, error) {
	if len(data) < 4 || !bytes.HasPrefix(data, []byte("---")) {
		return nil, nil, fmt.Errorf("missing opening frontmatter marker")
	}
	firstEnd := markerLineEnd(data, 0)
	if firstEnd >= len(data) {
		return nil, nil, fmt.Errorf("unterminated opening frontmatter marker")
	}
	if !isMarkerLine(data[:firstEnd]) {
		return nil, nil, fmt.Errorf("opening frontmatter marker must be on its own line")
	}
	pos := firstEnd + 1
	for pos < len(data) {
		lineEnd := markerLineEnd(data, pos)
		if isMarkerLine(data[pos:lineEnd]) {
			bodyStart := lineEnd
			if bodyStart < len(data) && data[bodyStart] == '\n' {
				bodyStart++
			}
			return data[firstEnd+1 : pos], data[bodyStart:], nil
		}
		if lineEnd == len(data) {
			break
		}
		pos = lineEnd + 1
	}
	return nil, nil, fmt.Errorf("missing closing frontmatter marker")
}

func markerLineEnd(data []byte, start int) int {
	if idx := bytes.IndexByte(data[start:], '\n'); idx >= 0 {
		return start + idx
	}
	return len(data)
}

func isMarkerLine(line []byte) bool {
	line = bytes.TrimSuffix(line, []byte("\r"))
	return bytes.Equal(line, []byte("---"))
}

func appendYAMLField(buf *bytes.Buffer, key string, value any) error {
	data, err := yaml.Marshal(map[string]any{key: value})
	if err != nil {
		return err
	}
	buf.Write(data)
	return nil
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

func compact(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:0]
	var last string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || value == last {
			continue
		}
		out = append(out, value)
		last = value
	}
	return out
}

var cyrillicSlug = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}
