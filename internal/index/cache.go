package index

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/snjax/sya/internal/task"
)

const (
	indexCacheVersion = 1
	racyWindow        = 2 * time.Second
)

type LoadOptions struct {
	DisableCache bool
	ProjectRoot  string
	CacheDir     string
	Now          func() time.Time
}

type taskFileMeta struct {
	path    string
	size    int64
	mtimeNS int64
}

type indexCache struct {
	Version     int                   `json:"version"`
	SchemaHash  string                `json:"schema_hash"`
	WrittenAtNS int64                 `json:"written_at_ns"`
	Entries     map[string]cacheEntry `json:"entries"`
}

type cacheEntry struct {
	Path    string     `json:"path"`
	Size    int64      `json:"size"`
	MtimeNS int64      `json:"mtime_ns"`
	Task    cachedTask `json:"task"`
}

type cachedTask struct {
	Task     task.Task       `json:"task"`
	BodyRaw  []byte          `json:"body_raw,omitempty"`
	Sections []cachedSection `json:"sections,omitempty"`
}

type cachedSection struct {
	Name string `json:"name"`
	Raw  []byte `json:"raw,omitempty"`
}

func cacheEnabled(fsys fs.FS, dir string, opts LoadOptions) (cacheContext, bool) {
	if opts.DisableCache || os.Getenv("SYA_NO_CACHE") != "" {
		return cacheContext{}, false
	}
	root := opts.ProjectRoot
	if root == "" {
		root = localRootFromFS(fsys)
	}
	if root == "" {
		return cacheContext{}, false
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return cacheContext{}, false
	}
	schemaBytes, err := fs.ReadFile(fsys, pathJoin(strings.Trim(dir, "/"), "schema.yml"))
	if err != nil {
		return cacheContext{}, false
	}
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir = defaultCacheDir()
	}
	if cacheDir == "" {
		return cacheContext{}, false
	}
	sum := sha256.Sum256([]byte(root))
	return cacheContext{
		projectRoot: root,
		cachePath:   filepath.Join(cacheDir, "sya", hex.EncodeToString(sum[:])+".json"),
		schemaHash:  sha256Hex(schemaBytes),
		now:         opts.Now,
	}, true
}

type cacheContext struct {
	projectRoot string
	cachePath   string
	schemaHash  string
	now         func() time.Time
}

func (c cacheContext) currentTime() time.Time {
	if c.now != nil {
		return c.now().UTC()
	}
	return time.Now().UTC()
}

func readCache(ctx cacheContext) (*indexCache, bool) {
	data, err := os.ReadFile(ctx.cachePath)
	if err != nil {
		return nil, false
	}
	var cache indexCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false
	}
	if cache.Version != indexCacheVersion || cache.SchemaHash != ctx.schemaHash || cache.Entries == nil {
		return nil, false
	}
	return &cache, true
}

func reusableCachedTask(cache *indexCache, meta taskFileMeta) (*task.Task, bool) {
	if cache == nil {
		return nil, false
	}
	entry, ok := cache.Entries[meta.path]
	if !ok || entry.Size != meta.size || entry.MtimeNS != meta.mtimeNS {
		return nil, false
	}
	if withinRacyWindow(meta.mtimeNS, cache.WrittenAtNS) {
		return nil, false
	}
	t := taskFromCache(entry.Task)
	t.File = meta.path
	return t, true
}

func writeCache(ctx cacheContext, metas []taskFileMeta, results map[string]taskFileResult) {
	entries := make(map[string]cacheEntry, len(results))
	for _, meta := range metas {
		result, ok := results[meta.path]
		if !ok || result.task == nil {
			continue
		}
		entries[meta.path] = cacheEntry{
			Path:    meta.path,
			Size:    meta.size,
			MtimeNS: meta.mtimeNS,
			Task:    taskToCache(result.task),
		}
	}
	cache := indexCache{
		Version:     indexCacheVersion,
		SchemaHash:  ctx.schemaHash,
		WrittenAtNS: ctx.currentTime().UnixNano(),
		Entries:     entries,
	}
	writeCacheFile(ctx.cachePath, cache)
}

func writeCacheFile(cachePath string, cache indexCache) {
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return
	}
	lock, err := os.OpenFile(cachePath+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(cachePath), "."+filepath.Base(cachePath)+".tmp-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(tmpName, cachePath)
}

func taskToCache(t *task.Task) cachedTask {
	copyTask := *t
	copyTask.File = ""
	copyTask.Body = task.Body{}
	sections := make([]cachedSection, 0, len(t.Body.Sections))
	for _, section := range t.Body.Sections {
		sections = append(sections, cachedSection{
			Name: section.Name,
			Raw:  append([]byte(nil), section.Raw...),
		})
	}
	return cachedTask{
		Task:     copyTask,
		BodyRaw:  append([]byte(nil), t.Body.Raw...),
		Sections: sections,
	}
}

func taskFromCache(cached cachedTask) *task.Task {
	t := cached.Task
	t.Body.Raw = append([]byte(nil), cached.BodyRaw...)
	t.Body.Sections = make([]task.Section, 0, len(cached.Sections))
	for _, section := range cached.Sections {
		t.Body.Sections = append(t.Body.Sections, task.Section{
			Name: section.Name,
			Raw:  append([]byte(nil), section.Raw...),
		})
	}
	return &t
}

func defaultCacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".cache")
}

func localRootFromFS(fsys fs.FS) string {
	value := reflect.ValueOf(fsys)
	if value.IsValid() && value.Kind() == reflect.String {
		return value.String()
	}
	return ""
}

func pathJoin(left, right string) string {
	if left == "" || left == "." {
		return right
	}
	return left + "/" + right
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func withinRacyWindow(mtimeNS, writtenAtNS int64) bool {
	if writtenAtNS == 0 {
		return true
	}
	delta := time.Duration(mtimeNS - writtenAtNS)
	if delta < 0 {
		delta = -delta
	}
	return delta <= racyWindow
}

func sortMetas(metas []taskFileMeta) {
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].path < metas[j].path
	})
}

func statMtimeNS(info fs.FileInfo) int64 {
	if info == nil {
		return 0
	}
	return info.ModTime().UnixNano()
}
