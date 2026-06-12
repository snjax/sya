package textsim

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

type Doc struct {
	ID   string
	Text string
}

type Pair struct {
	A     string  `json:"a"`
	B     string  `json:"b"`
	Score float64 `json:"score"`
}

func Similar(docs []Doc, threshold float64) []Pair {
	vectors := make(map[string]map[string]float64, len(docs))
	order := make([]string, 0, len(docs))
	for _, doc := range docs {
		if doc.ID == "" {
			continue
		}
		vectors[doc.ID] = vectorize(doc.Text)
		order = append(order, doc.ID)
	}
	var pairs []Pair
	for i := 0; i < len(order); i++ {
		for j := i + 1; j < len(order); j++ {
			score := cosine(vectors[order[i]], vectors[order[j]])
			if score >= threshold {
				a, b := order[i], order[j]
				if b < a {
					a, b = b, a
				}
				pairs = append(pairs, Pair{A: a, B: b, Score: score})
			}
		}
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].Score != pairs[j].Score {
			return pairs[i].Score > pairs[j].Score
		}
		if pairs[i].A != pairs[j].A {
			return pairs[i].A < pairs[j].A
		}
		return pairs[i].B < pairs[j].B
	})
	return pairs
}

func vectorize(text string) map[string]float64 {
	normalized := normalize(text)
	terms := make(map[string]float64)
	for _, token := range strings.Fields(normalized) {
		terms["tok:"+token]++
	}
	runes := []rune(strings.ReplaceAll(normalized, " ", ""))
	for i := 0; i+3 <= len(runes); i++ {
		terms["tri:"+string(runes[i:i+3])]++
	}
	return terms
}

func normalize(text string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func cosine(a, b map[string]float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	var dot, magA, magB float64
	for term, left := range a {
		dot += left * b[term]
		magA += left * left
	}
	for _, right := range b {
		magB += right * right
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}
