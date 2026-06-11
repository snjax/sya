package task

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/snjax/sya/internal/syaerr"
)

func TestParseSerializeGolden(t *testing.T) {
	t.Parallel()

	data := readTestdata(t, "sya_written.md")
	parsed, err := ParseBytes(data)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	got, err := Serialize(parsed)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("Serialize(Parse(sya-written)) mismatch\n--- got ---\n%s\n--- want ---\n%s", got, data)
	}
}

func TestParseTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        []byte
		wantSections []string
		wantErr      any
	}{
		{
			name: "empty body",
			input: []byte("---\n" +
				"id: aaaaaa\n" +
				"type: task\n" +
				"status: todo\n" +
				"---\n"),
		},
		{
			name: "no sections",
			input: []byte("---\n" +
				"id: aaaaab\n" +
				"type: task\n" +
				"status: todo\n" +
				"---\n" +
				"plain body\nwithout h2\n"),
			wantSections: []string{""},
		},
		{
			name:         "unicode",
			input:        readTestdata(t, "foreign_valid.md"),
			wantSections: []string{"", "Description"},
		},
		{
			name:         "crlf",
			input:        readTestdata(t, "crlf.md"),
			wantSections: []string{"", "Description"},
		},
		{
			name: "frontmatter edge values",
			input: []byte("---\n" +
				"id: deadbe\n" +
				"type: task\n" +
				"title: Edge\n" +
				"status: todo\n" +
				"priority: low\n" +
				"labels: []\n" +
				"relations: {}\n" +
				"fields:\n" +
				"  empty_string: \"\"\n" +
				"  number: 7\n" +
				"  list: [a, b]\n" +
				"links:\n" +
				"  - path: internal/task/task.go\n" +
				"created: 2026-06-11T12:00:00Z\n" +
				"schema_version: 1\n" +
				"archived: true\n" +
				"---\n"),
		},
		{
			name: "unknown key",
			input: []byte("---\n" +
				"id: aaaaac\n" +
				"type: task\n" +
				"status: todo\n" +
				"updated: 2026-06-11T12:00:00Z\n" +
				"---\n"),
			wantErr: syaerr.SchemaInvalid{},
		},
		{
			name: "missing id",
			input: []byte("---\n" +
				"type: task\n" +
				"status: todo\n" +
				"---\n"),
			wantErr: syaerr.SchemaInvalid{},
		},
		{
			name: "missing type",
			input: []byte("---\n" +
				"id: aaaaad\n" +
				"status: todo\n" +
				"---\n"),
			wantErr: syaerr.SchemaInvalid{},
		},
		{
			name: "missing status",
			input: []byte("---\n" +
				"id: aaaaae\n" +
				"type: task\n" +
				"---\n"),
			wantErr: syaerr.SchemaInvalid{},
		},
		{
			name: "conflict markers",
			input: []byte("---\n" +
				"id: aaaaaf\n" +
				"type: task\n" +
				"status: todo\n" +
				"---\n" +
				"<<<<<<< HEAD\n"),
			wantErr: syaerr.ErrConflictMarkers{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseBytes(tt.input)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("ParseBytes() error = nil, want %T", tt.wantErr)
				}
				assertErrorAs(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("ParseBytes() error = %v", err)
			}
			if got.Body.Raw == nil {
				t.Fatalf("Body.Raw is nil")
			}
			var names []string
			for _, section := range got.Body.Sections {
				names = append(names, section.Name)
			}
			if !reflect.DeepEqual(names, tt.wantSections) {
				t.Fatalf("section names = %#v, want %#v", names, tt.wantSections)
			}
		})
	}
}

func TestParsePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "task.md")
	if err := os.WriteFile(path, readTestdata(t, "sya_written.md"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.File != path {
		t.Fatalf("File = %q, want %q", got.File, path)
	}
}

func TestForeignValidBodyPreservedAndFrontmatterSemantic(t *testing.T) {
	t.Parallel()

	data := readTestdata(t, "foreign_valid.md")
	parsed, err := ParseBytes(data)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	serialized, err := Serialize(parsed)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	reparsed, err := ParseBytes(serialized)
	if err != nil {
		t.Fatalf("ParseBytes(serialized) error = %v", err)
	}
	if !bytes.Equal(reparsed.Body.Raw, parsed.Body.Raw) {
		t.Fatalf("body changed\n--- got ---\n%s\n--- want ---\n%s", reparsed.Body.Raw, parsed.Body.Raw)
	}
	assertFrontmatterEqual(t, reparsed, parsed)
}

func TestAppendLog(t *testing.T) {
	t.Parallel()

	parsed, err := ParseBytes([]byte("---\nid: aaaaaa\ntype: task\nstatus: todo\n---\n## Description\nbody\n"))
	if err != nil {
		t.Fatal(err)
	}
	AppendLog(parsed, "codex", "implemented")

	if got := parsed.Body.Sections[len(parsed.Body.Sections)-1].Name; got != "Log" {
		t.Fatalf("last section = %q, want Log", got)
	}
	re := regexp.MustCompile(`(?m)^- \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z @codex: implemented$`)
	if !re.Match(parsed.Body.Raw) {
		t.Fatalf("log entry missing from body:\n%s", parsed.Body.Raw)
	}
}

func TestEditSection(t *testing.T) {
	t.Parallel()

	parsed, err := ParseBytes([]byte("---\nid: aaaaaa\ntype: task\nstatus: todo\n---\n## Description\nold\n## Notes\nkeep\n"))
	if err != nil {
		t.Fatal(err)
	}
	EditSection(parsed, "Description", []byte("new\n"))
	EditSection(parsed, "Acceptance", []byte("- [ ] done\n"))

	if !bytes.Contains(parsed.Body.Raw, []byte("## Description\nnew\n## Notes\nkeep\n")) {
		t.Fatalf("Description was not replaced correctly:\n%s", parsed.Body.Raw)
	}
	if !bytes.HasSuffix(parsed.Body.Raw, []byte("## Acceptance\n- [ ] done\n")) {
		t.Fatalf("Acceptance was not appended last:\n%s", parsed.Body.Raw)
	}
}

func TestRoundTripGeneratedTasks(t *testing.T) {
	t.Parallel()

	rng := newDeterministicRand(1)
	for i := 0; i < 1000; i++ {
		task := generatedTask(rng, i)
		data, err := Serialize(task)
		if err != nil {
			t.Fatalf("Serialize(%d) error = %v", i, err)
		}
		parsed, err := ParseBytes(data)
		if err != nil {
			t.Fatalf("ParseBytes(%d) error = %v\n%s", i, err, data)
		}
		again, err := Serialize(parsed)
		if err != nil {
			t.Fatalf("Serialize(parsed %d) error = %v", i, err)
		}
		if !bytes.Equal(again, data) {
			t.Fatalf("round trip %d mismatch\n--- got ---\n%s\n--- want ---\n%s", i, again, data)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, name := range []string{"sya_written.md", "foreign_valid.md", "crlf.md"} {
		f.Add(readTestdata(f, name))
	}
	f.Add([]byte("---\nid: aaaaaa\ntype: task\nstatus: todo\n---\n## Description\nbody\n"))
	f.Add([]byte("not frontmatter"))
	f.Add(fuzzParseCrasher60969d9411a56c08())
	f.Add([]byte("---\nid: aaaaaa\ntype: task\nstatus: todo\nlabels: [[[[[[[[[[[[[[[[[]]]]]]]]]]]]]]]]]\n---\n"))
	f.Add([]byte("---\nid: aaaaaa\ntype: task\nstatus: todo\nlabels: scalar-not-array\n---\n"))
	f.Add([]byte("---\nid: &id aaaaaa\ntype: *id\nstatus: todo\nrelations: &rels {depends_on: [bbbbbb]}\nfields: {copy: *rels}\n---\n"))
	f.Add([]byte("---\nid: aaaaaa\ntype: task\nstatus: \"--- inside value\"\nlabels: !0000000000000000000 000\n---\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		parsed, err := ParseBytes(data)
		if err != nil {
			return
		}
		serialized, err := Serialize(parsed)
		if err != nil {
			t.Fatalf("Serialize() error = %v", err)
		}
		reparsed, err := ParseBytes(serialized)
		if err != nil {
			t.Fatalf("ParseBytes(Serialize()) error = %v", err)
		}
		again, err := Serialize(reparsed)
		if err != nil {
			t.Fatalf("Serialize(reparsed) error = %v", err)
		}
		if !bytes.Equal(again, serialized) {
			t.Fatalf("second serialize changed output")
		}
	})
}

func fuzzParseCrasher60969d9411a56c08() []byte {
	return []byte("---\n" +
		"type: 00\n" +
		"id: 000080\n" +
		"status: \"\xae\xbd\xba\xbe \xb8 \xbf\x80\x8f\xbe\xba  00\x87\xb5\xb9\"\n" +
		"labels: !0000000000000000000 000\n" +
		"---")
}

func readTestdata(tb testing.TB, name string) []byte {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		tb.Fatal(err)
	}
	return data
}

func assertErrorAs(t *testing.T, err error, target any) {
	t.Helper()
	switch target.(type) {
	case syaerr.SchemaInvalid:
		var got syaerr.SchemaInvalid
		if !errors.As(err, &got) {
			t.Fatalf("error = %T %v, want SchemaInvalid", err, err)
		}
	case syaerr.ErrConflictMarkers:
		var got syaerr.ErrConflictMarkers
		if !errors.As(err, &got) {
			t.Fatalf("error = %T %v, want ErrConflictMarkers", err, err)
		}
	default:
		t.Fatalf("unsupported target %T", target)
	}
}

func assertFrontmatterEqual(t *testing.T, got, want *Task) {
	t.Helper()
	gotBody, wantBody := got.Body, want.Body
	gotFile, wantFile := got.File, want.File
	got.Body, want.Body = Body{}, Body{}
	got.File, want.File = "", ""
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("frontmatter mismatch\n got: %#v\nwant: %#v", got, want)
	}
	got.Body, want.Body = gotBody, wantBody
	got.File, want.File = gotFile, wantFile
}

type deterministicRand struct {
	state uint64
}

func newDeterministicRand(seed uint64) *deterministicRand {
	return &deterministicRand{state: seed}
}

func (r *deterministicRand) next() uint64 {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return r.state
}

func (r *deterministicRand) intn(n int) int {
	return int(r.next() % uint64(n))
}

func generatedTask(r *deterministicRand, i int) *Task {
	id := strings.ToLower(fmtHex(i + 1))
	created := time.Date(2026, 6, 11, 12, i%60, 0, 0, time.UTC)
	sections := []Section{
		{Name: "Description", Raw: []byte("## Description\n" + randomText(r) + "\n")},
		{Name: "Log", Raw: []byte("## Log\n- 2026-06-11T12:00:00Z @test: generated\n")},
	}
	body := NewBody(nil, sections)
	rebuildBodyRaw(&Task{Body: body})
	task := &Task{
		ID:            id,
		Type:          []string{"task", "bug", "feature"}[r.intn(3)],
		Title:         "Generated " + id,
		Status:        []string{"todo", "open", "impl"}[r.intn(3)],
		Priority:      []string{"", "low", "high"}[r.intn(3)],
		Labels:        []string{"generated", fmtHex(r.intn(16))},
		Relations:     map[string][]string{"depends_on": {fmtHex(r.intn(256) + 1)}},
		Fields:        map[string]any{"flag": r.intn(2) == 0, "count": r.intn(10)},
		Created:       created,
		SchemaVersion: 1,
		Body:          body,
	}
	rebuildBodyRaw(task)
	return task
}

func randomText(r *deterministicRand) string {
	words := []string{"alpha", "beta", "unicode-ю", "line", "value"}
	count := 1 + r.intn(8)
	var parts []string
	for i := 0; i < count; i++ {
		parts = append(parts, words[r.intn(len(words))])
	}
	return strings.Join(parts, " ")
}

func fmtHex(n int) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 6)
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = digits[n&0xf]
		n >>= 4
	}
	return string(out)
}
