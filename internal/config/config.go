package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

const appName = "logdog"

type Paths struct {
	ConfigDir  string
	ConfigFile string
	DataDir    string
}

type Config struct {
	Roots              []string `json:"roots"`
	IgnorePatterns     []string `json:"ignore_patterns"`
	Extensions         []string `json:"extensions"`
	MaxFileBytes       int64    `json:"max_file_bytes"`
	MaxPreviewLines    int      `json:"max_preview_lines"`
	CleanupKeepDays    int      `json:"cleanup_keep_days"`
	DefaultLevelFilter string   `json:"default_level_filter"`
}

func ResolvePaths() Paths {
	configBase, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		configBase = filepath.Join(home, ".config")
	}

	dataBase := os.Getenv("LOGDOG_DIR")
	if dataBase == "" {
		home, _ := os.UserHomeDir()
		dataBase = filepath.Join(home, ".local", "share", appName)
	}

	configDir := filepath.Join(configBase, appName)
	return Paths{
		ConfigDir:  configDir,
		ConfigFile: filepath.Join(configDir, "logdog-config.json"),
		DataDir:    dataBase,
	}
}

func DefaultConfig() Config {
	paths := ResolvePaths()
	return Config{
		Roots:           []string{paths.DataDir},
		IgnorePatterns:  []string{".git", "node_modules", "vendor", "dist", "build", ".next", "coverage"},
		Extensions:      []string{".log", ".txt", ".out", ".json", ".jsonl", ".ndjson"},
		MaxFileBytes:    50 * 1024 * 1024,
		MaxPreviewLines: 5000,
		CleanupKeepDays: 14,
	}
}

func Load() (Config, error) {
	cfg := DefaultConfig()
	path := ResolvePaths().ConfigFile

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), err
	}
	cfg.normalize()
	return cfg, nil
}

func Ensure() (Config, error) {
	cfg, err := Load()
	if err != nil {
		return cfg, err
	}
	if err := Save(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func Save(cfg Config) error {
	cfg.normalize()
	paths := ResolvePaths()
	if err := os.MkdirAll(paths.ConfigDir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(paths.ConfigFile, data, 0o644)
}

func ProjectLogDir(projectPath string) string {
	projectName := filepath.Base(projectPath)
	return filepath.Join(ResolvePaths().DataDir, projectName, "logs")
}

func (c *Config) normalize() {
	if c.IgnorePatterns == nil {
		c.IgnorePatterns = slices.Clone(DefaultConfig().IgnorePatterns)
	}
	if c.Extensions == nil {
		c.Extensions = slices.Clone(DefaultConfig().Extensions)
	}
	if c.MaxFileBytes <= 0 {
		c.MaxFileBytes = DefaultConfig().MaxFileBytes
	}
	if c.MaxPreviewLines <= 0 {
		c.MaxPreviewLines = DefaultConfig().MaxPreviewLines
	}
	if c.CleanupKeepDays < 0 {
		c.CleanupKeepDays = DefaultConfig().CleanupKeepDays
	}

	c.Roots = cleanPaths(c.Roots)
	c.IgnorePatterns = cleanStrings(c.IgnorePatterns)
	c.Extensions = cleanStrings(c.Extensions)
}

func cleanPaths(values []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, value := range values {
		if value == "" {
			continue
		}
		expanded := expandHome(value)
		abs, err := filepath.Abs(expanded)
		if err != nil {
			abs = expanded
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}

func cleanStrings(values []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func expandHome(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if len(path) > 2 && path[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
