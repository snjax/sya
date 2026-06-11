package events

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAppendReadConcurrent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	const writers = 64
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := ResultOK
			if i%2 == 0 {
				result = ResultDenied
			}
			err := Append(root, Event{
				TS:     time.Date(2026, 1, 2, 3, 4, i%60, 0, time.UTC),
				Actor:  "test",
				Task:   "t00001",
				From:   "todo",
				To:     "done",
				Result: result,
			})
			if err != nil {
				t.Errorf("append: %v", err)
			}
		}()
	}
	wg.Wait()

	data, err := os.ReadFile(Path(root))
	if err != nil {
		t.Fatalf("read raw journal: %v", err)
	}
	if lines := strings.Count(strings.TrimSpace(string(data)), "\n") + 1; lines != writers {
		t.Fatalf("lines=%d want=%d\n%s", lines, writers, data)
	}

	all, err := Read(root, Filters{})
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if len(all) != writers {
		t.Fatalf("events=%d want=%d", len(all), writers)
	}
	denied, err := Read(root, Filters{DeniedOnly: true, Task: "t00001", Limit: 5})
	if err != nil {
		t.Fatalf("read denied: %v", err)
	}
	if len(denied) != 5 {
		t.Fatalf("denied limit=%d want=5", len(denied))
	}
	for _, event := range denied {
		if event.Result != ResultDenied || event.Task != "t00001" {
			t.Fatalf("unexpected filtered event: %#v", event)
		}
	}
}

func TestReadMissingJournal(t *testing.T) {
	t.Parallel()

	events, err := Read(t.TempDir(), Filters{})
	if err != nil {
		t.Fatalf("read missing: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events=%#v", events)
	}
}
