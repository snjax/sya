package task

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/snjax/sya/internal/syaerr"
)

const DefaultIDLength = 6
const MinIDLength = 4
const MaxIDLength = 12

func NewID(existing map[string]struct{}, length int) (string, error) {
	length = ClampIDLength(length)
	buf := make([]byte, (length+1)/2)
	for {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		id := hex.EncodeToString(buf)[:length]
		if validNewID(existing, id) {
			return id, nil
		}
	}
}

func ClampIDLength(length int) int {
	if length <= 0 {
		return DefaultIDLength
	}
	if length < MinIDLength {
		return MinIDLength
	}
	if length > MaxIDLength {
		return MaxIDLength
	}
	return length
}

func ResolvePrefix(ids []string, prefix string) (string, []string, error) {
	var matches []string
	for _, id := range ids {
		if id == prefix {
			return id, nil, nil
		}
		if strings.HasPrefix(id, prefix) {
			matches = append(matches, id)
		}
	}
	sort.Strings(matches)
	switch len(matches) {
	case 0:
		return "", nil, syaerr.NotFound{ID: prefix}
	case 1:
		return matches[0], nil, nil
	default:
		return "", matches, syaerr.Ambiguous{Prefix: prefix, Candidates: candidates(matches)}
	}
}

func validNewID(existing map[string]struct{}, id string) bool {
	for existingID := range existing {
		if id == existingID || strings.HasPrefix(id, existingID) || strings.HasPrefix(existingID, id) {
			return false
		}
	}
	return true
}

func candidates(ids []string) []syaerr.Candidate {
	out := make([]syaerr.Candidate, 0, len(ids))
	for _, id := range ids {
		out = append(out, syaerr.Candidate{ID: id})
	}
	return out
}
