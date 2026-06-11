package schema

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestParseDefaultsAndStrictYAML(t *testing.T) {
	t.Parallel()

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()
		schema := parseSchema(t, `
schema_version: 1
defaults: {type: task}
relations:
  depends_on:
    reverse: blocks
types:
  task:
    pipeline: [todo, done, scrapped]
    terminal: [done, scrapped]
    transitions:
      todo -> done: {}
      "* -> scrapped": {}
`)
		if got := schema.Relations["depends_on"].From; !slices.Equal(got, []string{"*"}) {
			t.Fatalf("relation from default = %v, want [*]", got)
		}
		if got := schema.Relations["depends_on"].To; !slices.Equal(got, []string{"*"}) {
			t.Fatalf("relation to default = %v, want [*]", got)
		}
		task := schema.Types["task"]
		if task.Board == nil || !*task.Board {
			t.Fatalf("board default = %v, want true", task.Board)
		}
		if got := task.Transitions["todo -> done"].Kind; got != TransitionAdvance {
			t.Fatalf("explicit kind default = %q, want %q", got, TransitionAdvance)
		}
		if got := task.Transitions["* -> scrapped"].Kind; got != TransitionSetback {
			t.Fatalf("wildcard kind default = %q, want %q", got, TransitionSetback)
		}
	})

	t.Run("unknown top level key", func(t *testing.T) {
		t.Parallel()
		if _, err := Parse([]byte(baseSchemaYAML() + "\nunknown: true\n")); err == nil {
			t.Fatal("Parse succeeded with unknown top-level key")
		}
	})

	t.Run("unknown guard key", func(t *testing.T) {
		t.Parallel()
		input := strings.Replace(baseSchemaYAML(), "message: ok", "message: ok\n            unexpected: true", 1)
		if _, err := Parse([]byte(input)); err == nil {
			t.Fatal("Parse succeeded with unknown guard key")
		}
	})

	t.Run("wildcard only on left", func(t *testing.T) {
		t.Parallel()
		input := strings.Replace(baseSchemaYAML(), "todo -> in_progress", "todo -> *", 1)
		if _, err := Parse([]byte(input)); err == nil {
			t.Fatal("Parse succeeded with wildcard target")
		}
	})
}

