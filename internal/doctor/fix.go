package doctor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/task"
)

type Change struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Message string `json:"message,omitempty"`
}

func FixMerge(path string) ([]Change, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	oursBytes, theirsBytes, err := splitConflictVersions(data)
	if err != nil {
		return nil, err
	}
	ours, err := task.ParseBytes(oursBytes)
	if err != nil {
		return nil, fmt.Errorf("ours side is not a valid task: %w", err)
	}
	theirs, err := task.ParseBytes(theirsBytes)
	if err != nil {
		return nil, fmt.Errorf("theirs side is not a valid task: %w", err)
	}
	if !taskFrontmatterEqual(ours, theirs) || !nonLogSectionsEqual(ours, theirs) {
		return nil, fmt.Errorf("conflict touches frontmatter or non-Log sections")
	}

	merged := *ours
	merged.Body.Sections = append([]task.Section(nil), ours.Body.Sections...)
	setLogSection(&merged, mergeLogRaw(logRaw(ours), logRaw(theirs)))
	out, err := task.Serialize(&merged)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return nil, err
	}
	return []Change{{Path: path, Action: "fix_merge", Message: "merged Log conflict"}}, nil
}

func ReassignID(idx *index.Index, oldID string) ([]Change, error) {
	return ReassignIDInDir("", idx, oldID)
}

func ReassignIDInDir(projectDir string, idx *index.Index, oldID string) ([]Change, error) {
	target, err := idx.Get(oldID)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]struct{})
	for _, t := range idx.All() {
		if t.ID != oldID {
			existing[t.ID] = struct{}{}
		}
	}
	newID, err := task.NewID(existing, task.DefaultIDLength)
	if err != nil {
		return nil, err
	}

	var changes []Change
	oldPath := resolvePath(projectDir, target.File)
	newPath := reassignedPath(oldPath, oldID, newID)
	target.ID = newID
	if err := writeTask(newPath, target); err != nil {
		return nil, err
	}
	if oldPath != newPath {
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	changes = append(changes, Change{Path: target.File, Action: "reassign_id", From: oldID, To: newID})

	for _, t := range idx.All() {
		if t == target {
			continue
		}
		changed := false
		if t.Parent == oldID {
			t.Parent = newID
			changed = true
		}
		for relation, targets := range t.Relations {
			for i, targetID := range targets {
				if targetID == oldID {
					targets[i] = newID
					changed = true
				}
			}
			t.Relations[relation] = compactSorted(targets)
		}
		if !changed {
			continue
		}
		if err := writeTask(resolvePath(projectDir, t.File), t); err != nil {
			return nil, err
		}
		changes = append(changes, Change{Path: t.File, Action: "update_reference", From: oldID, To: newID})
	}
	return changes, nil
}

func FixSymmetricDup(projectDir string, idx *index.Index, sch *schema.Schema) ([]Change, error) {
	var changes []Change
	for _, t := range idx.All() {
		changed := false
		for relation, targets := range t.Relations {
			relationDef, ok := sch.Relations[relation]
			if !ok || !relationDef.Symmetric {
				continue
			}
			var kept []string
			for _, targetID := range targets {
				if targetID == "" || t.ID > targetID {
					changed = true
					continue
				}
				if contains(kept, targetID) {
					changed = true
					continue
				}
				kept = append(kept, targetID)
			}
			sort.Strings(kept)
			t.Relations[relation] = kept
		}
		if !changed {
			continue
		}
		if err := writeTask(resolvePath(projectDir, t.File), t); err != nil {
			return nil, err
		}
		changes = append(changes, Change{Path: t.File, Action: "fix_symmetric_duplicate"})
	}
	return changes, nil
}

func splitConflictVersions(data []byte) ([]byte, []byte, error) {
	var ours, theirs bytes.Buffer
	for len(data) > 0 {
		start := bytes.Index(data, []byte("<<<<<<<"))
		if start < 0 {
			ours.Write(data)
			theirs.Write(data)
			return ours.Bytes(), theirs.Bytes(), nil
		}
		ours.Write(data[:start])
		theirs.Write(data[:start])
		hunkEnd := bytes.IndexByte(data[start:], '\n')
		if hunkEnd < 0 {
			return nil, nil, fmt.Errorf("unterminated conflict start")
		}
		bodyStart := start + hunkEnd + 1
		midRel := bytes.Index(data[bodyStart:], []byte("\n======="))
		if midRel < 0 {
			return nil, nil, fmt.Errorf("missing conflict separator")
		}
		mid := bodyStart + midRel
		theirsStartRel := bytes.IndexByte(data[mid+1:], '\n')
		if theirsStartRel < 0 {
			return nil, nil, fmt.Errorf("unterminated conflict separator")
		}
		theirsStart := mid + 1 + theirsStartRel + 1
		endRel := bytes.Index(data[theirsStart:], []byte("\n>>>>>>>"))
		if endRel < 0 {
			return nil, nil, fmt.Errorf("missing conflict end")
		}
		end := theirsStart + endRel
		endLineRel := bytes.IndexByte(data[end+1:], '\n')
		next := len(data)
		if endLineRel >= 0 {
			next = end + 1 + endLineRel + 1
		}
		ours.Write(data[bodyStart:mid])
		theirs.Write(data[theirsStart:end])
		data = data[next:]
	}
	return ours.Bytes(), theirs.Bytes(), nil
}

func nonLogSectionsEqual(left, right *task.Task) bool {
	return bytes.Equal(nonLogRaw(left), nonLogRaw(right))
}

func nonLogRaw(t *task.Task) []byte {
	var out []byte
	for _, section := range t.Body.Sections {
		if section.Name == "Log" {
			continue
		}
		out = append(out, section.Raw...)
	}
	return out
}

func logRaw(t *task.Task) []byte {
	for _, section := range t.Body.Sections {
		if section.Name == "Log" {
			return append([]byte(nil), section.Raw...)
		}
	}
	return []byte("## Log\n")
}

func mergeLogRaw(ours, theirs []byte) []byte {
	lines := append(logLines(ours), logLines(theirs)...)
	seen := make(map[string]bool, len(lines))
	out := []byte("## Log\n")
	for _, line := range lines {
		if seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out
}

func logLines(raw []byte) []string {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	var out []string
	for i, line := range lines {
		if i == 0 && strings.HasPrefix(line, "## Log") {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func setLogSection(t *task.Task, raw []byte) {
	for i, section := range t.Body.Sections {
		if section.Name == "Log" {
			t.Body.Sections[i].Raw = raw
			rebuildBodyRaw(t)
			return
		}
	}
	t.Body.Sections = append(t.Body.Sections, task.Section{Name: "Log", Raw: raw})
	rebuildBodyRaw(t)
}

func rebuildBodyRaw(t *task.Task) {
	t.Body.Raw = nil
	for _, section := range t.Body.Sections {
		t.Body.Raw = append(t.Body.Raw, section.Raw...)
	}
}

func writeTask(path string, t *task.Task) error {
	out, err := task.Serialize(t)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func resolvePath(projectDir, path string) string {
	if projectDir == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(projectDir, filepath.FromSlash(path))
}

func reassignedPath(path, oldID, newID string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if strings.HasPrefix(base, oldID+"-") {
		return filepath.Join(dir, newID+strings.TrimPrefix(base, oldID))
	}
	return filepath.Join(dir, newID+".md")
}

func compactSorted(values []string) []string {
	sort.Strings(values)
	var out []string
	for _, value := range values {
		if value == "" || contains(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}
