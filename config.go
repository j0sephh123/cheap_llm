package main

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the main config file (~/.ctx/config.yaml)
type Config struct {
	ActiveContext string   `yaml:"active_context"`
	ActiveExclude string   `yaml:"active_exclude"`
	SkipPrefixes  []string `yaml:"skip_prefixes"`
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() Config {
	return Config{
		ActiveContext: "default",
		ActiveExclude: "default",
		SkipPrefixes:  []string{"work", "projects", "code", "dev", "repos"},
	}
}

// ConfigDir returns the path to ~/.ctx/
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ctx"), nil
}

// EnsureConfigDir creates ~/.ctx/ and subdirectories if they don't exist
func EnsureConfigDir() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	dirs := []string{
		dir,
		filepath.Join(dir, "contexts"),
		filepath.Join(dir, "excludes"),
		filepath.Join(dir, "history"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}

	// Create default config if it doesn't exist
	configPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := DefaultConfig()
		if err := SaveConfig(cfg); err != nil {
			return err
		}
	}

	// Create default context if it doesn't exist
	defaultCtxPath := filepath.Join(dir, "contexts", "default.yaml")
	if _, err := os.Stat(defaultCtxPath); os.IsNotExist(err) {
		ctx := Context{
			Name:           "default",
			ProjectContext: "",
			Request:        "",
			Files:          []string{},
		}
		if err := SaveContext(ctx); err != nil {
			return err
		}
	}

	// Create default exclude if it doesn't exist
	defaultExcludePath := filepath.Join(dir, "excludes", "default.yaml")
	if _, err := os.Stat(defaultExcludePath); os.IsNotExist(err) {
		exc := ExcludeRule{
			Name: "default",
			Patterns: []string{
				"**/node_modules/**",
				"**/.git/**",
				"**/.env",
				"**/.env.*",
				"**/*.env",
				"**/package-lock.json",
				"**/pnpm-lock.yaml",
				"**/yarn.lock",
			},
		}
		if err := SaveExcludeRule(exc); err != nil {
			return err
		}
	}

	return nil
}

// LoadConfig loads the config from ~/.ctx/config.yaml
func LoadConfig() (Config, error) {
	dir, err := ConfigDir()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	// Ensure skip_prefixes has defaults if empty
	if len(cfg.SkipPrefixes) == 0 {
		cfg.SkipPrefixes = DefaultConfig().SkipPrefixes
	}

	return cfg, nil
}

// SaveConfig saves the config to ~/.ctx/config.yaml
func SaveConfig(cfg Config) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0600)
}
