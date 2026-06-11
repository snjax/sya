package index

import (
	"io/fs"
	"strings"

	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
)

type FileWriter interface {
	WriteFile(name string, data []byte, perm fs.FileMode) error
}

type Storage interface {
	fs.FS
	FileWriter
}

type Index interface {
	Load(dir string) error
	Get(id string) (*task.Task, error)
	ResolvePrefix(prefix string) (*task.Task, []task.Summary, error)
	All() []*task.Task
	ReverseEdges() ReverseEdges
}

type ReverseEdges map[string]map[string][]string

type MemoryIndex struct {
	fs      Storage
	tasks   map[string]*task.Task
	reverse ReverseEdges
}

func New(storage Storage) *MemoryIndex {
	return &MemoryIndex{
		fs:      storage,
		tasks:   make(map[string]*task.Task),
		reverse: make(ReverseEdges),
	}
}

func (i *MemoryIndex) Load(dir string) error {
	_ = dir
	return nil
}

func (i *MemoryIndex) Get(id string) (*task.Task, error) {
	t, ok := i.tasks[id]
	if !ok {
		return nil, syaerr.NotFound{ID: id}
	}
	return t, nil
}

func (i *MemoryIndex) ResolvePrefix(prefix string) (*task.Task, []task.Summary, error) {
	var matches []task.Summary
	var found *task.Task
	for id, candidate := range i.tasks {
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		found = candidate
		matches = append(matches, candidate.Summary())
	}
	if len(matches) == 0 {
		return nil, nil, syaerr.NotFound{ID: prefix}
	}
	if len(matches) > 1 {
		return nil, matches, syaerr.Ambiguous{Prefix: prefix, Candidates: matches}
	}
	return found, nil, nil
}

func (i *MemoryIndex) All() []*task.Task {
	tasks := make([]*task.Task, 0, len(i.tasks))
	for _, t := range i.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

func (i *MemoryIndex) ReverseEdges() ReverseEdges {
	return i.reverse
}
