package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Completion represents a single directory completion suggestion.
type Completion struct {
	Name     string // display name (e.g. "my-app/")
	FullPath string // absolute path
}

// ExpandPath expands ~ to the user's home directory.
func ExpandPath(input string) string {
	if strings.HasPrefix(input, "~/") || input == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return input
		}
		return filepath.Join(home, input[1:])
	}
	return input
}

// ContractPath replaces the home directory prefix with ~.
func ContractPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}

// ListCompletions returns directory entries matching the current input.
// If input ends with "/", lists children of that directory.
// Otherwise, splits into parent + prefix and filters children by prefix.
// Hidden directories (starting with ".") are only shown when the prefix starts with ".".
func ListCompletions(input string) ([]Completion, error) {
	expanded := ExpandPath(input)

	var parentDir, prefix string
	if strings.HasSuffix(expanded, "/") {
		parentDir = expanded
		prefix = ""
	} else {
		parentDir = filepath.Dir(expanded)
		prefix = filepath.Base(expanded)
	}

	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return nil, err
	}

	showHidden := strings.HasPrefix(prefix, ".")

	var completions []Completion
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden dirs unless prefix starts with "."
		if strings.HasPrefix(name, ".") && !showHidden {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}
		fullPath := filepath.Join(parentDir, name)
		completions = append(completions, Completion{
			Name:     name + "/",
			FullPath: fullPath,
		})
	}

	sort.Slice(completions, func(i, j int) bool {
		return completions[i].Name < completions[j].Name
	})

	return completions, nil
}

// CommonPrefix returns the longest common prefix among a list of strings.
func CommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}
	prefix := strs[0]
	for _, s := range strs[1:] {
		for !strings.HasPrefix(s, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}
