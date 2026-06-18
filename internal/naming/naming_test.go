package naming

import "testing"

func TestSafeNames(t *testing.T) {
	n := New(120, 16, ".meta", true)
	for _, name := range []string{"movie.mp4", "Season 01", "hello-world"} {
		if !n.IsSafe(name) {
			t.Fatalf("%q should be safe", name)
		}
	}
	for _, name := range []string{"", ".", "..", "bad/name", "bad\\name", "CON", "file.meta", "trail."} {
		if n.IsSafe(name) {
			t.Fatalf("%q should be unsafe", name)
		}
	}
}

func TestStorageNamePreservesSafeExtensionForUnsafeFile(t *testing.T) {
	n := New(20, 8, ".meta", true)
	got, direct := n.StorageName("this name is too long.mp4", false)
	if direct {
		t.Fatal("expected generated storage name")
	}
	if len(got) > 20 {
		t.Fatalf("storage name too long: %q", got)
	}
	if got[len(got)-4:] != ".mp4" {
		t.Fatalf("expected .mp4 extension, got %q", got)
	}
}

func TestUnsafeDirectoryHasNoDirectorySuffix(t *testing.T) {
	n := New(10, 8, ".meta", true)
	got, direct := n.StorageName("this directory name is too long", true)
	if direct {
		t.Fatal("expected generated storage name")
	}
	if got == "" || got[len(got)-1:] == "/" {
		t.Fatalf("bad directory storage name %q", got)
	}
	if got == "this directory name is too long.dir" || got[len(got)-4:] == ".dir" {
		t.Fatalf("directory storage name must not use .dir suffix: %q", got)
	}
}
