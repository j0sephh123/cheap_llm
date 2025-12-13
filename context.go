package main

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Context represents a context file (~/.ctx/contexts/*.yaml)
type Context struct {
	Name           string   `yaml:"name"`
	ProjectRoot    string   `yaml:"project_root,omitempty"` // base path to strip from file paths
	ProjectContext string   `yaml:"project_context"`
	Request        string   `yaml:"request"`
	Files          []string `yaml:"files"`
}

// LoadContext loads a context by name from ~/.ctx/contexts/
func LoadContext(name string) (Context, error) {
	dir, err := ConfigDir()
	if err != nil {
		return Context{}, err
	}

	data, err := os.ReadFile(filepath.Join(dir, "contexts", name+".yaml"))
	if err != nil {
		return Context{}, err
	}

	var ctx Context
	if err := yaml.Unmarshal(data, &ctx); err != nil {
		return Context{}, err
	}

	return ctx, nil
}

// SaveContext saves a context to ~/.ctx/contexts/
func SaveContext(ctx Context) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(ctx)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "contexts", ctx.Name+".yaml"), data, 0600)
}

// ListContexts returns the names of all contexts in ~/.ctx/contexts/
func ListContexts() ([]string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(filepath.Join(dir, "contexts"))
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

// ContextPath returns the full path to a context file
func ContextPath(name string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "contexts", name+".yaml"), nil
}

// DeleteContext removes a context file
func DeleteContext(name string) error {
	path, err := ContextPath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// AddFileToContext adds a file path to the context if not already present
// Returns true if the file was added, false if it was already present
func (ctx *Context) AddFile(path string) bool {
	// Check for duplicates
	for _, f := range ctx.Files {
		if f == path {
			return false
		}
	}
	ctx.Files = append(ctx.Files, path)
	return true
}

// RemoveFile removes a file path from the context
func (ctx *Context) RemoveFile(path string) {
	var newFiles []string
	for _, f := range ctx.Files {
		if f != path {
			newFiles = append(newFiles, f)
		}
	}
	ctx.Files = newFiles
}

// RemoveFiles removes multiple file paths from the context
func (ctx *Context) RemoveFiles(paths []string) {
	pathSet := make(map[string]bool)
	for _, p := range paths {
		pathSet[p] = true
	}

	var newFiles []string
	for _, f := range ctx.Files {
		if !pathSet[f] {
			newFiles = append(newFiles, f)
		}
	}
	ctx.Files = newFiles
}
