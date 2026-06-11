package index

import (
	"io/fs"
	"runtime"
	"sort"
	"sync"

	"github.com/snjax/sya/internal/task"
)

type taskFileResult struct {
	path       string
	task       *task.Task
	quarantine *QuarantinedFile
}

func loadTaskFilesParallel(fsys fs.FS, paths []string) []taskFileResult {
	if len(paths) == 0 {
		return nil
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > len(paths) {
		workers = len(paths)
	}
	jobs := make(chan string)
	results := make(chan taskFileResult, len(paths))

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for filePath := range jobs {
				results <- loadTaskFile(fsys, filePath)
			}
		}()
	}
	for _, filePath := range paths {
		jobs <- filePath
	}
	close(jobs)
	wg.Wait()
	close(results)

	out := make([]taskFileResult, 0, len(paths))
	for result := range results {
		out = append(out, result)
	}
	sort.Slice(out, func(a, b int) bool {
		return out[a].path < out[b].path
	})
	return out
}
