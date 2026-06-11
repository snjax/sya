package index

import (
	"bytes"

	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/task"
)

var _ schema.Resolver = (*Resolver)(nil)
var _ schema.TaskView = (*TaskView)(nil)

type Resolver struct {
	index *Index
}

type TaskView struct {
	index *Index
	task  *task.Task
}

func (i *Index) Resolver() *Resolver {
	return &Resolver{index: i}
}

func (r *Resolver) Get(id string) (schema.TaskView, bool) {
	if r == nil || r.index == nil {
		return nil, false
	}
	t, ok := r.index.tasks[id]
	if !ok {
		return nil, false
	}
	return &TaskView{index: r.index, task: t}, true
}

func (v *TaskView) Status() string {
	return v.task.Status
}

func (v *TaskView) Type() string {
	return v.task.Type
}

func (v *TaskView) Relations(name string) []string {
	return append([]string(nil), v.index.forward[v.task.ID][name]...)
}

func (v *TaskView) Children() []string {
	return v.index.Children(v.task.ID)
}

func (v *TaskView) Parent() (string, bool) {
	if v.task.Parent == "" {
		return "", false
	}
	return v.task.Parent, true
}

func (v *TaskView) Field(name string) (any, bool) {
	value, ok := v.task.Fields[name]
	return value, ok
}

func (v *TaskView) SectionNonEmpty(name string) bool {
	for _, section := range v.task.Body.Sections {
		if section.Name != name {
			continue
		}
		body := bytes.TrimSpace(bytes.TrimPrefix(section.Raw, []byte("## "+name)))
		return len(body) > 0
	}
	return false
}

func (v *TaskView) Archived() bool {
	return v.task.Archived
}
