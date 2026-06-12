package fsutil

import "strings"

const SearchIgnoreContent = "# sya issues database: use the sya CLI, not direct file reads\ntasks/\nwisps/\nevents.jsonl\n.lock\n"

var SearchIgnoreFiles = []string{".ignore", ".rgignore"}

func IsLegacySearchIgnoreContent(data []byte) bool {
	var entries []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		entries = append(entries, line)
	}
	return len(entries) == 1 && entries[0] == "*"
}
