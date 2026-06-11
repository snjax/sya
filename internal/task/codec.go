package task

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/snjax/sya/internal/syaerr"
)

var frontmatterOrder = []string{
	"id",
	"type",
	"title",
	"status",
	"priority",
	"parent",
	"assignee",
	"labels",
	"relations",
	"fields",
	"links",
	"created",
	"schema_version",
	"archived",
}

type frontmatter struct {
	ID            string              `yaml:"id"`
	Type          string              `yaml:"type"`
	Title         string              `yaml:"title"`
	Status        string              `yaml:"status"`
	Priority      string              `yaml:"priority"`
	Parent        string              `yaml:"parent"`
	Assignee      string              `yaml:"assignee"`
	Labels        []string            `yaml:"labels"`
	Relations     map[string][]string `yaml:"relations"`
	Fields        map[string]any      `yaml:"fields"`
	Links         []Link              `yaml:"links"`
	Created       time.Time           `yaml:"created"`
	SchemaVersion int                 `yaml:"schema_version"`
	Archived      bool                `yaml:"archived"`
}

func Parse(path string) (*Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parse(path, data)
}

func ParseFile(path string) (*Task, error) {
	return Parse(path)
}

func ParseBytes(data []byte) (*Task, error) {
	return parse("", data)
}

func Serialize(t *Task) ([]byte, error) {
	if t == nil {
		return nil, syaerr.SchemaInvalid{Message: "task is nil"}
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	for _, key := range frontmatterOrder {
		if err := appendFrontmatterField(&buf, key, t); err != nil {
			return nil, err
		}
	}
	buf.WriteString("---\n")
	buf.Write(bodyBytes(t))
	return buf.Bytes(), nil
}

func AppendLog(t *Task, actor, line string) error {
	if err := validateSectionName("Log"); err != nil {
		return err
	}
	entry := escapeSectionContent([]byte(fmt.Sprintf("- %s @%s: %s\n", time.Now().UTC().Format(time.RFC3339), actor, line)))
	idx := sectionIndex(t, "Log")
	if idx < 0 {
		return appendSection(t, "Log", entry)
	}
	section := &t.Body.Sections[idx]
	if len(section.Raw) > 0 && !bytes.HasSuffix(section.Raw, []byte("\n")) {
		section.Raw = append(section.Raw, '\n')
	}
	section.Raw = append(section.Raw, entry...)
	rebuildBodyRaw(t)
	return nil
}

func EditSection(t *Task, name string, content []byte) error {
	if err := validateSectionName(name); err != nil {
		return err
	}
	content = escapeSectionContent(content)
	idx := sectionIndex(t, name)
	if idx < 0 {
		return appendSection(t, name, content)
	}

	section := &t.Body.Sections[idx]
	headerEnd := bytes.IndexByte(section.Raw, '\n')
	if headerEnd < 0 {
		headerEnd = len(section.Raw)
		section.Raw = append(section.Raw, '\n')
	} else {
		headerEnd++
	}
	raw := append([]byte(nil), section.Raw[:headerEnd]...)
	raw = append(raw, content...)
	section.Raw = raw
	rebuildBodyRaw(t)
	return nil
}

func parse(path string, data []byte) (*Task, error) {
	if hasConflictMarkers(data) {
		return nil, syaerr.ErrConflictMarkers{Path: path}
	}

	yml, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, schemaInvalid(path, err.Error())
	}

	var fm frontmatter
	if err := unmarshalFrontmatter(yml, &fm); err != nil {
		return nil, schemaInvalid(path, err.Error())
	}
	if fm.ID == "" {
		return nil, schemaInvalid(path, "missing required field id")
	}
	if fm.Type == "" {
		return nil, schemaInvalid(path, "missing required field type")
	}
	if fm.Status == "" {
		return nil, schemaInvalid(path, "missing required field status")
	}

	t := &Task{
		ID:            fm.ID,
		Type:          fm.Type,
		Title:         fm.Title,
		Status:        fm.Status,
		Priority:      fm.Priority,
		Parent:        fm.Parent,
		Assignee:      fm.Assignee,
		Labels:        fm.Labels,
		Relations:     fm.Relations,
		Fields:        fm.Fields,
		Links:         fm.Links,
		Created:       fm.Created,
		SchemaVersion: fm.SchemaVersion,
		Archived:      fm.Archived,
		Body:          splitBody(body),
		File:          path,
	}
	if t.Relations == nil {
		t.Relations = make(map[string][]string)
	}
	if t.Fields == nil {
		t.Fields = make(map[string]any)
	}
	return t, nil
}

