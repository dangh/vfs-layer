package view

import (
	"hash/fnv"
	"io/fs"
	"syscall"

	gofs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"vfs-layer/internal/storage"
)

func lstat(path string) (*syscall.Stat_t, error) {
	st := &syscall.Stat_t{}
	if err := syscall.Lstat(path, st); err != nil {
		return nil, err
	}
	return st, nil
}

func stableFromStat(st *syscall.Stat_t) gofs.StableAttr {
	return gofs.StableAttr{
		Mode: uint32(st.Mode),
		Ino:  uint64(st.Ino),
		Gen:  1,
	}
}

func stableFromEntry(entry storage.Entry) gofs.StableAttr {
	return gofs.StableAttr{
		Mode: modeFromInfo(entry.Info),
		Ino:  inodeFromRel(entry.StorageRel),
		Gen:  1,
	}
}

func fillEntryAttr(attr *fuse.Attr, entry storage.Entry) {
	if st, ok := entry.Info.Sys().(*syscall.Stat_t); ok && st != nil {
		attr.FromStat(st)
		attr.Ino = inodeFromRel(entry.StorageRel)
		return
	}
	attr.Ino = inodeFromRel(entry.StorageRel)
	attr.Mode = modeFromInfo(entry.Info)
	attr.Size = uint64(entry.Info.Size())
	mod := entry.Info.ModTime()
	attr.SetTimes(&mod, &mod, &mod)
}

func modeFromInfo(info fs.FileInfo) uint32 {
	perm := uint32(info.Mode().Perm())
	switch {
	case info.IsDir():
		return fuse.S_IFDIR | perm
	case info.Mode().Type()&fs.ModeSymlink != 0:
		return syscall.S_IFLNK | perm
	default:
		return fuse.S_IFREG | perm
	}
}

func inodeFromRel(rel string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(rel))
	v := h.Sum64()
	if v == 0 {
		return 1
	}
	return v
}
