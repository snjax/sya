package task

import (
	"errors"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/snjax/sya/internal/syaerr"
)

func TestNewID(t *testing.T) {
	t.Parallel()

	existing := map[string]struct{}{
		"aaaaaa": {},
		"bbbbbb": {},
	}
	got, err := NewID(existing, DefaultIDLength)
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{6}$`).MatchString(got) {
		t.Fatalf("NewID() = %q, want 6 lowercase hex chars", got)
	}
	if !validNewID(existing, got) {
		t.Fatalf("NewID() = %q violates prefix property against %#v", got, existing)
	}
}

func TestValidNewIDPrefixPropertyExhaustive(t *testing.T) {
	t.Parallel()

	ids := shortHexIDs(2)
	for _, candidate := range ids {
		for _, existingID := range ids {
			existing := map[string]struct{}{existingID: {}}
			got := validNewID(existing, candidate)
			want := candidate != existingID &&
				!strings.HasPrefix(candidate, existingID) &&
				!strings.HasPrefix(existingID, candidate)
			if got != want {
				t.Fatalf("validNewID(%q vs %q) = %v, want %v", candidate, existingID, got, want)
			}
		}
	}
}

func TestResolvePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ids            []string
		prefix         string
		wantID         string
		wantCandidates []string
		wantErr        any
	}{
		{
			name:   "exact wins over ambiguous prefix",
			ids:    []string{"abc123", "abc999", "abc"},
			prefix: "abc",
			wantID: "abc",
		},
		{
			name:   "unique prefix",
			ids:    []string{"abc123", "def456"},
			prefix: "abc",
			wantID: "abc123",
		},
		{
			name:           "ambiguous sorted",
			ids:            []string{"abc999", "abc123", "def456"},
			prefix:         "abc",
			wantCandidates: []string{"abc123", "abc999"},
			wantErr:        syaerr.Ambiguous{},
		},
		{
			name:    "not found",
			ids:     []string{"abc999"},
			prefix:  "def",
			wantErr: syaerr.NotFound{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotID, gotCandidates, err := ResolvePrefix(tt.ids, tt.prefix)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("ResolvePrefix() error = nil, want %T", tt.wantErr)
				}
				switch tt.wantErr.(type) {
				case syaerr.Ambiguous:
					var got syaerr.Ambiguous
					if !errors.As(err, &got) {
						t.Fatalf("error = %T %v, want Ambiguous", err, err)
					}
				case syaerr.NotFound:
					var got syaerr.NotFound
					if !errors.As(err, &got) {
						t.Fatalf("error = %T %v, want NotFound", err, err)
					}
				}
				if !reflect.DeepEqual(gotCandidates, tt.wantCandidates) {
					t.Fatalf("candidates = %#v, want %#v", gotCandidates, tt.wantCandidates)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolvePrefix() error = %v", err)
			}
			if gotID != tt.wantID {
				t.Fatalf("id = %q, want %q", gotID, tt.wantID)
			}
			if len(gotCandidates) != 0 {
				t.Fatalf("candidates = %#v, want none", gotCandidates)
			}
		})
	}
}

func FuzzResolvePrefix(f *testing.F) {
	f.Add("abc123,abc999,def456", "abc")
	f.Add("abc123,def456", "abc")
	f.Add("", "abc")

	f.Fuzz(func(t *testing.T, csv, prefix string) {
		ids := splitIDs(csv)
		got, candidates, err := ResolvePrefix(ids, prefix)
		if err != nil {
			if len(candidates) > 1 && !sort.StringsAreSorted(candidates) {
				t.Fatalf("ambiguous candidates are not sorted: %#v", candidates)
			}
			return
		}
		if got == "" {
			t.Fatalf("empty id without error")
		}
		if got != prefix && !strings.HasPrefix(got, prefix) {
			t.Fatalf("resolved %q for prefix %q", got, prefix)
		}
	})
}

func shortHexIDs(maxLen int) []string {
	var ids []string
	var walk func(prefix string, depth int)
	walk = func(prefix string, depth int) {
		if depth > 0 {
			ids = append(ids, prefix)
		}
		if depth == maxLen {
			return
		}
		for _, ch := range "0123456789abcdef" {
			walk(prefix+string(ch), depth+1)
		}
	}
	walk("", 0)
	return ids
}

func splitIDs(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	ids := parts[:0]
	seen := make(map[string]struct{})
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}
