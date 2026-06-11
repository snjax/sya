package schema

import "fmt"

type guardKindDef struct {
	Evaluate     func(guardEvalContext) *Violation
	ValidateDecl func(guardValidateContext) []Diagnostic
}

type guardEvalContext struct {
	Schema   *Schema
	Resolver Resolver
	Task     TaskView
	Guard    Guard
}

type guardValidateContext struct {
	Result    *ValidationResult
	TypeName  string
	TypeDef   TypeDef
	GuardPath string
	Guard     Guard
}

var guardKindRegistry = map[GuardKind]guardKindDef{
	GuardRelationStatus: {
		Evaluate: func(ctx guardEvalContext) *Violation {
			relation, ok := stringParam(ctx.Guard, "relation")
			if !ok || !relationDeclared(ctx.Schema, relation) {
				return violationPtr(guardViolation(ctx.Guard, ""))
			}
			statuses, ok := stringSliceParam(ctx.Guard, "in")
			if !ok {
				return violationPtr(guardViolation(ctx.Guard, relation))
			}
			violation, failed := evaluateRelatedStatuses(ctx.Schema, ctx.Resolver, ctx.Task.Relations(relation), statuses, guardViolation(ctx.Guard, relation))
			if !failed {
				return nil
			}
			return &violation
		},
		ValidateDecl: func(ctx guardValidateContext) []Diagnostic {
			relation, ok := stringParam(ctx.Guard, "relation")
			if !ok {
				return []Diagnostic{guardDiagnostic("guard_param_missing", ctx.GuardPath+".relation", "relation_status guard must declare relation")}
			}
			var diagnostics []Diagnostic
			if _, declared := ctx.Result.schemaRelations(ctx.TypeName)[relation]; !declared {
				diagnostics = append(diagnostics, guardDiagnostic("guard_relation_unknown", ctx.GuardPath+".relation", fmt.Sprintf("guard references undeclared relation %q", relation)))
			}
			ctx.Result.warnUnknownInStatuses(ctx.GuardPath+".in", ctx.Guard, ctx.Result.relationTargetTypes(ctx.TypeName, relation))
			return diagnostics
		},
	},
	GuardRelationExists: {
		Evaluate: func(ctx guardEvalContext) *Violation {
			relation, ok := stringParam(ctx.Guard, "relation")
			if !ok || !relationDeclared(ctx.Schema, relation) {
				return violationPtr(guardViolation(ctx.Guard, ""))
			}
			if len(ctx.Task.Relations(relation)) == 0 {
				return violationPtr(guardViolation(ctx.Guard, relation))
			}
			return nil
		},
		ValidateDecl: func(ctx guardValidateContext) []Diagnostic {
			relation, ok := stringParam(ctx.Guard, "relation")
			if !ok {
				return []Diagnostic{guardDiagnostic("guard_param_missing", ctx.GuardPath+".relation", "relation_exists guard must declare relation")}
			}
			if _, declared := ctx.Result.schemaRelations(ctx.TypeName)[relation]; !declared {
				return []Diagnostic{guardDiagnostic("guard_relation_unknown", ctx.GuardPath+".relation", fmt.Sprintf("guard references undeclared relation %q", relation))}
			}
			return nil
		},
	},
	GuardField: {
		Evaluate: func(ctx guardEvalContext) *Violation {
			field, ok := stringParam(ctx.Guard, "field")
			if !ok {
				return violationPtr(guardViolation(ctx.Guard, ""))
			}
			value, exists := ctx.Task.Field(field)
			if exists && fieldGuardMatches(ctx.Guard, value) {
				return nil
			}
			violation := guardViolation(ctx.Guard, "")
			violation.Field = field
			return &violation
		},
		ValidateDecl: func(ctx guardValidateContext) []Diagnostic {
			field, ok := stringParam(ctx.Guard, "field")
			if !ok {
				return []Diagnostic{guardDiagnostic("guard_param_missing", ctx.GuardPath+".field", "field guard must declare field")}
			}
			if _, declared := ctx.TypeDef.Fields[field]; !declared {
				return []Diagnostic{guardDiagnostic("guard_field_unknown", ctx.GuardPath+".field", fmt.Sprintf("guard references undeclared field %q", field))}
			}
			return nil
		},
	},
	GuardChildrenStatus: {
		Evaluate: func(ctx guardEvalContext) *Violation {
			statuses, ok := stringSliceParam(ctx.Guard, "in")
			if !ok {
				return violationPtr(guardViolation(ctx.Guard, ""))
			}
			violation, failed := evaluateRelatedStatuses(ctx.Schema, ctx.Resolver, ctx.Task.Children(), statuses, guardViolation(ctx.Guard, ""))
			if !failed {
				return nil
			}
			return &violation
		},
		ValidateDecl: func(ctx guardValidateContext) []Diagnostic {
			ctx.Result.warnUnknownInStatuses(ctx.GuardPath+".in", ctx.Guard, ctx.Result.childrenTargetTypes(ctx.TypeDef))
			return nil
		},
	},
	GuardParentStatus: {
		Evaluate: func(ctx guardEvalContext) *Violation {
			statuses, ok := stringSliceParam(ctx.Guard, "in")
			if !ok {
				return violationPtr(guardViolation(ctx.Guard, ""))
			}
			parentID, ok := ctx.Task.Parent()
			if !ok {
				return violationPtr(guardViolation(ctx.Guard, ""))
			}
			violation, failed := evaluateRelatedStatuses(ctx.Schema, ctx.Resolver, []string{parentID}, statuses, guardViolation(ctx.Guard, ""))
			if !failed {
				return nil
			}
			return &violation
		},
		ValidateDecl: func(ctx guardValidateContext) []Diagnostic {
			ctx.Result.warnUnknownInStatuses(ctx.GuardPath+".in", ctx.Guard, ctx.Result.parentTargetTypes(ctx.TypeName))
			return nil
		},
	},
	GuardSectionNonempty: {
		Evaluate: func(ctx guardEvalContext) *Violation {
			section, ok := stringParam(ctx.Guard, "section")
			if ok && ctx.Task.SectionNonEmpty(section) {
				return nil
			}
			violation := guardViolation(ctx.Guard, "")
			violation.Section = section
			return &violation
		},
		ValidateDecl: func(ctx guardValidateContext) []Diagnostic {
			section, ok := stringParam(ctx.Guard, "section")
			if !ok {
				return []Diagnostic{guardDiagnostic("guard_param_missing", ctx.GuardPath+".section", "section_nonempty guard must declare section")}
			}
			if !makeStringSet(ctx.TypeDef.Sections)[section] {
				return []Diagnostic{guardDiagnostic("guard_section_unknown", ctx.GuardPath+".section", fmt.Sprintf("guard references undeclared section %q", section))}
			}
			return nil
		},
	},
}

func violationPtr(violation Violation) *Violation {
	return &violation
}

func guardDiagnostic(kind, path, message string) Diagnostic {
	return Diagnostic{Kind: kind, Path: path, Message: message}
}
