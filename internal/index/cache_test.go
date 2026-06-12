package index

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoadCacheHit(t *testing.T) {
	skipIfCacheDisabled(t)
	root, cacheDir := cacheFixture(t, "OldTitle", oldCacheTime())
	idx := loadCached(t, root, cacheDir, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if idx.All()[0].Title != "OldTitle" {
		t.Fatalf("initial title=%q", idx.All()[0].Title)
	}

	path := filepath.Join(root, ".sya", "tasks", "a.md")
	mtime := oldCacheTime()
	contents := sameSizeTaskDoc("NewTitle")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("rewrite task: %v", err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	idx = loadCached(t, root, cacheDir, time.Date(2026, 1, 2, 0, 0, 1, 0, time.UTC))
	if idx.All()[0].Title != "OldTitle" {
		t.Fatalf("cache miss title=%q, want cached OldTitle", idx.All()[0].Title)
	}
}

func TestLoadCacheMissOnMetadataChange(t *testing.T) {
	skipIfCacheDisabled(t)
	root, cacheDir := cacheFixture(t, "OldTitle", oldCacheTime())
	_ = loadCached(t, root, cacheDir, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	path := filepath.Join(root, ".sya", "tasks", "a.md")
	newMTime := oldCacheTime().Add(10 * time.Second)
	if err := os.WriteFile(path, []byte(sameSizeTaskDoc("NewTitle")), 0o644); err != nil {
		t.Fatalf("rewrite task: %v", err)
	}
	if err := os.Chtimes(path, newMTime, newMTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	idx := loadCached(t, root, cacheDir, time.Date(2026, 1, 2, 0, 0, 1, 0, time.UTC))
	if idx.All()[0].Title != "NewTitle" {
		t.Fatalf("title=%q, want parsed NewTitle", idx.All()[0].Title)
	}
}

func TestLoadCacheInvalidatesOnSchemaHashChange(t *testing.T) {
	skipIfCacheDisabled(t)
	root, cacheDir := cacheFixture(t, "OldTitle", oldCacheTime())
	_ = loadCached(t, root, cacheDir, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	path := filepath.Join(root, ".sya", "tasks", "a.md")
	mtime := oldCacheTime()
	if err := os.WriteFile(path, []byte(sameSizeTaskDoc("NewTitle")), 0o644); err != nil {
		t.Fatalf("rewrite task: %v", err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sya", "schema.yml"), []byte("schema: changed\n"), 0o644); err != nil {
		t.Fatalf("rewrite schema: %v", err)
	}
	idx := loadCached(t, root, cacheDir, time.Date(2026, 1, 2, 0, 0, 1, 0, time.UTC))
	if idx.All()[0].Title != "NewTitle" {
		t.Fatalf("title=%q, want schema-invalidated NewTitle", idx.All()[0].Title)
	}
}

func TestLoadCacheRacyWindowReparses(t *testing.T) {
	skipIfCacheDisabled(t)
	racy := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	root, cacheDir := cacheFixture(t, "OldTitle", racy)
	_ = loadCached(t, root, cacheDir, racy)

	path := filepath.Join(root, ".sya", "tasks", "a.md")
	if err := os.WriteFile(path, []byte(sameSizeTaskDoc("NewTitle")), 0o644); err != nil {
		t.Fatalf("rewrite task: %v", err)
	}
	if err := os.Chtimes(path, racy, racy); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	idx := loadCached(t, root, cacheDir, racy.Add(time.Second))
	if idx.All()[0].Title != "NewTitle" {
		t.Fatalf("title=%q, want racy reparse NewTitle", idx.All()[0].Title)
	}
}

func TestLoadCacheIgnoresCorruption(t *testing.T) {
	skipIfCacheDisabled(t)
	root, cacheDir := cacheFixture(t, "OldTitle", oldCacheTime())
	cachePath := cachePathForTest(t, root, cacheDir)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt cache: %v", err)
	}
	idx := loadCached(t, root, cacheDir, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if idx.All()[0].Title != "OldTitle" {
		t.Fatalf("title=%q", idx.All()[0].Title)
	}
}

func TestLoadCacheIgnoresOldVersion(t *testing.T) {
	skipIfCacheDisabled(t)
	root, cacheDir := cacheFixture(t, "OldTitle", oldCacheTime())
	cachePath := cachePathForTest(t, root, cacheDir)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte(`{"version":0,"schema_hash":"ignored","entries":{}}`), 0o644); err != nil {
		t.Fatalf("write old cache: %v", err)
	}
	idx := loadCached(t, root, cacheDir, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if idx.All()[0].Title != "OldTitle" {
		t.Fatalf("title=%q", idx.All()[0].Title)
	}
}

func TestLoadCacheConcurrentWriters(t *testing.T) {
	skipIfCacheDisabled(t)
	root, cacheDir := cacheFixture(t, "OldTitle", oldCacheTime())
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			idx := loadCached(t, root, cacheDir, time.Date(2026, 1, 2, 0, 0, i%30, 0, time.UTC))
			if got := len(idx.All()); got != 1 {
				t.Errorf("loaded %d tasks, want 1", got)
			}
		}(i)
	}
	wg.Wait()
	idx := loadCached(t, root, cacheDir, time.Date(2026, 1, 2, 0, 1, 0, 0, time.UTC))
	if idx.All()[0].ID != "a00001" {
		t.Fatalf("bad index after concurrent writers: %#v", idx.All())
	}
}

func TestLoadCacheDisabledByEnv(t *testing.T) {
	for _, value := range []string{"1", "0", "false"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SYA_NO_CACHE", value)
			root, cacheDir := cacheFixture(t, "OldTitle", oldCacheTime())
			_ = loadCached(t, root, cacheDir, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
			entries, err := os.ReadDir(cacheDir)
			if err != nil {
				t.Fatalf("read cache dir: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("cache dir has entries while SYA_NO_CACHE=%q: %#v", value, entries)
			}
		})
	}
}

func cacheFixture(t *testing.T, title string, mtime time.Time) (string, string) {
	t.Helper()
	root := t.TempDir()
	cacheDir := t.TempDir()
	writeIndexSchema(t, root, "schema: one\n")
	writeTaskFile(t, root, "a.md", sameSizeTaskDoc(title))
	taskPath := filepath.Join(root, ".sya", "tasks", "a.md")
	if err := os.Chtimes(taskPath, mtime, mtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	return root, cacheDir
}

func loadCached(t *testing.T, root, cacheDir string, now time.Time) *Index {
	t.Helper()
	idx, err := LoadWithOptions(os.DirFS(root), ".sya", testSchema(), LoadOptions{
		CacheDir: cacheDir,
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	return idx
}

func writeIndexSchema(t testing.TB, root, contents string) {
	t.Helper()
	path := filepath.Join(root, ".sya", "schema.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir schema: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
}

func cachePathForTest(t *testing.T, root, cacheDir string) string {
	t.Helper()
	ctx, ok := cacheEnabled(os.DirFS(root), ".sya", LoadOptions{CacheDir: cacheDir})
	if !ok {
		t.Fatal("cache disabled")
	}
	return ctx.cachePath
}

func oldCacheTime() time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

func sameSizeTaskDoc(title string) string {
	if len(title) != len("OldTitle") {
		panic("test title must match OldTitle length")
	}
	doc := taskDoc(taskFields{
		ID:       "a00001",
		Type:     "task",
		Title:    title,
		Status:   "todo",
		Priority: "normal",
		Created:  "2026-01-01T09:00:00Z",
	})
	if !strings.Contains(doc, fmt.Sprintf("title: %q", title)) {
		panic("bad fixture")
	}
	return doc
}

func skipIfCacheDisabled(t *testing.T) {
	t.Helper()
	if os.Getenv("SYA_NO_CACHE") != "" {
		t.Skip("cache behavior test requires cache enabled")
	}
}
