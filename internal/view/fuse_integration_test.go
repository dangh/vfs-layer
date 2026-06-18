//go:build fuse && linux

package view

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vfs-layer/internal/config"
	"vfs-layer/internal/meta"
	"vfs-layer/internal/storage"
)

func TestRealFuseSidecarWorkflow(t *testing.T) {
	cfg := config.Default()
	cfg.MaxSafeBasenameBytes = 32
	cfg.HashLength = 12

	root := t.TempDir()
	storageRoot := filepath.Join(root, "storage")
	viewRoot := filepath.Join(root, "view")
	if err := os.MkdirAll(storageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(viewRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	store, err := storage.New(storageRoot, cfg)
	if err != nil {
		t.Fatal(err)
	}

	server, err := Mount(viewRoot, store, false)
	if err != nil {
		t.Fatalf("mount real FUSE filesystem: %v", err)
	}
	t.Cleanup(func() {
		server.Unmount()
		server.Wait()
	})

	longName := "this filename is intentionally far beyond the configured safe storage limit.mp4"
	viewPath := filepath.Join(viewRoot, longName)
	if err := os.WriteFile(viewPath, []byte("hello through fuse"), 0o644); err != nil {
		t.Fatalf("write through FUSE view: %v", err)
	}

	viewData, err := os.ReadFile(viewPath)
	if err != nil {
		t.Fatalf("read through FUSE view: %v", err)
	}
	if string(viewData) != "hello through fuse" {
		t.Fatalf("unexpected view data %q", viewData)
	}

	storageEntries, err := os.ReadDir(storageRoot)
	if err != nil {
		t.Fatal(err)
	}
	var dataName, metaName string
	for _, entry := range storageEntries {
		switch {
		case strings.HasSuffix(entry.Name(), cfg.MetadataSuffix):
			metaName = entry.Name()
		default:
			dataName = entry.Name()
		}
	}
	if dataName == "" || dataName == longName {
		t.Fatalf("expected generated safe storage name, got %q", dataName)
	}
	if metaName != dataName+cfg.MetadataSuffix {
		t.Fatalf("expected sidecar %q, got %q", dataName+cfg.MetadataSuffix, metaName)
	}

	metaContents, err := os.ReadFile(filepath.Join(storageRoot, metaName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(metaContents)) != longName {
		t.Fatalf("metadata got %q", metaContents)
	}

	viewEntries, err := os.ReadDir(viewRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(viewEntries) != 1 || viewEntries[0].Name() != longName {
		t.Fatalf("unexpected view entries: %#v", viewEntries)
	}
	if _, err := os.Stat(filepath.Join(viewRoot, metaName)); !os.IsNotExist(err) {
		t.Fatalf("metadata should be hidden from view, stat err=%v", err)
	}

	safeName := "safe.mp4"
	if err := os.Rename(viewPath, filepath.Join(viewRoot, safeName)); err != nil {
		t.Fatalf("rename through FUSE view: %v", err)
	}
	if _, err := os.Stat(filepath.Join(storageRoot, safeName)); err != nil {
		t.Fatalf("safe storage file missing after rename: %v", err)
	}
	if _, err := os.Stat(meta.SidecarPath(filepath.Join(storageRoot, safeName), cfg.MetadataSuffix)); !os.IsNotExist(err) {
		t.Fatalf("safe rename should remove sidecar, stat err=%v", err)
	}
}