func TestValidateRules(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		input       string
		wantValid   bool
		wantKind    string
		wantWarning string
	}{
		{name: "valid base", input: baseSchemaYAML(), wantValid: true},
		{
			name:      "defaults type exists",
			input:     strings.Replace(baseSchemaYAML(), "type: task", "type: ghost", 1),
			wantKind:  "unknown_default_type",
			wantValid: false,
		},
		{
			name:      "transition target in pipeline",
			input:     strings.Replace(baseSchemaYAML(), "\"* -> scrapped\"", "\"* -> ghost\"", 1),
			wantKind:  "transition_target_unknown",
			wantValid: false,
		},
		{
			name: "unreachable status",
			input: `
schema_version: 1
defaults: {type: task}
types:
  task:
    pipeline: [todo, unreachable, done]
    terminal: [done]
    transitions:
      todo -> done: {}
      unreachable -> done: {}
`,
			wantKind:  "status_unreachable",
			wantValid: false,
		},
		{
			name: "non-terminal outgoing transition",
			input: `
schema_version: 1
defaults: {type: task}
types:
  task:
    pipeline: [todo, stuck, done]
    terminal: [done]
    transitions:
      todo -> stuck: {}
`,
			wantKind:  "nonterminal_dead_end",
			wantValid: false,
		},
		{
			name:      "guard relation declared",
			input:     strings.Replace(baseSchemaYAML(), "relation: depends_on", "relation: missing", 1),
			wantKind:  "guard_relation_unknown",
			wantValid: false,
		},
		{
			name:      "guard field declared",
			input:     strings.Replace(baseSchemaYAML(), "field: ready", "field: missing", 1),
			wantKind:  "guard_field_unknown",
			wantValid: false,
		},
		{
			name:      "guard section declared",
			input:     strings.Replace(baseSchemaYAML(), "section: Design", "section: Missing", 1),
			wantKind:  "guard_section_unknown",
			wantValid: false,
		},
		{
			name:        "guard literal status warning",
			input:       strings.Replace(baseSchemaYAML(), "in: [done]", "in: [ghost]", 1),
			wantValid:   true,
			wantWarning: "guard_status_unknown",
		},
		{
			name:      "terminal required",
			input:     strings.Replace(baseSchemaYAML(), "    terminal: [done, scrapped]\n", "", 1),
			wantKind:  "terminal_required",
			wantValid: false,
		},
		{
			name:      "terminal subset",
			input:     strings.Replace(baseSchemaYAML(), "terminal: [done, scrapped]", "terminal: [done, ghost]", 1),
			wantKind:  "terminal_subset",
			wantValid: false,
		},
		{
			name:      "working subset",
			input:     strings.Replace(baseSchemaYAML(), "working: [in_progress]", "working: [ghost]", 1),
			wantKind:  "working_subset",
			wantValid: false,
		},
		{
			name:      "parked subset",
			input:     strings.Replace(baseSchemaYAML(), "parked: [accepted]", "parked: [ghost]", 1),
			wantKind:  "parked_subset",
			wantValid: false,
		},
		{
			name:      "statuses subset",
			input:     strings.Replace(baseSchemaYAML(), "done: Done", "done: Done\n      ghost: Ghost", 1),
			wantKind:  "status_description_unknown",
			wantValid: false,
		},
		{
			name:      "child type ref exists",
			input:     strings.Replace(baseSchemaYAML(), "children: [task]", "children: [ghost]", 1),
			wantKind:  "child_type_ref",
			wantValid: false,
		},
		{
			name:      "children require container",
			input:     strings.Replace(baseSchemaYAML(), "container: true\n    children: [task]", "children: [task]", 1),
			wantKind:  "children_on_non_container",
			wantValid: false,
		},
		{
			name:      "relation endpoint type exists",
			input:     strings.Replace(baseSchemaYAML(), "from: [task]", "from: [ghost]", 1),
			wantKind:  "relation_type_ref",
			wantValid: false,
		},
		{
			name:      "self transitions forbidden",
			input:     strings.Replace(baseSchemaYAML(), "todo -> in_progress", "todo -> todo", 1),
			wantKind:  "transition_self",
			wantValid: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			schema := parseSchema(t, tc.input)
			result := schema.Validate()
			if got := result.Valid(); got != tc.wantValid {
				t.Fatalf("Valid() = %v, want %v; violations=%v warnings=%v", got, tc.wantValid, result.Violations, result.Warnings)
			}
			if tc.wantKind != "" && !hasDiagnosticKind(result.Violations, tc.wantKind) {
				t.Fatalf("missing violation kind %q in %#v", tc.wantKind, result.Violations)
			}
			if tc.wantWarning != "" && !hasDiagnosticKind(result.Warnings, tc.wantWarning) {
				t.Fatalf("missing warning kind %q in %#v", tc.wantWarning, result.Warnings)
			}
		})
	}
}

func TestExpandTransitionsWildcardTruthTable(t *testing.T) {
	t.Parallel()

	typeDef := TypeDef{
		Pipeline: []string{"open", "review", "done", "scrapped"},
		Terminal: []string{"done", "scrapped"},
		Transitions: map[string]Transition{
			"open -> scrapped": {From: "open", To: "scrapped", Kind: TransitionAdvance},
			"* -> scrapped":    {From: "*", To: "scrapped", Kind: TransitionSetback},
			"review -> done":   {From: "review", To: "done", Kind: TransitionAdvance},
		},
	}
	transitions, err := ExpandTransitions(typeDef)
	if err != nil {
		t.Fatalf("ExpandTransitions returned error: %v", err)
	}
	got := transitionPairs(transitions)
	want := []string{"open -> scrapped", "review -> done", "review -> scrapped"}
	if !slices.Equal(got, want) {
		t.Fatalf("expanded transitions = %v, want %v", got, want)
	}
	for _, transition := range transitions {
		if transition.From == "review" && transition.To == "scrapped" && transition.Kind != TransitionSetback {
			t.Fatalf("wildcard expansion kind = %q, want setback", transition.Kind)
		}
		if transition.From == "open" && transition.To == "scrapped" && transition.Kind != TransitionAdvance {
			t.Fatalf("explicit transition did not take precedence: %q", transition.Kind)
		}
	}
}

