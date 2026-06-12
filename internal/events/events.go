package events

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/snjax/sya/internal/syaerr"
)

const (
	ResultOK     = "ok"
	ResultDenied = "denied"
)

type Event struct {
	TS         time.Time          `json:"ts"`
	Actor      string             `json:"actor,omitempty"`
	Task       string             `json:"task,omitempty"`
	From       string             `json:"from,omitempty"`
	To         string             `json:"to,omitempty"`
	Result     string             `json:"result"`
	ErrorType  string             `json:"error_type,omitempty"`
	Attest     []Attestation      `json:"attest,omitempty"`
	Violations []syaerr.Violation `json:"violations,omitempty"`
}

type Attestation struct {
	ID     string `json:"id"`
	Answer string `json:"answer"`
}

type Filters struct {
	DeniedOnly bool
	Task       string
	Since      time.Time
	Limit      int
}

func Append(projectRoot string, event Event) error {
	if event.TS.IsZero() {
		event.TS = time.Now().UTC()
	}
	if event.Result == "" {
		event.Result = ResultOK
	}
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	path := Path(projectRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	// O_APPEND plus one Write keeps each small JSONL event line intact across
	// concurrent writers on local filesystems.
	_, err = file.Write(line)
	return err
}

func Read(projectRoot string, filters Filters) ([]Event, error) {
	file, err := os.Open(Path(projectRoot))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readJSONL(file, filters)
}

func Path(projectRoot string) string {
	return filepath.Join(projectRoot, ".sya", "events.jsonl")
}

func readJSONL(r io.Reader, filters Filters) ([]Event, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var out []Event
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		if !matches(event, filters) {
			continue
		}
		out = append(out, event)
		if filters.Limit > 0 && len(out) > filters.Limit {
			copy(out, out[len(out)-filters.Limit:])
			out = out[:filters.Limit]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func matches(event Event, filters Filters) bool {
	if filters.DeniedOnly && event.Result != ResultDenied {
		return false
	}
	if filters.Task != "" && event.Task != filters.Task {
		return false
	}
	if !filters.Since.IsZero() && event.TS.Before(filters.Since) {
		return false
	}
	return true
}
