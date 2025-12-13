package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// ExcludeRule represents an exclude file (~/.ctx/excludes/*.yaml)
type ExcludeRule struct {
	Name     string   `yaml:"name"`
	Patterns []string `yaml:"patterns"`
}

// LoadExcludeRule loads an exclude rule by name from ~/.ctx/excludes/
func LoadExcludeRule(name string) (ExcludeRule, error) {
	dir, err := ConfigDir()
	if err != nil {
		return ExcludeRule{}, err
	}

	data, err := os.ReadFile(filepath.Join(dir, "excludes", name+".yaml"))
	if err != nil {
		return ExcludeRule{}, err
	}

	var exc ExcludeRule
	if err := yaml.Unmarshal(data, &exc); err != nil {
		return ExcludeRule{}, err
	}

	return exc, nil
}

// SaveExcludeRule saves an exclude rule to ~/.ctx/excludes/
func SaveExcludeRule(exc ExcludeRule) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(exc)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "excludes", exc.Name+".yaml"), data, 0600)
}

// ListExcludeRules returns the names of all exclude rules in ~/.ctx/excludes/
func ListExcludeRules() ([]string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(filepath.Join(dir, "excludes"))
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			name := strings.TrimSuffix(e.Name(), ".yaml")
			names = append(names, name)
		}
	}

	return names, nil
}

// ShouldExclude checks if a path should be excluded based on the patterns
func (exc *ExcludeRule) ShouldExclude(path string) bool {
	for _, pattern := range exc.Patterns {
		// Try matching the full path
		if matched, _ := doublestar.Match(pattern, path); matched {
			return true
		}
		// Also try matching just the relative part (after any common prefix)
		// This helps with patterns like "**/node_modules/**"
		if matched, _ := doublestar.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}
	return false
}

// ExpandDirectory recursively lists all files in a directory, filtered by exclude rules
func ExpandDirectory(dir string, exclude *ExcludeRule) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories themselves, we only want files
		if d.IsDir() {
			// Check if this directory should be excluded
			if exclude != nil && exclude.ShouldExclude(path) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file should be excluded
		if exclude != nil && exclude.ShouldExclude(path) {
			return nil
		}

		files = append(files, path)
		return nil
	})

	return files, err
}
