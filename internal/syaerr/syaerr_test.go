package syaerr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestErrorEnvelopeGoldens(t *testing.T) {
	t.Parallel()

	tests := map[string]error{
		"usage": Usage{Message: "missing task id"},
		"not_found": NotFound{
			ID: "a3f8c1",
		},
		"ambiguous": Ambiguous{
			Prefix: "a3",
			Candidates: []Candidate{
				{ID: "a3f8c1", Title: "Streaming responses", Type: "feature", Status: "impl", File: ".sya/tasks/a3f8c1-streaming-responses.md"},
				{ID: "a3b771", Title: "Retry transport", Type: "bug", Status: "todo", File: ".sya/tasks/a3b771-retry-transport.md"},
			},
		},
		"transition_not_allowed": TransitionNotAllowed{
			Task:     "a3f8c1",
			TaskType: "feature",
			From:     "draft",
			To:       "done",
			Allowed: []TransitionOption{
				{To: "spec", Kind: "advance", Description: "Requirements are ready for specification"},
				{To: "scrapped", Kind: "setback", Description: "Task was cancelled with rationale in Log", Terminal: true},
			},
		},
		"transition_blocked": TransitionBlocked{
			Task: "a3f8c1",
			Transition: TransitionRef{
				From:        "spec",
				To:          "impl",
				Kind:        "advance",
				Description: "Specification approved; start implementation",
			},
			Violations: []Violation{
				{
					Kind:    "field",
					Field:   "spec_approved",
					Message: "Spec is not approved (fields.spec_approved)",
					Hint:    "After spec review: sya update a3f8c1 --field spec_approved=true",
				},
				{
					Kind:     "blocking_relation",
					Relation: "depends_on",
					Message:  "Dependencies are not closed",
					Offending: []Candidate{
						{ID: "b771d2", Title: "Transport spike", Type: "task", Status: "impl", File: ".sya/tasks/b771d2-transport-spike.md"},
					},
				},
			},
			Alternatives: []TransitionOption{
				{To: "scrapped", Kind: "setback", Description: "Task was cancelled with rationale in Log"},
			},
		},
		"schema_invalid": SchemaInvalid{
			Message: "schema validation failed",
			Violations: []Violation{
				{Kind: "schema", File: ".sya/schema.yml", Message: "types.feature.terminal is required", Hint: "Add at least one terminal status"},
			},
		},
		"conflict_markers": ErrConflictMarkers{Path: ".sya/tasks/a3f8c1-streaming-responses.md"},
	}

	for name, err := range tests {
		name, err := name, err
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gotBytes, marshalErr := MarshalEnvelope(Failure(err))
			if marshalErr != nil {
				t.Fatalf("marshal envelope: %v", marshalErr)
			}
			wantBytes, readErr := os.ReadFile(filepath.Join("testdata", name+".golden.json"))
			if readErr != nil {
				t.Fatalf("read golden: %v", readErr)
			}
			got := strings.TrimSpace(string(gotBytes))
			want := strings.TrimSpace(string(wantBytes))
			if got != want {
				t.Fatalf("envelope mismatch\nwant: %s\n got: %s", want, got)
			}
		})
	}
}
