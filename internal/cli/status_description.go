package cli

import (
	"strings"
	"unicode/utf8"

	"github.com/snjax/sya/internal/schema"
)

func statusDescription(sch *schema.Schema, typeName, status string) string {
	if sch == nil {
		return ""
	}
	typeDef, ok := sch.Types[typeName]
	if !ok {
		return ""
	}
	return typeDef.Statuses[status]
}

func formatStatusWithDescription(status, description string) string {
	if description == "" {
		return status
	}
	return status + " — " + description
}

func truncateStatusDescription(description string) string {
	description = strings.TrimSpace(description)
	if utf8.RuneCountInString(description) <= 60 {
		return description
	}
	runes := []rune(description)
	return strings.TrimSpace(string(runes[:60])) + "..."
}
