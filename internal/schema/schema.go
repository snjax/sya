package schema

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
