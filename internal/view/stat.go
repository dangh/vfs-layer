package view

import (
	"hash/fnv"
	"io/fs"
	"syscall"

	gofs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
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
