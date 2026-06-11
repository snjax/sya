package task

import (
	"reflect"
	"testing"
	"time"
)

func FuzzEditSection(f *testing.F) {
	for _, seed := range []struct {
		name    string
		content string
	}{
		{"Description", "plain text\n"},
		{"Design", "## injected\nbody\n"},
		{"Unicode-雪", "emoji \U0001f9ea and nul \x00\n"},
		{"Line\nBreak", "content"},
	} {
		f.Add(seed.name, seed.content)
	}
	f.Fuzz(func(t *testing.T, name, content string) {
		task := generatedTaskForFuzz()
		EditSection(task, name, []byte(content))
		data, err := Serialize(task)
		if err != nil {
			t.Fatalf("Serialize after EditSection(%q): %v", name, err)
		}
		if _, err := ParseBytes(data); err != nil {
			t.Fatalf("ParseBytes after EditSection(%q, %q): %v\n%s", name, content, err, data)
		}
	})
}

func FuzzAppendLog(f *testing.F) {
	for _, seed := range []struct {
		actor string
		line  string
	}{
		{"codex", "created"},
		{"agent\n## Injected", "line with ## heading"},
		{"雪", "\x00\U0001f9ea\nmulti\nline"},
	} {
		f.Add(seed.actor, seed.line)
	}
	f.Fuzz(func(t *testing.T, actor, line string) {
		task := generatedTaskForFuzz()
		AppendLog(task, actor, line)
		data, err := Serialize(task)
		if err != nil {
			t.Fatalf("Serialize after AppendLog: %v", err)
		}
		if _, err := ParseBytes(data); err != nil {
			t.Fatalf("ParseBytes after AppendLog(%q, %q): %v\n%s", actor, line, err, data)
		}
	})
}

func TestSerializeParseGeneratedTasksProperty(t *testing.T) {
	t.Parallel()

	for seed := 0; seed < 2000; seed++ {
		seed := seed
		t.Run("task", func(t *testing.T) {
			t.Parallel()
			task := hardeningGeneratedTask(seed)
			data, err := Serialize(task)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			parsed, err := ParseBytes(data)
			if err != nil {
				t.Fatalf("ParseBytes: %v\n%s", err, data)
			}
			normalizeTaskForCompare(task)
			normalizeTaskForCompare(parsed)
			if !reflect.DeepEqual(parsed, task) {
				t.Fatalf("round trip mismatch\nwant=%#v\n got=%#v", task, parsed)
			}
		})
	}
}

func generatedTaskForFuzz() *Task {
	return hardeningGeneratedTask(0)
}

func hardeningGeneratedTask(seed int) *Task {
	id := fmtHardeningID(seed)
	statuses := []string{"todo", "in_progress", "done", "scrapped"}
	priorities := []string{"low", "normal", "high", "critical"}
	task := &Task{
		ID:            id,
		Type:          "task",
		Title:         "Title 雪 \U0001f9ea " + string(rune(0x20+(seed%80))),
		Status:        statuses[seed%len(statuses)],
		Priority:      priorities[seed%len(priorities)],
		Assignee:      "agent-" + string(rune('a'+seed%26)),
		Labels:        []string{"label", "unicode-雪"},
		Relations:     map[string][]string{"depends_on": {"dep001"}},
		Fields:        map[string]any{"ready": seed%2 == 0, "estimate": seed % 13, "note": "value\n雪"},
		Created:       time.Date(2026, time.January, 1, 0, 0, seed%60, 0, time.UTC),
		SchemaVersion: 1,
		Archived:      seed%17 == 0,
		Body: Body{
			Sections: []Section{
				{Name: "Description", Raw: []byte("## Description\nBody 雪\n")},
				{Name: "Design", Raw: []byte("## Design\nUnicode 🧪 content\n")},
			},
		},
	}
	rebuildGeneratedBody(task)
	return task
}

func rebuildGeneratedBody(task *Task) {
	task.Body.Raw = nil
	for _, section := range task.Body.Sections {
		task.Body.Raw = append(task.Body.Raw, section.Raw...)
	}
}

func normalizeTaskForCompare(task *Task) {
	task.File = ""
	if task.Relations == nil {
		task.Relations = map[string][]string{}
	}
	if task.Fields == nil {
		task.Fields = map[string]any{}
	}
	for key, value := range task.Fields {
		task.Fields[key] = normalizeScalarForCompare(value)
	}
}

func fmtHardeningID(seed int) string {
	const digits = "0123456789abcdef"
	value := seed + 1
	out := []byte("h00000")
	for i := len(out) - 1; i > 0; i-- {
		out[i] = digits[value&0xf]
		value >>= 4
	}
	return string(out)
}

func normalizeScalarForCompare(value any) any {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		return int64(typed)
	default:
		return value
	}
}
