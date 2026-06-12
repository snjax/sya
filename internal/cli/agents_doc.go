package cli

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	agentDocBegin = "<!-- BEGIN SYA INTEGRATION -->"
	agentDocEnd   = "<!-- END SYA INTEGRATION -->"
)

const defaultAgentDocBlock = `<!-- BEGIN SYA INTEGRATION -->
## sya Integration

- Use the ` + "`sya`" + ` CLI for all task tracking; do not read ` + "`.sya/tasks/`" + ` directly.
- Start sessions with ` + "`sya prime`" + `.
- Run ` + "`sya schema docs`" + ` for the full workflow and guard semantics.
- Find work: ` + "`sya ready`" + `; claim: ` + "`sya claim <id>`" + `.
- Move work: ` + "`sya move <id> <status>`" + `; inspect options: ` + "`sya transitions <id>`" + `.
- Record progress: ` + "`sya comment <id> -m \"...\"`" + `.
- Finish allowed work: ` + "`sya close <id> --reason \"...\"`" + `.
<!-- END SYA INTEGRATION -->
`

type AgentDocOptions struct {
	Block string
}

type AgentDocChange struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
}

func EnsureAgentDocs(dir string, opts AgentDocOptions) ([]AgentDocChange, error) {
	block := opts.Block
	if block == "" {
		block = defaultAgentDocBlock
	}
	if !strings.HasSuffix(block, "\n") {
		block += "\n"
	}

	agentsPath := filepath.Join(dir, "AGENTS.md")
	var changes []AgentDocChange
	change, wrote, err := ensureAgentDocFile(agentsPath, block, true)
	if err != nil {
		return nil, err
	}
	if wrote {
		changes = append(changes, change)
	}

	claudePath := filepath.Join(dir, "CLAUDE.md")
	claudeInfo, err := os.Stat(claudePath)
	if err != nil {
		if os.IsNotExist(err) {
			return changes, nil
		}
		return nil, err
	}
	agentsInfo, err := os.Stat(agentsPath)
	if err != nil {
		return nil, err
	}
	if os.SameFile(agentsInfo, claudeInfo) {
		changes = append(changes, AgentDocChange{
			Path:    claudePath,
			Action:  "skipped",
			Message: "same file as AGENTS.md",
		})
		return changes, nil
	}
	change, wrote, err = ensureAgentDocFile(claudePath, block, false)
	if err != nil {
		return nil, err
	}
	if wrote {
		changes = append(changes, change)
	}
	return changes, nil
}

func ensureAgentDocFile(path, block string, create bool) (AgentDocChange, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) || !create {
			return AgentDocChange{}, false, err
		}
		if err := writeAgentDocInPlace(path, []byte(block)); err != nil {
			return AgentDocChange{}, false, err
		}
		return AgentDocChange{Path: path, Action: "created", Message: "created managed sya block"}, true, nil
	}

	updated, action := ensureAgentDocBlock(string(data), block)
	if updated == string(data) {
		return AgentDocChange{}, false, nil
	}
	if err := writeAgentDocInPlace(path, []byte(updated)); err != nil {
		return AgentDocChange{}, false, err
	}
	return AgentDocChange{Path: path, Action: action, Message: action + " managed sya block"}, true, nil
}

func ensureAgentDocBlock(content, block string) (string, string) {
	begin := strings.Index(content, agentDocBegin)
	end := strings.Index(content, agentDocEnd)
	if begin >= 0 && end >= begin {
		end += len(agentDocEnd)
		updated := content[:begin] + strings.TrimRight(block, "\n") + content[end:]
		if strings.HasSuffix(content[end:], "\n") {
			return updated, "updated"
		}
		return updated + "\n", "updated"
	}

	prefix := content
	if prefix != "" && !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
	}
	if strings.TrimSpace(prefix) != "" && !strings.HasSuffix(prefix, "\n\n") {
		prefix += "\n"
	}
	return prefix + block, "appended"
}

func writeAgentDocInPlace(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// These files are intentionally written in place: temp+rename would silently
	// break symlink and hardlink pairs that users rely on for agent docs.
	return os.WriteFile(path, data, 0o644)
}
