package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/snjax/sya/internal/events"
	"github.com/snjax/sya/internal/syaerr"
)

func TestEventsCommandRecordsOKAndDeniedTransitions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Blocked")
	createSeedTask(t, root, "b00001", "Dependency")
	mustRun(t, root, nil, []string{"link", "a00001", "depends_on", "b00001"})

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"move", "a00001", "in_progress"})
	if code != syaerr.ExitTransitionRejected {
		t.Fatalf("denied move code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	mustRun(t, root, nil, []string{"move", "b00001", "in_progress"})

	read, err := events.Read(root, events.Filters{})
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if len(read) != 2 {
		t.Fatalf("events=%#v", read)
	}
	if read[0].Task != "a00001" || read[0].Result != events.ResultDenied || read[0].ErrorType != "transition_blocked" {
		t.Fatalf("unexpected denied event: %#v", read[0])
	}
	if read[1].Task != "b00001" || read[1].Result != events.ResultOK {
		t.Fatalf("unexpected ok event: %#v", read[1])
	}

	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"--json", "events", "--denied", "--task", "a00001", "--limit", "1"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("events command code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"task":"a00001"`) || !strings.Contains(stdout, `"result":"denied"`) {
		t.Fatalf("events output missing denied event: %s", stdout)
	}
}

func TestDeniedEventErrorTypes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Blocked")
	createSeedTask(t, root, "b00001", "Dependency")
	mustRun(t, root, nil, []string{"link", "a00001", "depends_on", "b00001"})
	_, _, blockedCode := runCLI(t, root, nil, nil, []string{"move", "a00001", "in_progress"})
	if blockedCode != syaerr.ExitTransitionRejected {
		t.Fatalf("blocked move code=%d", blockedCode)
	}
	_, _, notAllowedCode := runCLI(t, root, nil, nil, []string{"move", "a00001", "working"})
	if notAllowedCode != syaerr.ExitTransitionRejected {
		t.Fatalf("not allowed move code=%d", notAllowedCode)
	}
	read, err := events.Read(root, events.Filters{DeniedOnly: true, Task: "a00001"})
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if len(read) != 2 {
		t.Fatalf("events=%#v", read)
	}
	if read[0].ErrorType != "transition_blocked" {
		t.Fatalf("blocked error_type=%q", read[0].ErrorType)
	}
	if read[1].ErrorType != "transition_not_allowed" {
		t.Fatalf("not allowed error_type=%q", read[1].ErrorType)
	}
}

