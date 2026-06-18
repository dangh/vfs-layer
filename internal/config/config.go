package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	StorageRoot                    string
	ViewRoot                       string
	MaxSafeBasenameBytes           int
	HashLength                     int
	MetadataSuffix                 string
	PreserveSafeExtensions         bool
	CaseInsensitiveCollisionChecks bool
	CacheEnabled                   bool
	WatcherEnabled                 bool
}

func Default() Config {
	return Config{
		StorageRoot:                    "/storage",
		ViewRoot:                       "/view",
		MaxSafeBasenameBytes:           120,
		HashLength:                     16,
		MetadataSuffix:                 ".meta",
		PreserveSafeExtensions:         true,
		CaseInsensitiveCollisionChecks: true,
		CacheEnabled:                   true,
		WatcherEnabled:                 true,
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return cfg, fmt.Errorf("%s:%d: expected key: value", path, lineNo)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if err := apply(&cfg, key, value); err != nil {
			return cfg, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, err
	}
	cfg.ApplyDefaults()
	return cfg, nil
}

func (c *Config) ApplyDefaults() {
	def := Default()
	if c.StorageRoot == "" {
		c.StorageRoot = def.StorageRoot
	}
	if c.ViewRoot == "" {
		c.ViewRoot = def.ViewRoot
	}
	if c.MaxSafeBasenameBytes <= 0 {
		c.MaxSafeBasenameBytes = def.MaxSafeBasenameBytes
	}
	if c.HashLength <= 0 {
		c.HashLength = def.HashLength
	}
	if c.MetadataSuffix == "" {
		c.MetadataSuffix = def.MetadataSuffix
	}
}

func apply(c *Config, key, value string) error {
	switch key {
	case "storage_root":
		c.StorageRoot = value
	case "view_root":
		c.ViewRoot = value
	case "max_safe_basename_bytes":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s must be an integer", key)
		}
		c.MaxSafeBasenameBytes = v
	case "hash_length":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s must be an integer", key)
		}
		c.HashLength = v
	case "metadata_suffix":
		c.MetadataSuffix = value
	case "preserve_safe_extensions":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s must be a boolean", key)
		}
		c.PreserveSafeExtensions = v
	case "case_insensitive_collision_checks":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s must be a boolean", key)
		}
		c.CaseInsensitiveCollisionChecks = v
	case "cache_enabled":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s must be a boolean", key)
		}
		c.CacheEnabled = v
	case "watcher_enabled":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s must be a boolean", key)
		}
		c.WatcherEnabled = v
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}
