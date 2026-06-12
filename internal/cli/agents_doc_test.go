package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureAgentDocsCreatesAgentsOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	changes := ensureAgentDocsOK(t, root)
	assertAgentDocChange(t, changes, "AGENTS.md", "created")
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("CLAUDE.md should not be created, err=%v", err)
	}
	assertManagedBlock(t, filepath.Join(root, "AGENTS.md"), 1)
}

func TestEnsureAgentDocsAppendsToExisting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "AGENTS.md")
	writeFile(t, path, "Existing guidance.\n")

	changes := ensureAgentDocsOK(t, root)
	assertAgentDocChange(t, changes, "AGENTS.md", "appended")
	data := readFile(t, path)
	if !strings.HasPrefix(data, "Existing guidance.\n\n"+agentDocBegin) {
		t.Fatalf("existing content not preserved before block:\n%s", data)
	}
	assertManagedBlock(t, path, 1)
}

func TestEnsureAgentDocsRerunIsByteIdentical(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	ensureAgentDocsOK(t, root)
	before := readFile(t, filepath.Join(root, "AGENTS.md"))

	changes := ensureAgentDocsOK(t, root)
	if len(changes) != 0 {
		t.Fatalf("second run changes = %#v, want none", changes)
	}
	after := readFile(t, filepath.Join(root, "AGENTS.md"))
	if after != before {
		t.Fatalf("second run changed bytes\nbefore:\n%s\n\nafter:\n%s", before, after)
	}
}

func TestEnsureAgentDocsReplacesManagedBlock(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "AGENTS.md")
	writeFile(t, path, "Intro.\n\n"+agentDocBegin+"\nold content\n"+agentDocEnd+"\n\nTail.\n")

	changes := ensureAgentDocsOK(t, root)
	assertAgentDocChange(t, changes, "AGENTS.md", "updated")
	data := readFile(t, path)
	if strings.Contains(data, "old content") {
		t.Fatalf("old managed block remains:\n%s", data)
	}
	if !strings.Contains(data, "Intro.") || !strings.Contains(data, "Tail.") {
		t.Fatalf("unmanaged content not preserved:\n%s", data)
	}
	assertManagedBlock(t, path, 1)
}

func TestEnsureAgentDocsClaudeSymlinkToAgents(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	agentsPath := filepath.Join(root, "AGENTS.md")
	claudePath := filepath.Join(root, "CLAUDE.md")
	writeFile(t, agentsPath, "Shared guidance.\n")
	if err := os.Symlink("AGENTS.md", claudePath); err != nil {
		t.Fatal(err)
	}

	changes := ensureAgentDocsOK(t, root)
	assertAgentDocChange(t, changes, "AGENTS.md", "appended")
	assertAgentDocChange(t, changes, "CLAUDE.md", "skipped")
	if info, err := os.Lstat(claudePath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("CLAUDE.md should remain a symlink, info=%v err=%v", info, err)
	}
	assertManagedBlock(t, agentsPath, 1)
	assertManagedBlock(t, claudePath, 1)
}

func TestEnsureAgentDocsAgentsSymlinkToClaude(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	agentsPath := filepath.Join(root, "AGENTS.md")
	claudePath := filepath.Join(root, "CLAUDE.md")
	writeFile(t, claudePath, "Shared guidance.\n")
	if err := os.Symlink("CLAUDE.md", agentsPath); err != nil {
		t.Fatal(err)
	}

	changes := ensureAgentDocsOK(t, root)
	assertAgentDocChange(t, changes, "AGENTS.md", "appended")
	assertAgentDocChange(t, changes, "CLAUDE.md", "skipped")
	if info, err := os.Lstat(agentsPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("AGENTS.md should remain a symlink, info=%v err=%v", info, err)
	}
	assertManagedBlock(t, agentsPath, 1)
	assertManagedBlock(t, claudePath, 1)
}

func TestEnsureAgentDocsHardlinkedPair(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	agentsPath := filepath.Join(root, "AGENTS.md")
	claudePath := filepath.Join(root, "CLAUDE.md")
	writeFile(t, agentsPath, "Shared guidance.\n")
	if err := os.Link(agentsPath, claudePath); err != nil {
		t.Fatal(err)
	}
	beforeAgents, beforeClaude := statOK(t, agentsPath), statOK(t, claudePath)
	if !os.SameFile(beforeAgents, beforeClaude) {
		t.Fatal("test setup did not create hardlinked files")
	}

	changes := ensureAgentDocsOK(t, root)
	assertAgentDocChange(t, changes, "AGENTS.md", "appended")
	assertAgentDocChange(t, changes, "CLAUDE.md", "skipped")
	afterAgents, afterClaude := statOK(t, agentsPath), statOK(t, claudePath)
	if !os.SameFile(afterAgents, afterClaude) || !os.SameFile(beforeAgents, afterAgents) {
		t.Fatal("hardlink pair was broken by write")
	}
	assertManagedBlock(t, agentsPath, 1)
	assertManagedBlock(t, claudePath, 1)
}

func TestEnsureAgentDocsSeparateFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "AGENTS.md"), "Agents.\n")
	writeFile(t, filepath.Join(root, "CLAUDE.md"), "Claude.\n")

	changes := ensureAgentDocsOK(t, root)
	assertAgentDocChange(t, changes, "AGENTS.md", "appended")
	assertAgentDocChange(t, changes, "CLAUDE.md", "appended")
	assertManagedBlock(t, filepath.Join(root, "AGENTS.md"), 1)
	assertManagedBlock(t, filepath.Join(root, "CLAUDE.md"), 1)
}

func ensureAgentDocsOK(t *testing.T, root string) []AgentDocChange {
	t.Helper()
	changes, err := EnsureAgentDocs(root, AgentDocOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return changes
}

func assertAgentDocChange(t *testing.T, changes []AgentDocChange, base, action string) {
	t.Helper()
	for _, change := range changes {
		if filepath.Base(change.Path) == base && change.Action == action {
			return
		}
	}
	t.Fatalf("missing %s %s change in %#v", action, base, changes)
}

func assertManagedBlock(t *testing.T, path string, wantCount int) {
	t.Helper()
	data := readFile(t, path)
	if count := strings.Count(data, agentDocBegin); count != wantCount {
		t.Fatalf("%s begin marker count = %d, want %d\n%s", path, count, wantCount, data)
	}
	if count := strings.Count(data, agentDocEnd); count != wantCount {
		t.Fatalf("%s end marker count = %d, want %d\n%s", path, count, wantCount, data)
	}
	if !strings.Contains(data, "sya prime") || !strings.Contains(data, "sya schema docs") || !strings.Contains(data, ".sya/tasks/") {
		t.Fatalf("%s missing lean sya guidance:\n%s", path, data)
	}
}

func statOK(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
