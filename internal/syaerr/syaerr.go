package syaerr

import (
	"encoding/json"
	"fmt"

	"github.com/snjax/sya/internal/task"
)

const (
	ExitOK                 = 0
	ExitUsage              = 1
	ExitLookup             = 2
	ExitTransitionRejected = 3
	ExitSchemaInvalid      = 4
)

type Coded interface {
	error
	Type() string
	ExitCode() int
}

type Usage struct {
	Message string `json:"message"`
}

func (e Usage) Error() string { return e.Message }
func (e Usage) Type() string  { return "usage" }
func (e Usage) ExitCode() int { return ExitUsage }

type NotFound struct {
	ID string `json:"id"`
}

func (e NotFound) Error() string { return fmt.Sprintf("not found: %s", e.ID) }
func (e NotFound) Type() string  { return "not_found" }
func (e NotFound) ExitCode() int { return ExitLookup }

type Ambiguous struct {
	Prefix     string         `json:"prefix"`
	Candidates []task.Summary `json:"candidates"`
}

func (e Ambiguous) Error() string { return fmt.Sprintf("ambiguous prefix: %s", e.Prefix) }
func (e Ambiguous) Type() string  { return "ambiguous" }
func (e Ambiguous) ExitCode() int { return ExitLookup }

type TransitionNotAllowed struct {
	Task    string             `json:"task"`
	From    string             `json:"from"`
	To      string             `json:"to"`
	Allowed []TransitionOption `json:"allowed"`
}

func (e TransitionNotAllowed) Error() string { return "transition not allowed" }
func (e TransitionNotAllowed) Type() string  { return "transition_not_allowed" }
func (e TransitionNotAllowed) ExitCode() int { return ExitTransitionRejected }

type TransitionBlocked struct {
	Task         string             `json:"task"`
	Transition   TransitionRef      `json:"transition"`
	Violations   []Violation        `json:"violations"`
	Alternatives []TransitionOption `json:"alternatives"`
}

func (e TransitionBlocked) Error() string { return "transition blocked" }
func (e TransitionBlocked) Type() string  { return "transition_blocked" }
func (e TransitionBlocked) ExitCode() int { return ExitTransitionRejected }

type SchemaInvalid struct {
	Message    string      `json:"message"`
	Violations []Violation `json:"violations,omitempty"`
}

func (e SchemaInvalid) Error() string { return e.Message }
func (e SchemaInvalid) Type() string  { return "schema_invalid" }
func (e SchemaInvalid) ExitCode() int { return ExitSchemaInvalid }

type TransitionRef struct {
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Description string `json:"description,omitempty"`
}

type TransitionOption struct {
	To          string `json:"to"`
	Kind        string `json:"kind,omitempty"`
	Description string `json:"description,omitempty"`
}

type Violation struct {
	Kind      string         `json:"kind"`
	Field     string         `json:"field,omitempty"`
	Relation  string         `json:"relation,omitempty"`
	Section   string         `json:"section,omitempty"`
	File      string         `json:"file,omitempty"`
	Message   string         `json:"message"`
	Hint      string         `json:"hint,omitempty"`
	Offending []task.Summary `json:"offending,omitempty"`
}

type Envelope struct {
	OK    bool `json:"ok"`
	Data  any  `json:"data,omitempty"`
	Error any  `json:"error,omitempty"`
}

func Success(data any) Envelope {
	return Envelope{OK: true, Data: data}
}

func Failure(err error) Envelope {
	return Envelope{OK: false, Error: Payload(err)}
}

func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	if coded, ok := err.(interface{ ExitCode() int }); ok {
		return coded.ExitCode()
	}
	return ExitUsage
}

func Payload(err error) ErrorPayload {
	payload := ErrorPayload{
		Type:    "usage",
		Message: err.Error(),
	}
	if coded, ok := err.(Coded); ok {
		payload.Type = coded.Type()
	}
	switch e := err.(type) {
	case NotFound:
		payload.ID = e.ID
	case Ambiguous:
		payload.Prefix = e.Prefix
		payload.Candidates = e.Candidates
	case TransitionNotAllowed:
		payload.Task = e.Task
		payload.From = e.From
		payload.To = e.To
		payload.Allowed = e.Allowed
	case TransitionBlocked:
		payload.Task = e.Task
		payload.Transition = &e.Transition
		payload.Violations = e.Violations
		payload.Alternatives = e.Alternatives
	case SchemaInvalid:
		payload.Violations = e.Violations
	case Usage:
	}
	return payload
}

type ErrorPayload struct {
	Type         string             `json:"type"`
	Message      string             `json:"message,omitempty"`
	ID           string             `json:"id,omitempty"`
	Prefix       string             `json:"prefix,omitempty"`
	Candidates   []task.Summary     `json:"candidates,omitempty"`
	Task         string             `json:"task,omitempty"`
	From         string             `json:"from,omitempty"`
	To           string             `json:"to,omitempty"`
	Transition   *TransitionRef     `json:"transition,omitempty"`
	Violations   []Violation        `json:"violations,omitempty"`
	Alternatives []TransitionOption `json:"alternatives,omitempty"`
	Allowed      []TransitionOption `json:"allowed,omitempty"`
}

func MarshalEnvelope(envelope Envelope) ([]byte, error) {
	return json.Marshal(envelope)
}
