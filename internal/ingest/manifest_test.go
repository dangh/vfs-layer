package ingest

import (
	"os"
	"path/filepath"
	"testing"

	"vfs-layer/internal/config"
	"vfs-layer/internal/storage"
)

func TestRunCreatesStorageLayout(t *testing.T) {
	cfg := config.Default()
	cfg.MaxSafeBasenameBytes = 16
	cfg.HashLength = 8
	store, err := storage.New(t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "src.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	line := `{"source_path":` + quote(src) + `,"dest_path":"folder/this filename is much too long.txt","type":"file"}` + "\n"
	if err := os.WriteFile(manifest, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(store, manifest); err != nil {
		t.Fatal(err)
	}
	storageRel, err := store.Resolve("folder/this filename is much too long.txt")
	if err != nil {
		t.Fatal(err)
	}
	abs, err := store.AbsStoragePath(storageRel)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q", data)
	}
}

func quote(s string) string {
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for _, r := range s {
		if r == '\\' || r == '"' {
			b = append(b, '\\')
		}
		b = append(b, string(r)...)
	}
	b = append(b, '"')
	return string(b)
}