func TestReferenceSchemaValidates(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/reference.yml")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse reference schema: %v", err)
	}
	if result := schema.Validate(); !result.Valid() || len(result.Warnings) > 0 {
		t.Fatalf("reference schema diagnostics: violations=%v warnings=%v", result.Violations, result.Warnings)
	}
}

func TestWildcardExpansionProperty(t *testing.T) {
	t.Parallel()

	pipelines := [][]string{
		{"a", "b", "done"},
		{"todo", "doing", "verify", "done", "scrapped"},
		{"only", "done"},
	}
	for _, pipeline := range pipelines {
		pipeline := pipeline
		t.Run(strings.Join(pipeline, "_"), func(t *testing.T) {
			t.Parallel()
			terminal := []string{pipeline[len(pipeline)-1]}
			for _, target := range pipeline {
				typeDef := TypeDef{
					Pipeline: pipeline,
					Terminal: terminal,
					Transitions: map[string]Transition{
						"* -> " + target: {From: "*", To: target},
					},
				}
				transitions, err := ExpandTransitions(typeDef)
				if err != nil {
					t.Fatalf("ExpandTransitions(%v -> %s): %v", pipeline, target, err)
				}
				terminalSet := makeStringSet(terminal)
				for _, transition := range transitions {
					if transition.From == transition.To {
						t.Fatalf("wildcard yielded self-transition %#v", transition)
					}
					if terminalSet[transition.From] {
						t.Fatalf("wildcard yielded terminal source %#v", transition)
					}
				}
			}
		})
	}
}

func FuzzParseSchema(f *testing.F) {
	for _, seed := range []string{
		baseSchemaYAML(),
		"schema_version: 1\n",
		"types:\n  task:\n    transitions:\n      \"* -> done\": {}\n",
		"not: [valid",
		"schema_version: 1\ntypes: [[[[[[[[[[[[[[[[[]]]]]]]]]]]]]]]]]\n",
		"schema_version: 1\ntypes:\n  task:\n    pipeline: todo\n",
		"schema_version: &v 1\ndefaults: {type: task}\ntypes: &types {task: {pipeline: [todo], terminal: [todo]}}\nrelations: {relates: {from: *types}}\n",
		"schema_version: 1\ndescription: \"--- inside value\"\ntypes: !0000000000000000000 000\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		schema, err := Parse([]byte(input))
		if err == nil {
			_ = schema.Validate()
		}
	})
}

func parseSchema(t *testing.T, input string) *Schema {
	t.Helper()
	schema, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	return schema
}

func hasDiagnosticKind(diagnostics []Diagnostic, kind string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Kind == kind {
			return true
		}
	}
	return false
}

func transitionPairs(transitions []Transition) []string {
	pairs := make([]string, 0, len(transitions))
	for _, transition := range transitions {
		pairs = append(pairs, transitionKey(transition.From, transition.To))
	}
	return pairs
}

func baseSchemaYAML() string {
	return `
schema_version: 1
defaults:
  type: task
relations:
  depends_on:
    reverse: blocks
    blocking: true
    from: [task]
    to: [task]
types:
  epic:
    container: true
    children: [task]
    pipeline: [open, done]
    terminal: [done]
    transitions:
      open -> done: {}
  decision:
    pipeline: [proposed, accepted, superseded]
    terminal: [superseded]
    parked: [accepted]
    transitions:
      proposed -> accepted: {}
      accepted -> superseded: {}
  task:
    pipeline: [todo, in_progress, done, scrapped]
    statuses:
      todo: Todo
      in_progress: In progress
      done: Done
      scrapped: Scrapped
    terminal: [done, scrapped]
    working: [in_progress]
    sections: [Description, Design]
    fields:
      ready: {type: bool, default: false}
    transitions:
      todo -> in_progress:
        guards:
          - kind: relation_status
            relation: depends_on
            in: [done]
            message: ok
          - kind: field
            field: ready
            equals: true
            message: ok
          - kind: section_nonempty
            section: Design
            message: ok
      in_progress -> done: {}
      in_progress -> todo: {}
      "* -> scrapped": {}
`
}
