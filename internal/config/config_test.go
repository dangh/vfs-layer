package config

import (
	"path/filepath"
	"testing"
)

func TestLoadMissingConfigUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	def := Default()
	if cfg.StorageRoot != def.StorageRoot {
		t.Fatalf("StorageRoot = %q, want %q", cfg.StorageRoot, def.StorageRoot)
	}
	if cfg.ViewRoot != def.ViewRoot {
		t.Fatalf("ViewRoot = %q, want %q", cfg.ViewRoot, def.ViewRoot)
	}
	if cfg.MetadataSuffix != def.MetadataSuffix {
		t.Fatalf("MetadataSuffix = %q, want %q", cfg.MetadataSuffix, def.MetadataSuffix)
	}
}
