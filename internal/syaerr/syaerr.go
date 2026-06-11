package syaerr

import (
	"encoding/json"
	"errors"
	"fmt"
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

type GitRequired struct {
	Message string `json:"message"`
}

func (e GitRequired) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "git repository required"
}
func (e GitRequired) Type() string  { return "git_required" }
func (e GitRequired) ExitCode() int { return ExitUsage }

type NotFound struct {
	ID string `json:"id"`
}

func (e NotFound) Error() string { return fmt.Sprintf("not found: %s", e.ID) }
func (e NotFound) Type() string  { return "not_found" }
func (e NotFound) ExitCode() int { return ExitLookup }

type Ambiguous struct {
	Prefix     string      `json:"prefix"`
	Candidates []Candidate `json:"candidates"`
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

type AlreadyClaimed struct {
	Task     string `json:"task"`
	Assignee string `json:"assignee"`
}

func (e AlreadyClaimed) Error() string { return fmt.Sprintf("already claimed by %s", e.Assignee) }
func (e AlreadyClaimed) Type() string  { return "already_claimed" }
func (e AlreadyClaimed) ExitCode() int { return ExitTransitionRejected }

type SchemaInvalid struct {
	Message    string      `json:"message"`
	Violations []Violation `json:"violations,omitempty"`
}

func (e SchemaInvalid) Error() string { return e.Message }
func (e SchemaInvalid) Type() string  { return "schema_invalid" }
func (e SchemaInvalid) ExitCode() int { return ExitSchemaInvalid }

type ErrConflictMarkers struct {
	Path string `json:"path,omitempty"`
}

func (e ErrConflictMarkers) Error() string {
	if e.Path == "" {
		return "conflict markers found"
	}
	return fmt.Sprintf("%s: conflict markers found", e.Path)
}
func (e ErrConflictMarkers) Type() string  { return "conflict_markers" }
func (e ErrConflictMarkers) ExitCode() int { return ExitSchemaInvalid }

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
	Kind      string      `json:"kind"`
	Field     string      `json:"field,omitempty"`
	Relation  string      `json:"relation,omitempty"`
	Section   string      `json:"section,omitempty"`
	File      string      `json:"file,omitempty"`
	Message   string      `json:"message"`
	Hint      string      `json:"hint,omitempty"`
	Offending []Candidate `json:"offending,omitempty"`
}

type Candidate struct {
	ID     string `json:"id"`
	Title  string `json:"title,omitempty"`
	Type   string `json:"type,omitempty"`
	Status string `json:"status,omitempty"`
	File   string `json:"file,omitempty"`
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
	var coded Coded
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return ExitUsage
}

func Payload(err error) ErrorPayload {
	payload := ErrorPayload{
		Type:    "internal",
		Message: err.Error(),
	}
	var coded Coded
	if errors.As(err, &coded) {
		payload.Type = coded.Type()
	}
	var notFound NotFound
	var ambiguous Ambiguous
	var notAllowed TransitionNotAllowed
	var blocked TransitionBlocked
	var alreadyClaimed AlreadyClaimed
	var schemaInvalid SchemaInvalid
	var conflictMarkers ErrConflictMarkers
	var gitRequired GitRequired
	var usage Usage
	switch {
	case errors.As(err, &notFound):
		payload.ID = notFound.ID
	case errors.As(err, &ambiguous):
		payload.Prefix = ambiguous.Prefix
		payload.Candidates = ambiguous.Candidates
	case errors.As(err, &notAllowed):
		payload.Task = notAllowed.Task
		payload.From = notAllowed.From
		payload.To = notAllowed.To
		payload.Allowed = notAllowed.Allowed
	case errors.As(err, &blocked):
		payload.Task = blocked.Task
		payload.Transition = &blocked.Transition
		payload.Violations = blocked.Violations
		payload.Alternatives = blocked.Alternatives
	case errors.As(err, &alreadyClaimed):
		payload.Task = alreadyClaimed.Task
		payload.Assignee = alreadyClaimed.Assignee
	case errors.As(err, &schemaInvalid):
		payload.Violations = schemaInvalid.Violations
	case errors.As(err, &conflictMarkers):
		payload.Path = conflictMarkers.Path
	case errors.As(err, &gitRequired):
	case errors.As(err, &usage):
	}
	return payload
}

func AsCoded(err error) (Coded, bool) {
	var coded Coded
	if errors.As(err, &coded) {
		return coded, true
	}
	return nil, false
}

func ErrorType(err error) string {
	if coded, ok := AsCoded(err); ok {
		return coded.Type()
	}
	return "internal"
}

func ErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	switch e := err.(type) {
	case NotFound:
		return e.Error()
	case Ambiguous:
		return e.Error()
	case TransitionNotAllowed:
		return e.Error()
	case TransitionBlocked:
		return e.Error()
	case AlreadyClaimed:
		return e.Error()
	case SchemaInvalid:
		return e.Error()
	case Usage:
		return e.Error()
	default:
		return err.Error()
	}
}

type ErrorPayload struct {
	Type         string             `json:"type"`
	Message      string             `json:"message,omitempty"`
	ID           string             `json:"id,omitempty"`
	Prefix       string             `json:"prefix,omitempty"`
	Path         string             `json:"path,omitempty"`
	Candidates   []Candidate        `json:"candidates,omitempty"`
	Task         string             `json:"task,omitempty"`
	Assignee     string             `json:"assignee,omitempty"`
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
