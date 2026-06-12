package query

import "testing"

func FuzzQuery(f *testing.F) {
	seeds := []string{
		`type=task`,
		`ready and not archived`,
		`(type=feature or type=bug) and age>7d`,
		`field.ready=true`,
		`rel.depends_on in (a,b,c)`,
		`title~"api gateway"`,
		`))))`,
		`not not not`,
		"\"\x00",
		`field.=bad`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		expr, err := Parse(input)
		if err != nil {
			return
		}
		if _, err := Parse(expr.String()); err != nil {
			t.Fatalf("stable parse failed: %q -> %q: %v", input, expr.String(), err)
		}
	})
}
