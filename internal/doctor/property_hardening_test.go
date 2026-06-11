package doctor

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDoctorMutationInvariantsProperty(t *testing.T) {
	t.Parallel()

	for seed := 0; seed < 500; seed++ {
		valid := mutateGeneratedProjectValid(generatedProject(seed), seed)
		sch, idx := loadProject(t, valid, ".")
		report, err := Run(valid, ".", sch, idx, Options{})
		if err != nil {
			t.Fatalf("seed %d valid mutation Run: %v", seed, err)
		}
		if len(report.Findings) != 0 {
			t.Fatalf("seed %d valid mutation findings = %#v", seed, report.Findings)
		}

		invalid := mutateGeneratedProjectInvalid(generatedProject(seed), seed)
		sch, idx = loadProject(t, invalid, ".")
		report, err = Run(invalid, ".", sch, idx, Options{})
		if err != nil {
			t.Fatalf("seed %d invalid mutation Run: %v", seed, err)
		}
		if len(report.Findings) == 0 {
			t.Fatalf("seed %d invalid mutation produced no findings", seed)
		}
	}
}

func mutateGeneratedProjectValid(fsys fstest.MapFS, seed int) fstest.MapFS {
	out := cloneMapFS(fsys)
	target := fmt.Sprintf("tasks/t%05x-generated.md", seed*16)
	file := out[target]
	if file == nil {
		return out
	}
	data := string(file.Data)
	statuses := []string{"todo", "doing", "done"}
	data = replaceLine(data, "status:", "status: "+statuses[seed%len(statuses)])
	data = replaceLine(data, "  ready:", fmt.Sprintf("  ready: %t", seed%2 == 1))
	file.Data = []byte(data)
	out[target] = file
	return out
}

func mutateGeneratedProjectInvalid(fsys fstest.MapFS, seed int) fstest.MapFS {
	out := cloneMapFS(fsys)
	if seed%2 == 0 {
		delete(out, fmt.Sprintf("tasks/t%05x-generated.md", seed*16))
		return out
	}
	target := fmt.Sprintf("tasks/t%05x-generated.md", seed*16)
	file := out[target]
	if file == nil {
		return out
	}
	file.Data = []byte(replaceLine(string(file.Data), "status:", "status: impossible"))
	out[target] = file
	return out
}

func cloneMapFS(fsys fstest.MapFS) fstest.MapFS {
	out := make(fstest.MapFS, len(fsys))
	for name, file := range fsys {
		copied := *file
		copied.Data = append([]byte(nil), file.Data...)
		out[name] = &copied
	}
	return out
}

func replaceLine(data, prefix, replacement string) string {
	lines := strings.Split(data, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimLeft(line, " "), strings.TrimSpace(prefix)) {
			lines[i] = replacement
			break
		}
	}
	return strings.Join(lines, "\n")
}
