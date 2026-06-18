package check

import (
	"os"
	"path/filepath"
	"testing"

	"vfs-layer/internal/config"
	"vfs-layer/internal/meta"
	"vfs-layer/internal/storage"
)

func TestScanReportsOrphanSidecar(t *testing.T) {
	cfg := config.Default()
	store, err := storage.New(t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	orphanBase := filepath.Join(store.Root(), "missing")
	if err := os.WriteFile(meta.SidecarPath(orphanBase, store.MetadataSuffix()), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	issues, err := Scan(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Severity != SeverityWarning {
		t.Fatalf("unexpected issues: %#v", issues)
	}
}