func hasConflictMarkers(data []byte) bool {
	lines := splitLines(data)
	if len(lines) > 0 && hasConflictStart(lines[0]) {
		return true
	}
	for i, line := range lines {
		if !hasConflictStart(line) {
			continue
		}
		separator := false
		for _, later := range lines[i+1:] {
			switch {
			case hasConflictSeparator(later):
				separator = true
			case separator && hasConflictEnd(later):
				return true
			}
		}
	}
	return false
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	for len(data) > 0 {
		line := data
		if idx := bytes.IndexByte(data, '\n'); idx >= 0 {
			line = data[:idx]
			data = data[idx+1:]
		} else {
			data = nil
		}
		lines = append(lines, bytes.TrimSuffix(line, []byte("\r")))
	}
	return lines
}

func hasConflictStart(line []byte) bool {
	return bytes.HasPrefix(line, []byte("<<<<<<<"))
}

func hasConflictSeparator(line []byte) bool {
	return bytes.HasPrefix(line, []byte("======="))
}

func hasConflictEnd(line []byte) bool {
	return bytes.HasPrefix(line, []byte(">>>>>>>"))
}

func hasConflictMarkerPrefix(line []byte) bool {
	return hasConflictStart(line) || hasConflictSeparator(line) || hasConflictEnd(line)
}

func validateSectionName(name string) error {
	switch {
	case name == "":
		return syaerr.Usage{Message: "section name is required"}
	case strings.ContainsAny(name, "\r\n"):
		return syaerr.Usage{Message: "section name must not contain newlines"}
	case strings.HasPrefix(name, "#"):
		return syaerr.Usage{Message: "section name must not start with #"}
	case hasConflictMarkerPrefix([]byte(name)):
		return syaerr.Usage{Message: "section name must not start with a git conflict marker"}
	default:
		return nil
	}
}

// escapeSectionContent protects bytes written through mutation helpers from
// being reinterpreted by the markdown section scanner. Parsed task bodies keep
// their raw bytes for Parse->Serialize identity; only newly edited content is
// escaped, using standard markdown backslash escapes for headings and conflict
// marker-looking lines.
func escapeSectionContent(content []byte) []byte {
	if len(content) == 0 {
		return nil
	}
	var out bytes.Buffer
	for len(content) > 0 {
		line := content
		if idx := bytes.IndexByte(content, '\n'); idx >= 0 {
			line = content[:idx+1]
			content = content[idx+1:]
		} else {
			content = nil
		}
		lineForCheck := bytes.TrimSuffix(line, []byte("\n"))
		lineForCheck = bytes.TrimSuffix(lineForCheck, []byte("\r"))
		if bytes.HasPrefix(lineForCheck, []byte("## ")) || hasConflictMarkerPrefix(lineForCheck) {
			out.WriteByte('\\')
		}
		out.Write(line)
	}
	return out.Bytes()
}

func appendSection(t *Task, name string, content []byte) error {
	if err := validateSectionName(name); err != nil {
		return err
	}
	if len(t.Body.Sections) == 0 && len(t.Body.Raw) > 0 {
		t.Body.Sections = splitBody(t.Body.Raw).Sections
	}
	if len(t.Body.Sections) == 0 && len(t.Body.Raw) > 0 {
		t.Body.Sections = append(t.Body.Sections, Section{Raw: append([]byte(nil), t.Body.Raw...)})
	}
	header := []byte("## " + name + "\n")
	raw := append([]byte(nil), header...)
	raw = append(raw, content...)
	if len(t.Body.Sections) > 0 {
		last := &t.Body.Sections[len(t.Body.Sections)-1]
		if len(last.Raw) > 0 && !bytes.HasSuffix(last.Raw, []byte("\n")) {
			last.Raw = append(last.Raw, '\n')
		}
	}
	t.Body.Sections = append(t.Body.Sections, Section{Name: name, Raw: raw})
	rebuildBodyRaw(t)
	return nil
}

func rebuildBodyRaw(t *Task) {
	var raw []byte
	for _, section := range t.Body.Sections {
		raw = append(raw, section.Raw...)
	}
	t.Body.Raw = raw
}

func bodyBytes(t *Task) []byte {
	if len(t.Body.Sections) == 0 {
		return t.Body.Raw
	}
	var raw []byte
	for _, section := range t.Body.Sections {
		raw = append(raw, section.Raw...)
	}
	return raw
}

