package memory

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestMemoryCRUD(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	note := Note{
		Name:        "Стриминг Process",
		Description: "How streaming deploys work",
		Tasks:       []string{"b00001", "a00001", "a00001"},
		Body:        "Use SSE for streaming responses.\n",
	}
	if err := Save(dir, note); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "striming-process.md")); err != nil {
		t.Fatalf("stat transliterated note file: %v", err)
	}

	got, err := Load(os.DirFS(dir), ".", "Стриминг Process")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != "striming-process" || got.Description != note.Description || strings.TrimSpace(got.Body) != strings.TrimSpace(note.Body) {
		t.Fatalf("loaded note mismatch: %#v", got)
	}
	if !reflect.DeepEqual(got.Tasks, []string{"a00001", "b00001"}) {
		t.Fatalf("tasks = %#v", got.Tasks)
	}

	if err := Delete(dir, got.Name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := Load(os.DirFS(dir), ".", got.Name); err == nil {
		t.Fatalf("Load after Delete succeeded")
	}
}

func TestListSorted(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"memory/b.md":  {Data: []byte("---\nname: b\ndescription: Bee\n---\nB\n")},
		"memory/a.md":  {Data: []byte("---\nname: a\ndescription: Aye\n---\nA\n")},
		"memory/x.txt": {Data: []byte("ignored")},
	}
	got, err := List(fsys, "memory")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var names []string
	for _, note := range got {
		names = append(names, note.Name)
	}
	if !reflect.DeepEqual(names, []string{"a", "b"}) {
		t.Fatalf("names = %#v", names)
	}
}

func TestSlug(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"Стриминг":      "striming",
		"API Стриминг":  "api-striming",
		"emoji only 🚀✨": "emoji-only",
		"🚀✨":            "",
	}
	for input, want := range tests {
		input, want := input, want
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if got := Slug(input); got != want {
				t.Fatalf("Slug(%q) = %q, want %q", input, got, want)
			}
		})
	}
}
