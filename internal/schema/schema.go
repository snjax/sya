package schema

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/snjax/sya/internal/syaerr"
)

type Schema struct {
	SchemaVersion int                    `json:"schema_version" yaml:"schema_version"`
	Description   string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Defaults      Defaults               `json:"defaults,omitempty" yaml:"defaults,omitempty"`
	Relations     map[string]RelationDef `json:"relations,omitempty" yaml:"relations,omitempty"`
	Types         map[string]TypeDef     `json:"types,omitempty" yaml:"types,omitempty"`
}

type Defaults struct {
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
}

type TypeDef struct {
	Description string                `json:"description,omitempty" yaml:"description,omitempty"`
	Pipeline    []string              `json:"pipeline,omitempty" yaml:"pipeline,omitempty"`
	Statuses    map[string]string     `json:"statuses,omitempty" yaml:"statuses,omitempty"`
	Terminal    []string              `json:"terminal,omitempty" yaml:"terminal,omitempty"`
	Working     []string              `json:"working,omitempty" yaml:"working,omitempty"`
	Parked      []string              `json:"parked,omitempty" yaml:"parked,omitempty"`
	Board       *bool                 `json:"board,omitempty" yaml:"board,omitempty"`
	Container   bool                  `json:"container,omitempty" yaml:"container,omitempty"`
	Children    []string              `json:"children,omitempty" yaml:"children,omitempty"`
	Sections    []string              `json:"sections,omitempty" yaml:"sections,omitempty"`
	Fields      map[string]FieldDef   `json:"fields,omitempty" yaml:"fields,omitempty"`
	Transitions map[string]Transition `json:"transitions,omitempty" yaml:"transitions,omitempty"`
}

type FieldDef struct {
	Type        string   `json:"type,omitempty" yaml:"type,omitempty"`
	Values      []string `json:"values,omitempty" yaml:"values,omitempty"`
	Default     any      `json:"default,omitempty" yaml:"default,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
}

type RelationDef struct {
	Reverse     string   `json:"reverse,omitempty" yaml:"reverse,omitempty"`
	Symmetric   bool     `json:"symmetric,omitempty" yaml:"symmetric,omitempty"`
	Graph       string   `json:"graph,omitempty" yaml:"graph,omitempty"`
	Blocking    bool     `json:"blocking,omitempty" yaml:"blocking,omitempty"`
	From        []string `json:"from,omitempty" yaml:"from,omitempty"`
	To          []string `json:"to,omitempty" yaml:"to,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
}

type TransitionKind string

const (
	TransitionAdvance TransitionKind = "advance"
	TransitionSetback TransitionKind = "setback"
)

type Transition struct {
	From           string         `json:"from,omitempty" yaml:"-"`
	To             string         `json:"to,omitempty" yaml:"-"`
	Kind           TransitionKind `json:"kind,omitempty" yaml:"kind,omitempty"`
	Description    string         `json:"description,omitempty" yaml:"description,omitempty"`
	Guards         []Guard        `json:"guards,omitempty" yaml:"guards,omitempty"`
	IgnoreBlocking []string       `json:"ignore_blocking,omitempty" yaml:"ignore_blocking,omitempty"`
}

type GuardKind string

const (
	GuardRelationStatus  GuardKind = "relation_status"
	GuardRelationExists  GuardKind = "relation_exists"
	GuardField           GuardKind = "field"
	GuardChildrenStatus  GuardKind = "children_status"
	GuardParentStatus    GuardKind = "parent_status"
	GuardSectionNonempty GuardKind = "section_nonempty"
	GuardCheck           GuardKind = "check"
	GuardAttest          GuardKind = "attest"
)

type Guard struct {
	Kind    GuardKind      `json:"kind" yaml:"kind"`
	Params  map[string]any `json:"params,omitempty" yaml:"-"`
	Message string         `json:"message,omitempty" yaml:"message,omitempty"`
	Hint    string         `json:"hint,omitempty" yaml:"hint,omitempty"`
}

func New() *Schema {
	return &Schema{
		Relations: make(map[string]RelationDef),
		Types:     make(map[string]TypeDef),
	}
}

func Parse(data []byte) (*Schema, error) {
	var schema Schema
	if err := unmarshalSchema(data, &schema); err != nil {
		return nil, err
	}
	if schema.Relations == nil {
		schema.Relations = make(map[string]RelationDef)
	}
	if schema.Types == nil {
		schema.Types = make(map[string]TypeDef)
	}
	if err := schema.normalize(); err != nil {
		return nil, err
	}
	return &schema, nil
}

