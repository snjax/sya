package schema

import "testing"

func FuzzTransitionKey(f *testing.F) {
	for _, seed := range []string{
		"todo -> done",
		"* -> scrapped",
		"todo->done",
		"todo -> *",
		"no arrow",
		"雪 -> done",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, key string) {
		from, to, err := parseTransitionKey(key)
		if err != nil {
			return
		}
		if from == "" || to == "" {
			t.Fatalf("empty endpoint from parseTransitionKey(%q): %q -> %q", key, from, to)
		}
		if to == "*" {
			t.Fatalf("wildcard target accepted for %q", key)
		}
	})
}
