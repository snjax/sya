package textsim

import "testing"

func TestSimilar(t *testing.T) {
	t.Parallel()
	docs := []Doc{
		{ID: "a", Text: "Fix schema validation for task fields"},
		{ID: "b", Text: "Fix task field schema validation"},
		{ID: "c", Text: "Render dashboard cards"},
		{ID: "d", Text: "Исправить проверку схемы задачи"},
		{ID: "e", Text: "Исправить проверку schema задачи"},
	}
	pairs := Similar(docs, 0.45)
	if len(pairs) < 2 {
		t.Fatalf("Similar returned %d pairs, want at least 2", len(pairs))
	}
	assertPair(t, pairs, "a", "b")
	assertPair(t, pairs, "d", "e")
	for i := 1; i < len(pairs); i++ {
		if pairs[i-1].Score < pairs[i].Score {
			t.Fatalf("pairs are not score-sorted: %#v", pairs)
		}
	}
}

func TestSimilarThreshold(t *testing.T) {
	t.Parallel()
	pairs := Similar([]Doc{
		{ID: "a", Text: "alpha beta"},
		{ID: "b", Text: "gamma delta"},
	}, 0.9)
	if len(pairs) != 0 {
		t.Fatalf("Similar returned %#v, want no pairs", pairs)
	}
}

func assertPair(t *testing.T, pairs []Pair, a, b string) {
	t.Helper()
	for _, pair := range pairs {
		if pair.A == a && pair.B == b {
			return
		}
	}
	t.Fatalf("pair %s/%s not found in %#v", a, b, pairs)
}
