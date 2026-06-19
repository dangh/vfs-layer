package storage

import (
	"os"
	"path/filepath"
	"testing"

	"vfs-layer/internal/config"
	"vfs-layer/internal/meta"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	cfg := config.Default()
	cfg.MaxSafeBasenameBytes = 16
	cfg.HashLength = 8
	store, err := New(t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestCreateUnsafeFileCreatesSidecarAndViewName(t *testing.T) {
	store := newTestStore(t)
	name := "this filename is much too long.mp4"
	f, storageRel, err := store.Open(name, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("hello"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if storageRel == name {
		t.Fatalf("expected generated storage name, got %q", storageRel)
	}
	abs, err := store.AbsStoragePath(storageRel)
	if err != nil {
		t.Fatal(err)
	}
	got, err := meta.ReadName(abs, store.MetadataSuffix())
	if err != nil {
		t.Fatal(err)
	}
	if got != name {
		t.Fatalf("sidecar got %q", got)
	}
	entries, err := store.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].VirtualName != name {
		t.Fatalf("bad listing: %#v", entries)
	}
}

func TestRenameUnsafeToSafeRemovesSidecar(t *testing.T) {
	store := newTestStore(t)
	f, oldRel, err := store.Open("this filename is much too long.mp4", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	oldAbs, _ := store.AbsStoragePath(oldRel)
	if _, err := os.Stat(meta.SidecarPath(oldAbs, store.MetadataSuffix())); err != nil {
		t.Fatal(err)
	}
	if err := store.Rename("this filename is much too long.mp4", "safe.mp4"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(meta.SidecarPath(oldAbs, store.MetadataSuffix())); !os.IsNotExist(err) {
		t.Fatalf("old sidecar still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), "safe.mp4.meta")); !os.IsNotExist(err) {
		t.Fatalf("safe file should not have sidecar: %v", err)
	}
	if _, err := store.Resolve("safe.mp4"); err != nil {
		t.Fatal(err)
	}
}

func TestUnsafeDirectoryUsesPlainSafeName(t *testing.T) {
	store := newTestStore(t)
	name := "this directory name is too long"
	if err := store.Mkdir(name, 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(store.Root())
	if err != nil {
		t.Fatal(err)
	}
	var dirName string
	for _, entry := range entries {
		if entry.IsDir() {
			dirName = entry.Name()
		}
	}
	if dirName == "" {
		t.Fatal("storage directory not created")
	}
	if filepath.Ext(dirName) == ".dir" {
		t.Fatalf("directory used .dir suffix: %q", dirName)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), dirName+".meta")); err != nil {
		t.Fatal(err)
	}
}

func TestListStorageCacheInvalidatesAfterMkdir(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.ListStorage(""); err != nil {
		t.Fatal(err)
	}
	if err := store.Mkdir("safe-dir", 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := store.ListStorage("")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].VirtualName != "safe-dir" {
		t.Fatalf("cache was not invalidated after mkdir: %#v", entries)
	}
}
