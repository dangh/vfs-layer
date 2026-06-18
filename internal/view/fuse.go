package view

import (
	"context"
	"errors"
	"os"
	"path"
	"syscall"

	gofs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"vfs-layer/internal/storage"
)

type Node struct {
	gofs.Inode
	store   *storage.Store
	viewRel string
}

func Mount(mountpoint string, store *storage.Store, debug bool) (*fuse.Server, error) {
	root := &Node{store: store}
	return gofs.Mount(mountpoint, root, &gofs.Options{
		MountOptions: fuse.MountOptions{
			Debug: debug,
			Name:  "vfs-layer",
		},
	})
}

var _ = (gofs.NodeLookuper)((*Node)(nil))

func (n *Node) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*gofs.Inode, syscall.Errno) {
	childRel := joinView(n.viewRel, name)
	entry, err := n.store.ResolveEntry(childRel)
	if err != nil {
		return nil, errno(err)
	}
	abs, err := n.store.AbsStoragePath(entry.StorageRel)
	if err != nil {
		return nil, errno(err)
	}
	st, err := lstat(abs)
	if err != nil {
		return nil, errno(err)
	}
	out.Attr.FromStat(st)
	return n.NewInode(ctx, &Node{store: n.store, viewRel: childRel}, stableFromStat(st)), 0
}

var _ = (gofs.NodeReaddirer)((*Node)(nil))

func (n *Node) Readdir(ctx context.Context) (gofs.DirStream, syscall.Errno) {
	entries, err := n.store.List(n.viewRel)
	if err != nil {
		return nil, errno(err)
	}
	dirEntries := make([]fuse.DirEntry, 0, len(entries))
	for _, entry := range entries {
		mode := modeFromInfo(entry.Info)
		dirEntries = append(dirEntries, fuse.DirEntry{
			Name: entry.VirtualName,
			Mode: mode,
			Ino:  inodeFromRel(entry.StorageRel),
		})
	}
	return gofs.NewListDirStream(dirEntries), 0
}

var _ = (gofs.NodeOpener)((*Node)(nil))

func (n *Node) Open(ctx context.Context, flags uint32) (gofs.FileHandle, uint32, syscall.Errno) {
	f, _, err := n.store.Open(n.viewRel, int(flags), 0)
	if err != nil {
		return nil, 0, errno(err)
	}
	fh, err := loopbackHandle(f)
	if err != nil {
		return nil, 0, errno(err)
	}
	return fh, 0, 0
}

var _ = (gofs.NodeCreater)((*Node)(nil))

func (n *Node) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*gofs.Inode, gofs.FileHandle, uint32, syscall.Errno) {
	childRel := joinView(n.viewRel, name)
	f, storageRel, err := n.store.Open(childRel, int(flags)|os.O_CREATE, os.FileMode(mode))
	if err != nil {
		return nil, nil, 0, errno(err)
	}
	fh, err := loopbackHandle(f)
	if err != nil {
		return nil, nil, 0, errno(err)
	}
	abs, err := n.store.AbsStoragePath(storageRel)
	if err != nil {
		return nil, nil, 0, errno(err)
	}
	st, err := lstat(abs)
	if err != nil {
		return nil, nil, 0, errno(err)
	}
	out.Attr.FromStat(st)
	return n.NewInode(ctx, &Node{store: n.store, viewRel: childRel}, stableFromStat(st)), fh, 0, 0
}

var _ = (gofs.NodeMkdirer)((*Node)(nil))

func (n *Node) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*gofs.Inode, syscall.Errno) {
	childRel := joinView(n.viewRel, name)
	if err := n.store.Mkdir(childRel, os.FileMode(mode)); err != nil {
		return nil, errno(err)
	}
	entry, err := n.store.ResolveEntry(childRel)
	if err != nil {
		return nil, errno(err)
	}
	abs, err := n.store.AbsStoragePath(entry.StorageRel)
	if err != nil {
		return nil, errno(err)
	}
	st, err := lstat(abs)
	if err != nil {
		return nil, errno(err)
	}
	out.Attr.FromStat(st)
	return n.NewInode(ctx, &Node{store: n.store, viewRel: childRel}, stableFromStat(st)), 0
}

var _ = (gofs.NodeRenamer)((*Node)(nil))

