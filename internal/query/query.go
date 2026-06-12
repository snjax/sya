package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Task interface {
	ID() string
	Type() string
	Status() string
	Priority() string
	Title() string
	Assignee() string
	Parent() string
	Labels() []string
	Field(name string) (any, bool)
	Relation(name string) []string
	Created() time.Time
	Archived() bool
	Terminal() bool
	Working() bool
	Parked() bool
	Ready() bool
	Blocked() bool
	DeadEnd() bool
}

type Predicate func(Task) bool

type Options struct {
	Now time.Time
}

type ParseError struct {
	Pos      int
	Expected string
	Got      string
}

func (e ParseError) Error() string {
	if e.Got == "" {
		return fmt.Sprintf("query parse error at position %d: expected %s", e.Pos, e.Expected)
	}
	return fmt.Sprintf("query parse error at position %d: expected %s, got %s", e.Pos, e.Expected, e.Got)
}

type Expr interface {
	Eval(Task, Options) bool
	String() string
	References(key string) bool
}

func Parse(input string) (Expr, error) {
	p := parser{tokens: lex(input)}
	expr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().typ != tokenEOF {
		tok := p.peek()
		return nil, ParseError{Pos: tok.pos, Expected: "end of expression", Got: tok.text}
	}
	return expr, nil
}

func Compile(input string, opts Options) (Predicate, Expr, error) {
	expr, err := Parse(input)
	if err != nil {
		return nil, nil, err
	}
	return func(task Task) bool {
		return expr.Eval(task, opts)
	}, expr, nil
}

type binaryExpr struct {
	op          string
	left, right Expr
}

func (e binaryExpr) Eval(task Task, opts Options) bool {
	switch e.op {
	case "and":
		return e.left.Eval(task, opts) && e.right.Eval(task, opts)
	case "or":
		return e.left.Eval(task, opts) || e.right.Eval(task, opts)
	default:
		return false
	}
}

func (e binaryExpr) String() string {
	return "(" + e.left.String() + " " + e.op + " " + e.right.String() + ")"
}

func (e binaryExpr) References(key string) bool {
	return e.left.References(key) || e.right.References(key)
}

type notExpr struct {
	expr Expr
}

func (e notExpr) Eval(task Task, opts Options) bool { return !e.expr.Eval(task, opts) }
func (e notExpr) String() string                    { return "(not " + e.expr.String() + ")" }
func (e notExpr) References(key string) bool        { return e.expr.References(key) }

type boolExpr struct {
	key string
}

func (e boolExpr) Eval(task Task, opts Options) bool {
	switch e.key {
	case "ready":
		return task.Ready()
	case "blocked":
		return task.Blocked()
	case "archived":
		return task.Archived()
	case "terminal":
		return task.Terminal()
	case "working":
		return task.Working()
	case "parked":
		return task.Parked()
	case "dead_end":
		return task.DeadEnd()
	default:
		if strings.HasPrefix(e.key, "rel.") {
			return len(task.Relation(strings.TrimPrefix(e.key, "rel."))) > 0
		}
	}
	return false
}

func (e boolExpr) String() string             { return e.key }
func (e boolExpr) References(key string) bool { return e.key == key }

type compareExpr struct {
	key    string
	op     string
	values []value
}

func (e compareExpr) Eval(task Task, opts Options) bool {
	if e.op == "in" {
		for _, value := range e.values {
			if compareTaskValue(task, opts, e.key, "=", value) {
				return true
			}
		}
		return false
	}
	if len(e.values) == 0 {
		return false
	}
	return compareTaskValue(task, opts, e.key, e.op, e.values[0])
}

func (e compareExpr) String() string {
	if e.op == "in" {
		parts := make([]string, 0, len(e.values))
		for _, value := range e.values {
			parts = append(parts, value.String())
		}
		return e.key + " in (" + strings.Join(parts, ",") + ")"
	}
	return e.key + e.op + e.values[0].String()
}

func (e compareExpr) References(key string) bool { return e.key == key }

type value struct {
	raw    string
	quoted bool
}

func (v value) String() string {
	if !v.quoted && safeBareValue(v.raw) {
		return v.raw
	}
	return strconv.Quote(v.raw)
}

