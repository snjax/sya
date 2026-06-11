package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snjax/sya/internal/schema"
)

func FuzzSlug(f *testing.F) {
	for _, seed := range []string{"Build API", "../escape", "雪 snow", "a/b\\c", strings.Repeat("x", 200)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, title string) {
		slug := slugify(title)
		if slug == "." || slug == ".." || strings.ContainsAny(slug, `/\`) {
			t.Fatalf("unsafe slug %q from %q", slug, title)
		}
		for _, r := range slug {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
				continue
			}
			t.Fatalf("slug %q contains unsafe rune %q", slug, r)
		}
	})
}

func FuzzDocsRender(f *testing.F) {
	reference, err := os.ReadFile(filepath.Join("..", "schema", "testdata", "reference.yml"))
	if err != nil {
		f.Fatalf("read reference schema: %v", err)
	}
	f.Add(string(reference))
	f.Add(strings.Replace(string(reference), "todo -> in_progress", "todo -> done", 1))
	f.Add(string(reference) + "\n# trailing\n")
	f.Fuzz(func(t *testing.T, input string) {
		sch, err := schema.Parse([]byte(input))
		if err != nil {
			return
		}
		if _, err := renderSchemaGraph(sch, ""); err != nil {
			t.Fatalf("renderSchemaGraph: %v", err)
		}
		if _, err := renderSchemaDocs(sch, ""); err != nil {
			t.Fatalf("renderSchemaDocs: %v", err)
		}
	})
}
