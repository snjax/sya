package task

import "time"

// Task mirrors the task-file frontmatter from PRD section 5.1.
type Task struct {
	ID            string              `json:"id" yaml:"id"`
	Type          string              `json:"type" yaml:"type"`
	Title         string              `json:"title" yaml:"title"`
	Status        string              `json:"status" yaml:"status"`
	Priority      string              `json:"priority" yaml:"priority"`
	Parent        string              `json:"parent,omitempty" yaml:"parent,omitempty"`
	Assignee      string              `json:"assignee,omitempty" yaml:"assignee,omitempty"`
	Labels        []string            `json:"labels,omitempty" yaml:"labels,omitempty"`
	Relations     map[string][]string `json:"relations,omitempty" yaml:"relations,omitempty"`
	Fields        map[string]any      `json:"fields,omitempty" yaml:"fields,omitempty"`
	Links         []Link              `json:"links,omitempty" yaml:"links,omitempty"`
	Created       time.Time           `json:"created" yaml:"created"`
	SchemaVersion int                 `json:"schema_version" yaml:"schema_version"`
	Archived      bool                `json:"archived,omitempty" yaml:"archived,omitempty"`
	Body          Body                `json:"body,omitempty" yaml:"-"`
	File          string              `json:"file,omitempty" yaml:"-"`
}

// Link is an annotation-only external reference attached to a task.
type Link struct {
	URL   string `json:"url,omitempty" yaml:"url,omitempty"`
	Path  string `json:"path,omitempty" yaml:"path,omitempty"`
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
}

// Body preserves the original markdown bytes and the body section order.
type Body struct {
	Raw      []byte    `json:"-" yaml:"-"`
	Sections []Section `json:"sections,omitempty" yaml:"-"`
}

// Section is one markdown body section in source order.
type Section struct {
	Name string `json:"name" yaml:"-"`
	Raw  []byte `json:"-" yaml:"-"`
}

// Summary is the compact task shape used by prefix resolution and errors.
type Summary struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Type   string `json:"type"`
	Status string `json:"status"`
	File   string `json:"file,omitempty"`
}

func New(id, taskType, title string) *Task {
	return &Task{
		ID:        id,
		Type:      taskType,
		Title:     title,
		Relations: make(map[string][]string),
		Fields:    make(map[string]any),
	}
}

func NewBody(raw []byte, sections []Section) Body {
	return Body{Raw: raw, Sections: sections}
}

func (t Task) Summary() Summary {
	return Summary{
		ID:     t.ID,
		Title:  t.Title,
		Type:   t.Type,
		Status: t.Status,
		File:   t.File,
	}
}
