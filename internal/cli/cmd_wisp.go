package cli

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/snjax/sya/internal/fsutil"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		cmd := &cobra.Command{Use: "wisp", Short: "Manage freeform wisps", SilenceUsage: true}
		cmd.AddCommand(app.wispCreateCommand())
		cmd.AddCommand(app.wispListCommand())
		cmd.AddCommand(app.wispShowCommand())
		cmd.AddCommand(app.wispSquashCommand())
		cmd.AddCommand(app.wispBurnCommand())
		return cmd
	})
}

type wisp struct {
	ID      string    `json:"id" yaml:"id"`
	Title   string    `json:"title" yaml:"title"`
	Created time.Time `json:"created" yaml:"created"`
	Body    []byte    `json:"-" yaml:"-"`
	File    string    `json:"file" yaml:"-"`
}

type WispResult struct {
	ID      string `json:"id"`
	Title   string `json:"title,omitempty"`
	File    string `json:"file,omitempty"`
	Created string `json:"created,omitempty"`
	Body    string `json:"body,omitempty"`
	Burned  bool   `json:"burned,omitempty"`
	TaskID  string `json:"task_id,omitempty"`
	Show    bool   `json:"-"`
}

type WispResults struct {
	Wisps []WispResult `json:"wisps"`
}

func (r WispResult) HumanText(Colorizer) string {
	switch {
	case r.Body != "":
		return strings.TrimRight(r.Body, "\n")
	case r.Show:
		return ""
	case r.TaskID != "":
		return fmt.Sprintf("%s: squashed into %s", r.ID, r.TaskID)
	case r.Burned:
		return fmt.Sprintf("%s: burned", r.ID)
	case r.File != "":
		return fmt.Sprintf("created %s %s", r.ID, r.File)
	default:
		return r.ID
	}
}

func (r WispResults) HumanText(Colorizer) string {
	lines := make([]string, 0, len(r.Wisps))
	for _, w := range r.Wisps {
		lines = append(lines, fmt.Sprintf("%s %-20s %s", w.ID, w.Created, w.Title))
	}
	return strings.Join(lines, "\n")
}

func (a *App) wispCreateCommand() *cobra.Command {
	var description string
	var file string
	cmd := a.command("create \"Title\"", "Create a freeform wisp", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		return a.withProjectMutationLock(func() (any, error) {
			return a.runWispCreate(args[0], description, file)
		})
	})
	cmd.Flags().StringVarP(&description, "description", "d", "", "wisp markdown body")
	cmd.Flags().StringVar(&file, "file", "", "wisp body file or -")
	return cmd
}

func (a *App) wispListCommand() *cobra.Command {
	return a.command("list", "List wisps", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		project, err := a.DiscoverProject()
		if err != nil {
			return nil, err
		}
		wisps, err := loadWisps(project)
		if err != nil {
			return nil, err
		}
		results := WispResults{Wisps: make([]WispResult, 0, len(wisps))}
		for _, w := range wisps {
			results.Wisps = append(results.Wisps, wispSummary(w))
		}
		return results, nil
	})
}

func (a *App) wispShowCommand() *cobra.Command {
	return a.command("show <id>", "Show a wisp", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		project, err := a.DiscoverProject()
		if err != nil {
			return nil, err
		}
		w, err := resolveWisp(project, args[0])
		if err != nil {
			return nil, err
		}
		result := wispSummary(w)
		result.Body = string(w.Body)
		result.Show = true
		return result, nil
	})
}

func (a *App) wispSquashCommand() *cobra.Command {
	var typ string
	cmd := a.command("squash <id>", "Create a real task from a wisp and burn it", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		return a.withProjectMutationLock(func() (any, error) {
			return a.runWispSquash(args[0], typ)
		})
	})
	cmd.Flags().StringVarP(&typ, "type", "t", "", "task type")
	return cmd
}

func (a *App) wispBurnCommand() *cobra.Command {
	return a.command("burn <id>", "Delete a wisp", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
		return a.withProjectMutationLock(func() (any, error) {
			project, err := a.DiscoverProject()
			if err != nil {
				return nil, err
			}
			w, err := resolveWisp(project, args[0])
			if err != nil {
				return nil, err
			}
			if err := os.Remove(filepath.Join(project.Root, filepath.FromSlash(w.File))); err != nil {
				return nil, err
			}
			result := wispSummary(w)
			result.Burned = true
			return result, nil
		})
	})
}

func (a *App) runWispCreate(title, description, file string) (WispResult, error) {
	if strings.TrimSpace(title) == "" {
		return WispResult{}, syaerr.Usage{Message: "title is required"}
	}
	if description != "" && file != "" {
		return WispResult{}, syaerr.Usage{Message: "--description and --file are mutually exclusive"}
	}
	project, err := a.DiscoverProject()
	if err != nil {
		return WispResult{}, err
	}
	body := []byte(description)
	if file != "" {
		body, err = readInputFile(file, a.in)
		if err != nil {
			return WispResult{}, err
		}
	}
	wisps, err := loadWisps(project)
	if err != nil {
		return WispResult{}, err
	}
	existing := make(map[string]struct{}, len(wisps))
	for _, w := range wisps {
		existing[strings.TrimPrefix(w.ID, "w-")] = struct{}{}
	}
	rawID, err := a.newID(existing, configuredIDLength(project))
	if err != nil {
		return WispResult{}, err
	}
	id := "w-" + rawID
	name := id
	if slug := slugify(title); slug != "" {
		name += "-" + slug
	}
	w := wisp{
		ID:      id,
		Title:   title,
		Created: a.now().UTC(),
		Body:    body,
		File:    filepath.ToSlash(filepath.Join(".sya", "wisps", name+".md")),
	}
	if err := writeWisp(project.Root, w); err != nil {
		return WispResult{}, err
	}
	return wispSummary(w), nil
}

