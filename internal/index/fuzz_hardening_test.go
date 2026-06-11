package index

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

func FuzzResolve(f *testing.F) {
	for _, seed := range []struct {
		prefix string
		salt   string
	}{
		{"a", "alpha"},
		{"abc", "same-prefix"},
		{"", "empty"},
		{"雪", "unicode"},
	} {
		f.Add(seed.prefix, seed.salt)
	}
	f.Fuzz(func(t *testing.T, prefix, salt string) {
		fsys := randomResolveProject(salt)
		idx, err := Load(fsys, ".sya", testSchema())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		_, _ = idx.Resolve(prefix)
		_, _, _ = idx.ResolvePrefix(prefix)
	})
}

func randomResolveProject(salt string) fstest.MapFS {
	if salt == "" {
		salt = "seed"
	}
	count := 1 + len([]rune(salt))%8
	files := make(fstest.MapFS, count)
	for i := 0; i < count; i++ {
		ch := 'a' + rune(i%26)
		if len(salt) > 0 {
			ch = 'a' + rune(salt[i%len(salt)]%26)
		}
		id := fmt.Sprintf("%c%05x", ch, i)
		files[fmt.Sprintf(".sya/tasks/%s.md", id)] = &fstest.MapFile{Data: []byte(taskDoc(taskFields{
			ID:       id,
			Type:     "task",
			Title:    "Resolve " + strings.Map(func(r rune) rune { return r }, salt),
			Status:   "todo",
			Priority: "normal",
			Created:  "2026-01-01T00:00:00Z",
		}))}
	}
	return files
}
