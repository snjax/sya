package plugin_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

type owner struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

type marketplaceManifest struct {
	Name    string              `json:"name"`
	Owner   owner               `json:"owner"`
	Plugins []marketplacePlugin `json:"plugins"`
}

type marketplacePlugin struct {
	Name   string          `json:"name"`
	Source json.RawMessage `json:"source"`
}

type pluginManifest struct {
	Name   string                  `json:"name"`
	Author *owner                  `json:"author,omitempty"`
	Hooks  map[string][]hookTarget `json:"hooks,omitempty"`
}

type hookTarget struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func TestMarketplaceManifest(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	var manifest marketplaceManifest
	readJSON(t, filepath.Join(root, ".claude-plugin", "marketplace.json"), &manifest)

	if !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(manifest.Name) {
		t.Fatalf("marketplace name must be kebab-case, got %q", manifest.Name)
	}
	if strings.TrimSpace(manifest.Owner.Name) == "" {
		t.Fatal("marketplace owner.name is required")
	}
	if len(manifest.Plugins) == 0 {
		t.Fatal("marketplace plugins must not be empty")
	}
	for i, plugin := range manifest.Plugins {
		if strings.TrimSpace(plugin.Name) == "" {
			t.Fatalf("plugins[%d].name is required", i)
		}
		validateMarketplaceSource(t, root, fmt.Sprintf("plugins[%d].source", i), plugin.Source)
	}
}

func TestPluginManifest(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	var manifest pluginManifest
	readJSON(t, filepath.Join(root, "claude-plugin", ".claude-plugin", "plugin.json"), &manifest)

	if strings.TrimSpace(manifest.Name) == "" {
		t.Fatal("plugin name is required")
	}
	if manifest.Author != nil && strings.TrimSpace(manifest.Author.Name) == "" {
		t.Fatal("plugin author.name is required when author is present")
	}
	for event, targets := range manifest.Hooks {
		if strings.TrimSpace(event) == "" {
			t.Fatal("hook event name must not be empty")
		}
		for i, target := range targets {
			if len(target.Hooks) == 0 {
				t.Fatalf("hooks[%q][%d].hooks must not be empty", event, i)
			}
			for j, hook := range target.Hooks {
				if hook.Type != "command" {
					t.Fatalf("hooks[%q][%d].hooks[%d].type = %q, want command", event, i, j, hook.Type)
				}
				if strings.TrimSpace(hook.Command) == "" {
					t.Fatalf("hooks[%q][%d].hooks[%d].command is required", event, i, j)
				}
			}
		}
	}
}

func TestPluginMarkdownFrontmatter(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	commandFiles, err := filepath.Glob(filepath.Join(root, "claude-plugin", "commands", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(commandFiles) == 0 {
		t.Fatal("expected command markdown files")
	}
	for _, path := range commandFiles {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			requireFrontmatterMap(t, path)
		})
	}

	skillFiles, err := filepath.Glob(filepath.Join(root, "claude-plugin", "skills", "*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(skillFiles) == 0 {
		t.Fatal("expected skill markdown files")
	}
	for _, path := range skillFiles {
		path := path
		t.Run(filepath.ToSlash(path[len(root)+1:]), func(t *testing.T) {
			t.Parallel()
			frontmatter := requireFrontmatterMap(t, path)
			for _, key := range []string{"name", "description"} {
				value, ok := frontmatter[key].(string)
				if !ok || strings.TrimSpace(value) == "" {
					t.Fatalf("%s frontmatter requires non-empty %s", path, key)
				}
			}
		})
	}
}

func TestClaudePluginValidate(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not found in PATH")
	}

	root := repoRoot(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "repo-root", args: []string{"plugin", "validate", root}},
		{name: "plugin-dir", args: []string{"plugin", "validate", "claude-plugin"}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command("claude", tc.args...)
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("claude %s failed: %v\n%s", strings.Join(tc.args, " "), err, output)
			}
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func validateMarketplaceSource(t *testing.T, root, field string, raw json.RawMessage) {
	t.Helper()
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		t.Fatalf("%s is required", field)
	}

	var source string
	if err := json.Unmarshal(raw, &source); err == nil {
		requireRelativePathExists(t, root, field, source)
		return
	}

	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("%s must be a relative path string or object: %v", field, err)
	}
	if len(object) == 0 {
		t.Fatalf("%s object must not be empty", field)
	}
	for _, key := range []string{"path", "source"} {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			requireRelativePathExists(t, root, field+"."+key, value)
		}
	}
}

func requireRelativePathExists(t *testing.T, root, field, source string) {
	t.Helper()
	source = strings.TrimSpace(source)
	if source == "" {
		t.Fatalf("%s must not be empty", field)
	}
	if filepath.IsAbs(source) {
		t.Fatalf("%s must be relative, got %q", field, source)
	}
	path := filepath.Join(root, filepath.FromSlash(source))
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s path does not exist: %s", field, path)
		}
		t.Fatalf("stat %s: %v", path, err)
	}
}

func requireFrontmatterMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("---")) {
		t.Fatalf("%s must start with --- frontmatter", path)
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) < 3 || lines[0] != "---" {
		t.Fatalf("%s must start with --- frontmatter", path)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		t.Fatalf("%s frontmatter is missing closing ---", path)
	}

	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &frontmatter); err != nil {
		t.Fatalf("parse %s frontmatter: %v", path, err)
	}
	if len(frontmatter) == 0 {
		t.Fatalf("%s frontmatter must be a non-empty YAML map", path)
	}
	return frontmatter
}
