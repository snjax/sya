package cli

import (
	"fmt"
	"strings"

	"github.com/snjax/sya/internal/syaerr"
)

func humanErrorText(err error, c Colorizer) string {
	payload := syaerr.Payload(err)
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", c.Red("error:"), humanErrorMessage(payload))
	switch payload.Type {
	case "transition_blocked":
		renderViolations(&b, payload.Violations)
		renderTransitionOptions(&b, "alternatives", payload.Alternatives)
	case "transition_not_allowed":
		renderTransitionOptions(&b, "allowed", payload.Allowed)
	case "ambiguous":
		renderCandidates(&b, payload.Candidates)
	case "schema_invalid":
		renderViolations(&b, payload.Violations)
	}
	return b.String()
}

func humanErrorMessage(payload syaerr.ErrorPayload) string {
	switch payload.Type {
	case "transition_blocked":
		if payload.Transition != nil {
			msg := "transition blocked"
			if payload.Transition.From != "" || payload.Transition.To != "" {
				msg += ": " + payload.Transition.From + " -> " + payload.Transition.To
			}
			if payload.Transition.Description != "" {
				msg += " (" + payload.Transition.Description + ")"
			}
			return msg
		}
	case "claim_not_reachable":
		return payload.Message
	}
	return payload.Message
}

func renderViolations(b *strings.Builder, violations []syaerr.Violation) {
	for _, violation := range violations {
		label := violation.Kind
		if violation.Relation != "" {
			label += " " + violation.Relation
		}
		message := violation.Message
		if violation.File != "" {
			message = violation.File + ": " + message
		}
		if len(violation.Offending) > 0 {
			message += ": " + formatCandidatesInline(violation.Offending)
		}
		fmt.Fprintf(b, "\n  - [%s] %s", label, message)
		if violation.Hint != "" {
			fmt.Fprintf(b, "\n    hint: %s", violation.Hint)
		}
	}
}

func renderTransitionOptions(b *strings.Builder, label string, options []syaerr.TransitionOption) {
	if len(options) == 0 {
		return
	}
	parts := make([]string, 0, len(options))
	for _, option := range options {
		parts = append(parts, formatTransitionOption(option))
	}
	fmt.Fprintf(b, "\n  %s: %s", label, strings.Join(parts, ", "))
}

func formatTransitionOption(option syaerr.TransitionOption) string {
	text := option.To
	if option.Kind != "" || option.Description != "" {
		detail := option.Kind
		if option.Description != "" {
			if detail != "" {
				detail += " - "
			}
			detail += option.Description
		}
		text += " (" + detail + ")"
	}
	return text
}

func renderCandidates(b *strings.Builder, candidates []syaerr.Candidate) {
	if len(candidates) == 0 {
		return
	}
	b.WriteString("\n  candidates:")
	for _, candidate := range candidates {
		fmt.Fprintf(b, "\n    - %s", formatCandidate(candidate))
	}
}

func formatCandidatesInline(candidates []syaerr.Candidate) string {
	parts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		parts = append(parts, formatCandidate(candidate))
	}
	return strings.Join(parts, ", ")
}

func formatCandidate(candidate syaerr.Candidate) string {
	text := candidate.ID
	var details []string
	if candidate.Title != "" {
		details = append(details, candidate.Title)
	}
	if candidate.Status != "" {
		details = append(details, candidate.Status)
	}
	if len(details) > 0 {
		text += " (" + strings.Join(details, ", ") + ")"
	}
	return text
}
