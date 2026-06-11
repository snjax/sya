package gitx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

type Error struct {
	Args   []string
	Output string
	Err    error
}

func (e Error) Error() string {
	output := strings.TrimSpace(e.Output)
	if output == "" {
		return fmt.Sprintf("git %s: %v", strings.Join(e.Args, " "), e.Err)
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), output)
}

func (e Error) Unwrap() error {
	return e.Err
}

func (ExecRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_MERGE_AUTOEDIT=no")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return out.Bytes(), Error{Args: append([]string(nil), args...), Output: out.String(), Err: err}
	}
	return out.Bytes(), nil
}

func RequireRepo(ctx context.Context, runner Runner, dir string) error {
	out, err := runner.Run(ctx, dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != "true" {
		return fmt.Errorf("not inside a git worktree")
	}
	return nil
}

func Show(ctx context.Context, runner Runner, dir, rev, path string) ([]byte, error) {
	return runner.Run(ctx, dir, "show", rev+":"+path)
}

func FollowLog(ctx context.Context, runner Runner, dir, path string) ([]string, error) {
	out, err := runner.Run(ctx, dir, "log", "--follow", "--format=%H", "--", path)
	if err != nil {
		return nil, err
	}
	var revs []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			revs = append(revs, line)
		}
	}
	return revs, nil
}