func (a *App) runWispSquash(id, typ string) (WispResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return WispResult{}, err
	}
	if typ == "" {
		typ = defaultTaskType(state.Schema)
	}
	w, err := resolveWisp(state.Project, id)
	if err != nil {
		return WispResult{}, err
	}
	created, err := a.createOne(state, batchCreateSpec{
		Title:       w.Title,
		Type:        typ,
		Priority:    "normal",
		Description: strings.TrimRight(string(w.Body), "\n"),
	})
	if err != nil {
		return WispResult{}, err
	}
	if err := os.Remove(filepath.Join(state.Project.Root, filepath.FromSlash(w.File))); err != nil {
		return WispResult{}, err
	}
	result := wispSummary(w)
	result.Burned = true
	result.TaskID = created.ID
	return result, nil
}

func wispSummary(w wisp) WispResult {
	created := ""
	if !w.Created.IsZero() {
		created = w.Created.UTC().Format(time.RFC3339)
	}
	return WispResult{ID: w.ID, Title: w.Title, File: w.File, Created: created}
}

func loadWisps(project Project) ([]wisp, error) {
	dir := filepath.Join(project.Root, ".sya", "wisps")
	var wisps []wisp
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return fs.SkipDir
			}
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		w, err := parseWispFile(project.Root, path)
		if err != nil {
			return err
		}
		wisps = append(wisps, w)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Slice(wisps, func(i, j int) bool {
		if !wisps[i].Created.Equal(wisps[j].Created) {
			return wisps[i].Created.Before(wisps[j].Created)
		}
		return wisps[i].ID < wisps[j].ID
	})
	return wisps, nil
}

func resolveWisp(project Project, idOrPrefix string) (wisp, error) {
	wisps, err := loadWisps(project)
	if err != nil {
		return wisp{}, err
	}
	var matches []wisp
	for _, w := range wisps {
		if w.ID == idOrPrefix {
			return w, nil
		}
		if strings.HasPrefix(w.ID, idOrPrefix) {
			matches = append(matches, w)
		}
	}
	switch len(matches) {
	case 0:
		return wisp{}, syaerr.NotFound{ID: idOrPrefix}
	case 1:
		return matches[0], nil
	default:
		candidates := make([]syaerr.Candidate, 0, len(matches))
		for _, w := range matches {
			candidates = append(candidates, syaerr.Candidate{ID: w.ID, Title: w.Title, Type: "wisp", File: w.File})
		}
		return wisp{}, syaerr.Ambiguous{Prefix: idOrPrefix, Candidates: candidates}
	}
}

func parseWispFile(root, path string) (wisp, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return wisp{}, err
	}
	yml, body, err := splitWispFrontmatter(data)
	if err != nil {
		return wisp{}, syaerr.SchemaInvalid{Message: filepath.ToSlash(path) + ": " + err.Error()}
	}
	var fm struct {
		ID      string    `yaml:"id"`
		Title   string    `yaml:"title"`
		Created time.Time `yaml:"created"`
	}
	if err := yaml.UnmarshalWithOptions(yml, &fm, yaml.DisallowUnknownField()); err != nil {
		return wisp{}, syaerr.SchemaInvalid{Message: filepath.ToSlash(path) + ": " + err.Error()}
	}
	if fm.ID == "" || fm.Title == "" {
		return wisp{}, syaerr.SchemaInvalid{Message: filepath.ToSlash(path) + ": missing required wisp id or title"}
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return wisp{}, err
	}
	return wisp{ID: fm.ID, Title: fm.Title, Created: fm.Created, Body: body, File: filepath.ToSlash(rel)}, nil
}

func writeWisp(root string, w wisp) error {
	var buf bytes.Buffer
	buf.WriteString("---\n")
	if err := appendWispYAML(&buf, "id", w.ID); err != nil {
		return err
	}
	if err := appendWispYAML(&buf, "title", w.Title); err != nil {
		return err
	}
	if err := appendWispYAML(&buf, "created", w.Created.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	buf.WriteString("---\n")
	buf.Write(w.Body)
	return fsutil.AtomicWriteFile(filepath.Join(root, filepath.FromSlash(w.File)), buf.Bytes(), 0o644)
}

func appendWispYAML(buf *bytes.Buffer, key string, value any) error {
	data, err := yaml.Marshal(map[string]any{key: value})
	if err != nil {
		return err
	}
	buf.Write(data)
	return nil
}

func splitWispFrontmatter(data []byte) ([]byte, []byte, error) {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, nil, fmt.Errorf("missing opening frontmatter marker")
	}
	end := bytes.Index(data[4:], []byte("\n---"))
	if end < 0 {
		return nil, nil, fmt.Errorf("missing closing frontmatter marker")
	}
	marker := 4 + end
	bodyStart := marker + len("\n---")
	if bodyStart < len(data) && data[bodyStart] == '\n' {
		bodyStart++
	}
	return data[4:marker], data[bodyStart:], nil
}
