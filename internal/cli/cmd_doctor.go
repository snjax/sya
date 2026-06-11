package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/snjax/sya/internal/doctor"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts doctorOptions
		cmd := app.command("doctor", "Validate sya project state", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runDoctor(opts)
		})
		cmd.Flags().BoolVar(&opts.Strict, "strict", false, "enable strict checks")
		cmd.Flags().BoolVar(&opts.FixMerge, "fix-merge", false, "repair Log-only conflict markers")
		cmd.Flags().StringVar(&opts.ReassignID, "reassign-id", "", "reassign duplicate task id")
		return cmd
	})
}

type doctorOptions struct {
	Strict     bool
	FixMerge   bool
	ReassignID string
}

type DoctorResult struct {
	Findings []doctor.Finding `json:"findings"`
	Changes  []doctor.Change  `json:"changes,omitempty"`
}

func (r DoctorResult) ResultExitCode() int {
	for _, finding := range r.Findings {
		if finding.Severity == doctor.SeverityError {
			return syaerr.ExitSchemaInvalid
		}
	}
	return syaerr.ExitOK
}

func (r DoctorResult) HumanText(Colorizer) string {
	var b strings.Builder
	if len(r.Findings) == 0 {
		b.WriteString("doctor: clean")
	} else {
		fmt.Fprintf(&b, "%-8s %-26s %-8s %-34s %s\n", "severity", "kind", "task", "path", "message")
		for _, finding := range r.Findings {
			fixable := ""
			if finding.Fixable {
				fixable = " [fixable]"
			}
			fmt.Fprintf(&b, "%-8s %-26s %-8s %-34s %s%s\n",
				finding.Severity,
				finding.Kind,
				finding.TaskID,
				finding.Path,
				finding.Message,
				fixable,
			)
		}
	}
	if len(r.Changes) > 0 {
		b.WriteString("\nchanges:\n")
		for _, change := range r.Changes {
			fmt.Fprintf(&b, "  %s %s", change.Action, change.Path)
			if change.From != "" || change.To != "" {
				fmt.Fprintf(&b, " %s -> %s", change.From, change.To)
			}
			if change.Message != "" {
				fmt.Fprintf(&b, " (%s)", change.Message)
			}
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (a *App) runDoctor(opts doctorOptions) (DoctorResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return DoctorResult{}, err
	}
	var changes []doctor.Change
	if opts.FixMerge {
		report, err := doctor.Run(os.DirFS(state.Project.Root), ".sya", state.Schema, state.Index, doctor.Options{Strict: opts.Strict})
		if err != nil {
			return DoctorResult{}, err
		}
		for _, finding := range report.Findings {
			if finding.Kind != "conflict_markers" || !finding.Fixable || finding.Path == "" {
				continue
			}
			fixed, err := doctor.FixMerge(filepath.Join(state.Project.Root, filepath.FromSlash(finding.Path)))
			if err != nil {
				continue
			}
			changes = append(changes, fixed...)
		}
		state, err = a.loadProject()
		if err != nil {
			return DoctorResult{}, err
		}
	}
	if opts.ReassignID != "" {
		fixed, err := doctor.ReassignIDInDir(state.Project.Root, state.Index, opts.ReassignID)
		if err != nil {
			return DoctorResult{}, err
		}
		changes = append(changes, fixed...)
		state, err = a.loadProject()
		if err != nil {
			return DoctorResult{}, err
		}
	}
	report, err := doctor.Run(os.DirFS(state.Project.Root), ".sya", state.Schema, state.Index, doctor.Options{Strict: opts.Strict})
	if err != nil {
		return DoctorResult{}, err
	}
	a.fireDoctorViolationAlerts(state.Project, report.Findings)
	return DoctorResult{Findings: report.Findings, Changes: changes}, nil
}
