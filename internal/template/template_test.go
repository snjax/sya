package template

import (
	"strings"
	"testing"
)

func TestApplySubstitutesParams(t *testing.T) {
	t.Parallel()

	tmpl, err := Parse([]byte(`
name: feature
params:
  - name: name
    required: true
tasks:
  - key: spec
    title: "Spec {{name}}"
    assignee: "{{owner}}"
    sections:
      Description: "Design {{ name }}"
    fields:
      note: "{{name}}"
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tmpl.Params = append(tmpl.Params, Param{Name: "owner", Default: "codex"})

	applied, err := tmpl.Apply(map[string]string{"name": "Streaming"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := applied.Tasks[0]
	if got.Title != "Spec Streaming" || got.Assignee != "codex" || got.Sections["Description"] != "Design Streaming" || got.Fields["note"] != "Streaming" {
		t.Fatalf("unexpected substitution: %#v", got)
	}
}

func TestTemplateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		args map[string]string
		want string
	}{
		{
			name: "strict unknown field",
			yaml: `
name: bad
extra: nope
tasks:
  - key: a
    title: A
`,
			want: "unknown field \"extra\"",
		},
		{
			name: "missing required param",
			yaml: `
name: bad
params:
  - name: topic
    required: true
tasks:
  - key: a
    title: "{{topic}}"
`,
			args: map[string]string{},
			want: "missing required template param",
		},
		{
			name: "unknown placeholder",
			yaml: `
name: bad
tasks:
  - key: a
    title: "{{missing}}"
`,
			args: map[string]string{},
			want: "unknown template placeholder",
		},
		{
			name: "cycle",
			yaml: `
name: bad
tasks:
  - key: a
    title: A
    relations:
      depends_on: [b]
  - key: b
    title: B
    relations:
      depends_on: [a]
`,
			args: map[string]string{},
			want: "template contains cycle",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpl, err := Parse([]byte(tt.yaml))
			if tt.name == "strict unknown field" || tt.name == "cycle" {
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("Parse() error = %v, want %q", err, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			_, err = tmpl.Apply(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Apply() error = %v, want %q", err, tt.want)
			}
		})
	}
}
