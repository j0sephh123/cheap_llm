package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const maxHistoryEntries = 100

// HistoryEntry represents a saved prompt in history
type HistoryEntry struct {
	Timestamp      time.Time `yaml:"timestamp"`
	ContextName    string    `yaml:"context_name"`
	ProjectContext string    `yaml:"project_context"`
	Request        string    `yaml:"request"`
	Files          []string  `yaml:"files"`
}

// HistoryDir returns the path to ~/.ctx/history/
func HistoryDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history"), nil
}

// EnsureHistoryDir creates ~/.ctx/history/ if it doesn't exist
func EnsureHistoryDir() error {
	dir, err := HistoryDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

// SaveHistoryEntry saves a new history entry and prunes old entries if needed
func SaveHistoryEntry(entry HistoryEntry) error {
	if err := EnsureHistoryDir(); err != nil {
		return err
	}

	dir, err := HistoryDir()
	if err != nil {
		return err
	}

	// Generate filename: 2025-01-15_14-30-45_contextname.yaml
	filename := entry.Timestamp.Format("2006-01-02_15-04-05") + "_" + sanitizeFilename(entry.ContextName) + ".yaml"

	data, err := yaml.Marshal(entry)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, filename), data, 0600); err != nil {
		return err
	}

	// Prune old entries
	return PruneHistory()
}

// ListHistoryEntries returns all history entries sorted by timestamp (newest first)
func ListHistoryEntries() ([]HistoryEntry, error) {
	dir, err := HistoryDir()
	if err != nil {
		return nil, err
	}

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []HistoryEntry{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var historyEntries []HistoryEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		entry, err := LoadHistoryEntry(e.Name())
		if err != nil {
			continue // Skip malformed entries
		}
		historyEntries = append(historyEntries, entry)
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(historyEntries, func(i, j int) bool {
		return historyEntries[i].Timestamp.After(historyEntries[j].Timestamp)
	})

	return historyEntries, nil
}

// LoadHistoryEntry loads a history entry by filename
func LoadHistoryEntry(filename string) (HistoryEntry, error) {
	dir, err := HistoryDir()
	if err != nil {
		return HistoryEntry{}, err
	}

	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return HistoryEntry{}, err
	}

	var entry HistoryEntry
	if err := yaml.Unmarshal(data, &entry); err != nil {
		return HistoryEntry{}, err
	}

	return entry, nil
}

// PruneHistory removes oldest entries if there are more than maxHistoryEntries
func PruneHistory() error {
	dir, err := HistoryDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	// Filter to only yaml files
	var yamlFiles []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			yamlFiles = append(yamlFiles, e)
		}
	}

	if len(yamlFiles) <= maxHistoryEntries {
		return nil
	}

	// Sort by name (which includes timestamp, so oldest first)
	sort.Slice(yamlFiles, func(i, j int) bool {
		return yamlFiles[i].Name() < yamlFiles[j].Name()
	})

	// Delete oldest entries
	toDelete := len(yamlFiles) - maxHistoryEntries
	for i := 0; i < toDelete; i++ {
		os.Remove(filepath.Join(dir, yamlFiles[i].Name()))
	}

	return nil
}

// HistoryEntryFilename returns the filename for a history entry
func HistoryEntryFilename(entry HistoryEntry) string {
	return entry.Timestamp.Format("2006-01-02_15-04-05") + "_" + sanitizeFilename(entry.ContextName) + ".yaml"
}

// sanitizeFilename removes/replaces characters that aren't safe for filenames
func sanitizeFilename(name string) string {
	// Replace unsafe characters with underscore
	unsafe := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", " "}
	result := name
	for _, char := range unsafe {
		result = strings.ReplaceAll(result, char, "_")
	}
	return result
}

// RequestPreview returns a truncated preview of the request (first line, max 50 chars)
func (e HistoryEntry) RequestPreview() string {
	if e.Request == "" {
		return "(no request)"
	}

	// Get first line
	lines := strings.Split(e.Request, "\n")
	preview := strings.TrimSpace(lines[0])

	if len(preview) > 50 {
		preview = preview[:47] + "..."
	}

	if preview == "" {
		return "(empty)"
	}

	return preview
}

// FormatTimestamp returns a human-readable timestamp
func (e HistoryEntry) FormatTimestamp() string {
	return e.Timestamp.Format("2006-01-02 15:04")
}
