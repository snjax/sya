package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/snjax/sya/internal/syaerr"
)

func TestActorResolver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		flag    string
		env     map[string]string
		gitUser string
		gitErr  error
		want    string
	}{
		{
			name:    "flag wins",
			flag:    "flag actor",
			env:     map[string]string{"SYA_ACTOR": "env actor"},
			gitUser: "git actor",
			want:    "flag actor",
		},
		{
			name:    "env wins over git",
			env:     map[string]string{"SYA_ACTOR": "env actor"},
			gitUser: "git actor",
			want:    "env actor",
		},
		{
			name:    "git config fallback",
			gitUser: "git actor",
			want:    "git actor",
		},
		{
			name:   "unknown when nothing set",
			gitErr: errors.New("git config missing"),
			want:   "unknown",
		},
		{
			name:    "blank values are skipped",
			flag:    "  ",
			env:     map[string]string{"SYA_ACTOR": "  "},
			gitUser: "  ",
			want:    "unknown",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := ActorResolver{
				Env: func(key string) string {
					return tt.env[key]
				},
				GitUser: func(context.Context) (string, error) {
					return tt.gitUser, tt.gitErr
				},
			}
			if got := resolver.Resolve(context.Background(), tt.flag); got != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindProject(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".sya"), 0o755); err != nil {
		t.Fatalf("mkdir .sya: %v", err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	for _, cwd := range []string{root, nested} {
		cwd := cwd
		t.Run(cwd, func(t *testing.T) {
			t.Parallel()

			project, err := FindProject(cwd)
			if err != nil {
				t.Fatalf("FindProject() error = %v", err)
			}
			if project.Root != root {
				t.Fatalf("Root = %q, want %q", project.Root, root)
			}
			if project.SyaDir != filepath.Join(root, ".sya") {
				t.Fatalf("SyaDir = %q, want %q", project.SyaDir, filepath.Join(root, ".sya"))
			}
		})
	}
}

func TestFindProjectMissing(t *testing.T) {
	t.Parallel()

	_, err := FindProject(t.TempDir())
	var notFound syaerr.NotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("FindProject() error = %T %v, want syaerr.NotFound", err, err)
	}
	if notFound.ID != ".sya" {
		t.Fatalf("NotFound.ID = %q, want .sya", notFound.ID)
	}
}

func TestPrimeOutsideProjectPrintsNothing(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"prime"}, {"--json", "prime"}} {
		args := args
		t.Run(args[0], func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			app := New(Options{
				Version: "test",
				Stdout:  &stdout,
				Stderr:  &stderr,
				WorkDir: t.TempDir(),
				Env: func(string) string {
					return ""
				},
			})
			if code := app.Execute(args); code != syaerr.ExitOK {
				t.Fatalf("exit code = %d, want %d", code, syaerr.ExitOK)
			}
			if stdout.String() != "" {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if stderr.String() != "" {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestJSONOutputUsesStdoutOnly(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	app := New(Options{
		Version: "test",
		Stdout:  &stdout,
		Stderr:  &stderr,
		Env: func(string) string {
			return ""
		},
	})
	if code := app.Execute([]string{"--json", "version"}); code != syaerr.ExitOK {
		t.Fatalf("exit code = %d, want %d", code, syaerr.ExitOK)
	}
	if got, want := stdout.String(), "{\"ok\":true,\"data\":{\"version\":\"test\"}}\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