func appendFrontmatterField(buf *bytes.Buffer, key string, t *Task) error {
	switch key {
	case "id":
		return appendYAMLField(buf, key, t.ID)
	case "type":
		return appendYAMLField(buf, key, t.Type)
	case "title":
		return appendYAMLField(buf, key, t.Title)
	case "status":
		return appendYAMLField(buf, key, t.Status)
	case "priority":
		return appendYAMLField(buf, key, t.Priority)
	case "parent":
		return appendYAMLField(buf, key, t.Parent)
	case "assignee":
		return appendYAMLField(buf, key, t.Assignee)
	case "labels":
		return appendYAMLField(buf, key, t.Labels)
	case "relations":
		return appendSortedMap(buf, key, t.Relations)
	case "fields":
		return appendSortedMap(buf, key, t.Fields)
	case "links":
		return appendYAMLField(buf, key, t.Links)
	case "created":
		if t.Created.IsZero() {
			return nil
		}
		return appendYAMLField(buf, key, t.Created.UTC().Format(time.RFC3339Nano))
	case "schema_version":
		if t.SchemaVersion == 0 {
			return nil
		}
		return appendYAMLField(buf, key, t.SchemaVersion)
	case "archived":
		if !t.Archived {
			return nil
		}
		return appendYAMLField(buf, key, t.Archived)
	default:
		return nil
	}
}

func appendYAMLField(buf *bytes.Buffer, key string, value any) error {
	if isEmpty(value) {
		return nil
	}
	data, err := yaml.Marshal(map[string]any{key: value})
	if err != nil {
		return err
	}
	buf.Write(data)
	return nil
}

func unmarshalFrontmatter(data []byte, out *frontmatter) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("malformed frontmatter: %v", recovered)
		}
	}()
	return yaml.UnmarshalWithOptions(data, out, yaml.DisallowUnknownField())
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
		if lineEnd < 0 {
			lineEnd = len(data)
		}
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

func splitBody(raw []byte) Body {
	body := Body{Raw: append([]byte{}, raw...)}
	starts := sectionStarts(raw)
	if len(starts) == 0 {
		if len(raw) > 0 {
			body.Sections = []Section{{Raw: append([]byte(nil), raw...)}}
		}
		return body
	}

	if starts[0] > 0 {
		body.Sections = append(body.Sections, Section{Raw: append([]byte(nil), raw[:starts[0]]...)})
	}
	for i, start := range starts {
		end := len(raw)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		sectionRaw := append([]byte(nil), raw[start:end]...)
		body.Sections = append(body.Sections, Section{
			Name: sectionName(sectionRaw),
			Raw:  sectionRaw,
		})
	}
	return body
}

func sectionStarts(raw []byte) []int {
	var starts []int
	if bytes.HasPrefix(raw, []byte("## ")) {
		starts = append(starts, 0)
	}
	for pos := 0; pos < len(raw); pos++ {
		if raw[pos] != '\n' {
			continue
		}
		next := pos + 1
		if next < len(raw) && bytes.HasPrefix(raw[next:], []byte("## ")) {
			starts = append(starts, next)
		}
	}
	return starts
}

func sectionName(raw []byte) string {
	if !bytes.HasPrefix(raw, []byte("## ")) {
		return ""
	}
	line := raw[3:]
	if idx := bytes.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}
	line = bytes.TrimSuffix(line, []byte("\r"))
	return string(line)
}

func sectionIndex(t *Task, name string) int {
	for i, section := range t.Body.Sections {
		if section.Name == name {
			return i
		}
	}
	return -1
}

func appendSortedMap[T any](buf *bytes.Buffer, key string, values map[string]T) error {
	if len(values) == 0 {
		return nil
	}
	buf.WriteString(key)
	buf.WriteString(":\n")
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		data, err := yaml.Marshal(map[string]any{k: values[k]})
		if err != nil {
			return err
		}
		for _, line := range bytes.SplitAfter(data, []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			buf.WriteString("  ")
			buf.Write(line)
		}
	}
	return nil
}

func isEmpty(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

func schemaInvalid(path, message string) syaerr.SchemaInvalid {
	if path == "" {
		return syaerr.SchemaInvalid{Message: message}
	}
	return syaerr.SchemaInvalid{Message: strings.TrimSpace(path + ": " + message)}
}
