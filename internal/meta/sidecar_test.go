package meta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadDeleteName(t *testing.T) {
	dir := t.TempDir()
	storagePath := filepath.Join(dir, "safe.mp4")
	if err := os.WriteFile(storagePath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteName(storagePath, ".meta", "⚡original.mp4"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadName(storagePath, ".meta")
	if err != nil {
		t.Fatal(err)
	}
	if got != "⚡original.mp4" {
		t.Fatalf("got %q", got)
	}
	if err := DeleteName(storagePath, ".meta"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(SidecarPath(storagePath, ".meta")); !os.IsNotExist(err) {
		t.Fatalf("expected sidecar removed, got %v", err)
	}
}