func TestEventsCommandDiscoversProjectWithoutLoadingIndex(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	if err := events.Append(root, events.Event{
		TS:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Actor:  "codex",
		Task:   "a00001",
		From:   "todo",
		To:     "in_progress",
		Result: events.ResultOK,
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sya", "schema.yml"), []byte("not: [valid"), 0o644); err != nil {
		t.Fatalf("corrupt schema: %v", err)
	}

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"--json", "events"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("events code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"task":"a00001"`) {
		t.Fatalf("events output missing event: %s", stdout)
	}
}

func TestEventAppendErrorWarnsButDoesNotFail(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "b00001", "Dependency")
	var stdout, stderr bytes.Buffer
	app := New(Options{
		Version: "test",
		Stdout:  &stdout,
		Stderr:  &stderr,
		WorkDir: root,
		Env: func(string) string {
			return ""
		},
		GitUser: func(context.Context) (string, error) {
			return "", errors.New("git user unset")
		},
		Now: func() time.Time {
			return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		},
		AppendEvent: func(string, events.Event) error {
			return errors.New("event store unavailable")
		},
	})

	code := app.Execute([]string{"move", "b00001", "in_progress"})
	if code != syaerr.ExitOK {
		t.Fatalf("move code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "warning: could not append event: event store unavailable") {
		t.Fatalf("stderr missing warning: %q", stderr.String())
	}
}

func TestDeniedTransitionAlertHookReceivesJSON(t *testing.T) {
	before := alertTempFiles(t)
	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Blocked")
	createSeedTask(t, root, "b00001", "Dependency")
	mustRun(t, root, nil, []string{"link", "a00001", "depends_on", "b00001"})
	out := filepath.Join(root, "alert.json")
	script := writeAlertScript(t, root)
	appendConfig(t, root, "alerts:\n  denied_transition: "+strconv.Quote(shellQuote(script)+" "+shellQuote(out))+"\n")

	_, _, code := runCLI(t, root, nil, nil, []string{"move", "a00001", "in_progress"})
	if code != syaerr.ExitTransitionRejected {
		t.Fatalf("denied move code=%d", code)
	}
	data := waitForFile(t, out, 2*time.Second)
	var event events.Event
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("alert json: %v\n%s", err, data)
	}
	if event.Task != "a00001" || event.Result != events.ResultDenied || event.ErrorType != "transition_blocked" {
		t.Fatalf("unexpected alert event: %#v", event)
	}
	waitForNoNewAlertTempFiles(t, before, 2*time.Second)
}

func TestAlertHookFailureIgnored(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Blocked")
	createSeedTask(t, root, "b00001", "Dependency")
	mustRun(t, root, nil, []string{"link", "a00001", "depends_on", "b00001"})
	appendConfig(t, root, "alerts:\n  denied_transition: \"exit 42\"\n")

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"move", "a00001", "in_progress"})
	if code != syaerr.ExitTransitionRejected || !strings.Contains(stderr, "transition blocked") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestAlertHookDoesNotBlockOnHangingCommand(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Blocked")
	createSeedTask(t, root, "b00001", "Dependency")
	mustRun(t, root, nil, []string{"link", "a00001", "depends_on", "b00001"})
	script := filepath.Join(root, "hang.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	appendConfig(t, root, "alerts:\n  denied_transition: "+strconv.Quote(shellQuote(script))+"\n")

	start := time.Now()
	_, _, code := runCLI(t, root, nil, nil, []string{"move", "a00001", "in_progress"})
	elapsed := time.Since(start)
	if code != syaerr.ExitTransitionRejected {
		t.Fatalf("code=%d", code)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("hanging alert blocked for %s", elapsed)
	}
}

func TestDoctorViolationAlertHookReceivesJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Invalid")
	path := filepath.Join(root, ".sya", "tasks", "a00001-invalid.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read task: %v", err)
	}
	data = []byte(strings.Replace(string(data), "status: todo", "status: ghost", 1))
	data = []byte(strings.Replace(string(data), "## Description\nTODO\n\n", "", 1))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("corrupt task: %v", err)
	}
	out := filepath.Join(root, "doctor-alert.json")
	script := writeAlertScript(t, root)
	appendConfig(t, root, "alerts:\n  doctor_violation: "+strconv.Quote(shellQuote(script)+" "+shellQuote(out))+"\n")

	_, _, code := runCLI(t, root, nil, nil, []string{"doctor"})
	if code != syaerr.ExitSchemaInvalid {
		t.Fatalf("doctor code=%d", code)
	}
	alert := waitForFile(t, out, 2*time.Second)
	if !strings.Contains(string(alert), `"kind":"task_status_unknown"`) {
		t.Fatalf("doctor alert missing finding: %s", alert)
	}
}

func TestInitAndDoctorEventsGitignore(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), ".sya/events.jsonl") ||
		!strings.Contains(string(gitignore), ".sya/wisps/") ||
		!strings.Contains(string(gitignore), ".sya/.lock") {
		t.Fatalf(".gitignore missing runtime entries:\n%s", gitignore)
	}

	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".sya/wisps/\n"), 0o644); err != nil {
		t.Fatalf("rewrite .gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sya", "events.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write events: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sya", ".lock"), nil, 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"doctor"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("doctor code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "events_not_ignored") {
		t.Fatalf("doctor missing events_not_ignored warning:\n%s", stdout)
	}
	if !strings.Contains(stdout, "lock_not_ignored") {
		t.Fatalf("doctor missing lock_not_ignored warning:\n%s", stdout)
	}
}

func writeAlertScript(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "alert.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncat > \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("write alert script: %v", err)
	}
	return path
}

func appendConfig(t *testing.T, root, text string) {
	t.Helper()
	path := filepath.Join(root, ".sya", "config.yml")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open config: %v", err)
	}
	defer file.Close()
	if _, err := fmt.Fprint(file, text); err != nil {
		t.Fatalf("append config: %v", err)
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) []byte {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return data
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func alertTempFiles(t *testing.T) map[string]bool {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "sya-alert-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool, len(matches))
	for _, match := range matches {
		out[match] = true
	}
	return out
}

func waitForNoNewAlertTempFiles(t *testing.T, before map[string]bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		matches, err := filepath.Glob(filepath.Join(os.TempDir(), "sya-alert-*.json"))
		if err != nil {
			t.Fatal(err)
		}
		var leaked []string
		for _, match := range matches {
			if !before[match] {
				leaked = append(leaked, match)
			}
		}
		if len(leaked) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("alert temp files leaked: %v", leaked)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