func compareTaskValue(task Task, opts Options, key, op string, expected value) bool {
	switch key {
	case "age":
		left, ok := taskAge(task, opts)
		if !ok {
			return false
		}
		right, ok := parseDuration(expected.raw)
		if !ok {
			return false
		}
		return compareDuration(left, op, right)
	case "priority":
		left, lok := priorityRank(task.Priority())
		right, rok := priorityRank(expected.raw)
		if lok && rok && isOrderedOp(op) {
			return compareInt(left, op, right)
		}
		return compareString(task.Priority(), op, expected.raw)
	case "label":
		return compareStringSlice(task.Labels(), op, expected.raw)
	default:
		if strings.HasPrefix(key, "field.") {
			val, ok := task.Field(strings.TrimPrefix(key, "field."))
			if !ok {
				return op == "!="
			}
			return compareString(fmt.Sprint(val), op, expected.raw)
		}
		if strings.HasPrefix(key, "rel.") {
			return compareStringSlice(task.Relation(strings.TrimPrefix(key, "rel.")), op, expected.raw)
		}
		return compareString(stringValue(task, key), op, expected.raw)
	}
}

func stringValue(task Task, key string) string {
	switch key {
	case "id":
		return task.ID()
	case "type":
		return task.Type()
	case "status":
		return task.Status()
	case "title":
		return task.Title()
	case "assignee":
		return task.Assignee()
	case "parent":
		return task.Parent()
	default:
		return ""
	}
}

func compareString(left, op, right string) bool {
	switch op {
	case "=":
		return left == right
	case "!=":
		return left != right
	case "~":
		return strings.Contains(strings.ToLower(left), strings.ToLower(right))
	case ">", ">=", "<", "<=":
		return compareInt(strings.Compare(left, right), normalizeCompareOp(op), 0)
	default:
		return false
	}
}

func compareStringSlice(values []string, op, expected string) bool {
	found := false
	for _, value := range values {
		if value == expected {
			found = true
			break
		}
	}
	switch op {
	case "=":
		return found
	case "!=":
		return !found
	case "~":
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), strings.ToLower(expected)) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func compareDuration(left time.Duration, op string, right time.Duration) bool {
	return compareInt64(int64(left), op, int64(right))
}

func compareInt(left int, op string, right int) bool {
	return compareInt64(int64(left), op, int64(right))
}

func compareInt64(left int64, op string, right int64) bool {
	switch op {
	case "=":
		return left == right
	case "!=":
		return left != right
	case ">":
		return left > right
	case ">=":
		return left >= right
	case "<":
		return left < right
	case "<=":
		return left <= right
	default:
		return false
	}
}

func normalizeCompareOp(op string) string {
	switch op {
	case ">":
		return ">"
	case ">=":
		return ">="
	case "<":
		return "<"
	case "<=":
		return "<="
	default:
		return op
	}
}

func isOrderedOp(op string) bool {
	switch op {
	case ">", ">=", "<", "<=":
		return true
	default:
		return false
	}
}

func taskAge(task Task, opts Options) (time.Duration, bool) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	created := task.Created()
	if created.IsZero() {
		return 0, false
	}
	if now.Before(created) {
		return 0, true
	}
	return now.Sub(created), true
}

func parseDuration(raw string) (time.Duration, bool) {
	if len(raw) < 2 {
		return 0, false
	}
	unit := raw[len(raw)-1]
	n, err := strconv.Atoi(raw[:len(raw)-1])
	if err != nil || n < 0 {
		return 0, false
	}
	switch unit {
	case 'h':
		return time.Duration(n) * time.Hour, true
	case 'd':
		return time.Duration(n) * 24 * time.Hour, true
	case 'w':
		return time.Duration(n) * 7 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

func priorityRank(priority string) (int, bool) {
	switch strings.ToLower(priority) {
	case "critical":
		return 5, true
	case "high":
		return 4, true
	case "normal", "":
		return 3, true
	case "low":
		return 2, true
	case "deferred":
		return 1, true
	default:
		return 0, false
	}
}

func safeBareValue(raw string) bool {
	if raw == "" {
		return false
	}
	for _, r := range raw {
		if !(r == '_' || r == '-' || r == '.' || r == '/' || r == ':' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return false
		}
	}
	return true
}