func (n *Node) Rename(ctx context.Context, name string, newParent gofs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	if flags != 0 {
		return syscall.EINVAL
	}
	parent, ok := newParent.(*Node)
	if !ok {
		return syscall.EXDEV
	}
	return errno(n.store.Rename(joinView(n.viewRel, name), joinView(parent.viewRel, newName)))
}

var _ = (gofs.NodeUnlinker)((*Node)(nil))

func (n *Node) Unlink(ctx context.Context, name string) syscall.Errno {
	return errno(n.store.Remove(joinView(n.viewRel, name)))
}

var _ = (gofs.NodeRmdirer)((*Node)(nil))

func (n *Node) Rmdir(ctx context.Context, name string) syscall.Errno {
	return errno(n.store.Remove(joinView(n.viewRel, name)))
}

var _ = (gofs.NodeGetattrer)((*Node)(nil))

func (n *Node) Getattr(ctx context.Context, f gofs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	if f != nil {
		if getter, ok := f.(gofs.FileGetattrer); ok {
			return getter.Getattr(ctx, out)
		}
	}
	storageRel, err := n.store.Resolve(n.viewRel)
	if err != nil {
		return errno(err)
	}
	abs, err := n.store.AbsStoragePath(storageRel)
	if err != nil {
		return errno(err)
	}
	st, err := lstat(abs)
	if err != nil {
		return errno(err)
	}
	out.FromStat(st)
	return 0
}

var _ = (gofs.NodeSetattrer)((*Node)(nil))

func (n *Node) Setattr(ctx context.Context, f gofs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	if f != nil {
		if setter, ok := f.(gofs.FileSetattrer); ok {
			if errno := setter.Setattr(ctx, in, out); errno != 0 {
				return errno
			}
			return 0
		}
	}

	storageRel, err := n.store.Resolve(n.viewRel)
	if err != nil {
		return errno(err)
	}
	abs, err := n.store.AbsStoragePath(storageRel)
	if err != nil {
		return errno(err)
	}

	if mode, ok := in.GetMode(); ok {
		if err := syscall.Chmod(abs, mode); err != nil {
			return errno(err)
		}
	}
	uid, uidOK := in.GetUID()
	gid, gidOK := in.GetGID()
	if uidOK || gidOK {
		suid := -1
		sgid := -1
		if uidOK {
			suid = int(uid)
		}
		if gidOK {
			sgid = int(gid)
		}
		if err := syscall.Chown(abs, suid, sgid); err != nil {
			return errno(err)
		}
	}
	mtime, mOK := in.GetMTime()
	atime, aOK := in.GetATime()
	if mOK || aOK {
		ap := &atime
		mp := &mtime
		if !aOK {
			ap = nil
		}
		if !mOK {
			mp = nil
		}
		ts := [2]syscall.Timespec{
			fuse.UtimeToTimespec(ap),
			fuse.UtimeToTimespec(mp),
		}
		if err := syscall.UtimesNano(abs, ts[:]); err != nil {
			return errno(err)
		}
	}
	if size, ok := in.GetSize(); ok {
		if err := syscall.Truncate(abs, int64(size)); err != nil {
			return errno(err)
		}
	}
	return n.Getattr(ctx, f, out)
}

var _ = (gofs.NodeStatfser)((*Node)(nil))

func (n *Node) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	stat := syscall.Statfs_t{}
	if err := syscall.Statfs(n.store.Root(), &stat); err != nil {
		return errno(err)
	}
	out.FromStatfsT(&stat)
	return 0
}

func loopbackHandle(f *os.File) (gofs.FileHandle, error) {
	fd, err := syscall.Dup(int(f.Fd()))
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		_ = syscall.Close(fd)
		return nil, closeErr
	}
	return gofs.NewLoopbackFile(fd), nil
}

func joinView(parent, name string) string {
	if parent == "" {
		return name
	}
	return path.Join(parent, name)
}

func errno(err error) syscall.Errno {
	if err == nil {
		return 0
	}
	if errors.Is(err, storage.ErrNotFound) || os.IsNotExist(err) {
		return syscall.ENOENT
	}
	if errors.Is(err, storage.ErrExists) || os.IsExist(err) {
		return syscall.EEXIST
	}
	if errors.Is(err, storage.ErrInvalid) {
		return syscall.EINVAL
	}
	if errors.Is(err, storage.ErrCollision) {
		return syscall.EEXIST
	}
	return gofs.ToErrno(err)
}