func unmarshalSchema(data []byte, out *Schema) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = syaerr.SchemaInvalid{Message: fmt.Sprintf("malformed schema: %v", recovered)}
		}
	}()
	if err := yaml.UnmarshalWithOptions(data, out, yaml.Strict()); err != nil {
		return syaerr.SchemaInvalid{Message: err.Error()}
	}
	return nil
}

func (s *Schema) normalize() error {
	for name, relation := range s.Relations {
		if len(relation.From) == 0 {
			relation.From = []string{"*"}
		}
		if len(relation.To) == 0 {
			relation.To = []string{"*"}
		}
		s.Relations[name] = relation
	}
	for typeName, typeDef := range s.Types {
		if typeDef.Board == nil {
			board := true
			typeDef.Board = &board
		}
		if typeDef.Statuses == nil {
			typeDef.Statuses = make(map[string]string)
		}
		if typeDef.Fields == nil {
			typeDef.Fields = make(map[string]FieldDef)
		}
		normalized, err := normalizeTransitions(typeName, typeDef.Transitions)
		if err != nil {
			return err
		}
		typeDef.Transitions = normalized
		s.Types[typeName] = typeDef
	}
	return nil
}

func normalizeTransitions(typeName string, transitions map[string]Transition) (map[string]Transition, error) {
	if transitions == nil {
		return nil, nil
	}
	normalized := make(map[string]Transition, len(transitions))
	for key, transition := range transitions {
		from, to, err := parseTransitionKey(key)
		if err != nil {
			return nil, fmt.Errorf("types.%s.transitions.%q: %w", typeName, key, err)
		}
		transition.From = from
		transition.To = to
		if transition.Kind == "" {
			if from == "*" {
				transition.Kind = TransitionSetback
			} else {
				transition.Kind = TransitionAdvance
			}
		}
		normalized[transitionKey(from, to)] = transition
	}
	return normalized, nil
}

func parseTransitionKey(key string) (string, string, error) {
	parts := strings.Split(key, "->")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("transition key must have form %q", "from -> to")
	}
	from := strings.TrimSpace(parts[0])
	to := strings.TrimSpace(parts[1])
	if from == "" || to == "" {
		return "", "", fmt.Errorf("transition endpoints must be non-empty")
	}
	if to == "*" {
		return "", "", fmt.Errorf("wildcard is allowed only on the left side")
	}
	return from, to, nil
}

func transitionKey(from, to string) string {
	return from + " -> " + to
}

type guardYAML struct {
	Kind     GuardKind `yaml:"kind"`
	Relation string    `yaml:"relation,omitempty"`
	In       []string  `yaml:"in,omitempty"`
	Field    string    `yaml:"field,omitempty"`
	Equals   any       `yaml:"equals,omitempty"`
	Section  string    `yaml:"section,omitempty"`
	Run      string    `yaml:"run,omitempty"`
	Timeout  int       `yaml:"timeout,omitempty"`
	ID       string    `yaml:"id,omitempty"`
	Question string    `yaml:"question,omitempty"`
	Message  string    `yaml:"message,omitempty"`
	Hint     string    `yaml:"hint,omitempty"`
}

func (g *Guard) UnmarshalYAML(unmarshal func(any) error) error {
	var raw guardYAML
	if err := unmarshal(&raw); err != nil {
		return err
	}
	g.Kind = raw.Kind
	g.Message = raw.Message
	g.Hint = raw.Hint
	params := make(map[string]any)
	if raw.Relation != "" {
		params["relation"] = raw.Relation
	}
	if raw.In != nil {
		params["in"] = raw.In
	}
	if raw.Field != "" {
		params["field"] = raw.Field
	}
	if raw.Equals != nil {
		params["equals"] = raw.Equals
	}
	if raw.Section != "" {
		params["section"] = raw.Section
	}
	if raw.Run != "" {
		params["run"] = raw.Run
	}
	if raw.Timeout != 0 {
		params["timeout"] = raw.Timeout
	}
	if raw.ID != "" {
		params["id"] = raw.ID
	}
	if raw.Question != "" {
		params["question"] = raw.Question
	}
	if len(params) > 0 {
		g.Params = params
	}
	return nil
}
