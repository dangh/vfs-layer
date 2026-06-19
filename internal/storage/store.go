package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"vfs-layer/internal/config"
	"vfs-layer/internal/meta"
	"vfs-layer/internal/naming"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrExists    = errors.New("already exists")
	ErrCollision = errors.New("name collision")
	ErrInvalid   = errors.New("invalid path")
)

type Store struct {
	root         string
	namer        naming.Namer
	suffix       string
	cacheEnabled bool
	cacheTTL     time.Duration
	mu           sync.Mutex
	cacheMu      sync.Mutex
	dirCache     map[string]cachedDir
}

type Entry struct {
	StorageName string
	StorageRel  string
	VirtualName string
	IsDir       bool
	Info        fs.FileInfo
}

type cachedDir struct {
	expires time.Time
	entries []Entry
	byName  map[string]Entry
}

func New(root string, cfg config.Config) (*Store, error) {
	cfg.ApplyDefaults()
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	store := &Store{
		root:         abs,
		namer:        naming.New(cfg.MaxSafeBasenameBytes, cfg.HashLength, cfg.MetadataSuffix, cfg.PreserveSafeExtensions),
		suffix:       cfg.MetadataSuffix,
		cacheEnabled: cfg.CacheEnabled,
		cacheTTL:     5 * time.Second,
		dirCache:     map[string]cachedDir{},
	}
	return store, nil
}

func (s *Store) Root() string {
	return s.root
}

func (s *Store) Namer() naming.Namer {
	return s.namer
}

func (s *Store) MetadataSuffix() string {
	return s.suffix
}

func (s *Store) EncodeName(name string, isDir bool) string {
	storageName, _ := s.namer.StorageName(name, isDir)
	return storageName
}

func (s *Store) List(viewDirRel string) ([]Entry, error) {
	storageRel, err := s.Resolve(viewDirRel)
	if err != nil {
		return nil, err
	}
	return s.ListStorage(storageRel)
}

func (s *Store) ListStorage(storageDirRel string) ([]Entry, error) {
	storageDirRel, err := cleanStorageRel(storageDirRel)
	if err != nil {
		return nil, err
	}
	if entries, ok := s.cachedEntries(storageDirRel); ok {
		return entries, nil
	}

	dirAbs, err := s.abs(storageDirRel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dirAbs)
	if err != nil {
		return nil, err
	}

	out := make([]Entry, 0, len(entries))
	for _, de := range entries {
		name := de.Name()
		if meta.IsSidecarName(name, s.suffix) || meta.IsImplementationTempName(name) {
			continue
		}
		info, err := de.Info()
		if err != nil {
			return nil, err
		}
		storageRel := joinRel(storageDirRel, name)
		storageAbs := filepath.Join(dirAbs, name)
		virtual := name
		if metaName, err := meta.ReadName(storageAbs, s.suffix); err == nil {
			virtual = metaName
		}
		out = append(out, Entry{
			StorageName: name,
			StorageRel:  storageRel,
			VirtualName: virtual,
			IsDir:       info.IsDir(),
			Info:        info,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].VirtualName < out[j].VirtualName
	})
	s.setCachedEntries(storageDirRel, out)
	return cloneEntries(out), nil
}

func (s *Store) Resolve(viewRel string) (string, error) {
	rel, err := cleanViewRel(viewRel)
	if err != nil {
		return "", err
	}
	if rel == "" {
		return "", nil
	}

	cur := ""
	for _, part := range strings.Split(rel, "/") {
		entry, err := s.EntryInStorageDir(cur, part)
		if err != nil {
			return "", err
		}
		cur = entry.StorageRel
	}
	return cur, nil
}

func (s *Store) ResolveEntry(viewRel string) (Entry, error) {
	rel, err := cleanViewRel(viewRel)
	if err != nil {
		return Entry{}, err
	}
	if rel == "" {
		info, err := os.Stat(s.root)
		if err != nil {
			return Entry{}, err
		}
		return Entry{StorageRel: "", VirtualName: "", IsDir: true, Info: info}, nil
	}
	parentView, name := path.Split(rel)
	parentStorage, err := s.Resolve(strings.TrimSuffix(parentView, "/"))
	if err != nil {
		return Entry{}, err
	}
	return s.EntryInStorageDir(parentStorage, name)
}

func (s *Store) Open(viewRel string, flags int, perm fs.FileMode) (*os.File, string, error) {
	if flags&os.O_CREATE == 0 {
		storageRel, err := s.Resolve(viewRel)
		if err != nil {
			return nil, "", err
		}
		abs, err := s.abs(storageRel)
		if err != nil {
			return nil, "", err
		}
		f, err := os.OpenFile(abs, flags, perm)
		return f, storageRel, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if storageRel, err := s.Resolve(viewRel); err == nil {
		if flags&os.O_EXCL != 0 {
			return nil, "", ErrExists
		}
		abs, err := s.abs(storageRel)
		if err != nil {
			return nil, "", err
		}
		f, err := os.OpenFile(abs, flags&^os.O_CREATE, perm)
		return f, storageRel, err
	} else if !errors.Is(err, ErrNotFound) {
		return nil, "", err
	}

	parentStorage, name, err := s.parentStorageAndName(viewRel)
	if err != nil {
		return nil, "", err
	}
	if err := s.ensureVirtualMissing(parentStorage, name); err != nil {
		return nil, "", err
	}

	storageName, direct, err := s.allocateName(parentStorage, name, false)
	if err != nil {
		return nil, "", err
	}
	storageRel := joinRel(parentStorage, storageName)
	abs, err := s.abs(storageRel)
	if err != nil {
		return nil, "", err
	}
	if !direct {
		if err := meta.WriteName(abs, s.suffix, name); err != nil {
			return nil, "", err
		}
	}
	f, err := os.OpenFile(abs, flags|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		if !direct {
			_ = meta.DeleteName(abs, s.suffix)
		}
		return nil, "", err
	}
	s.invalidateDir(parentStorage)
	return f, storageRel, nil
}

func (s *Store) Mkdir(viewRel string, perm fs.FileMode) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	parentStorage, name, err := s.parentStorageAndName(viewRel)
	if err != nil {
		return err
	}
	if err := s.ensureVirtualMissing(parentStorage, name); err != nil {
		return err
	}
	storageName, direct, err := s.allocateName(parentStorage, name, true)
	if err != nil {
		return err
	}
	storageRel := joinRel(parentStorage, storageName)
	abs, err := s.abs(storageRel)
	if err != nil {
		return err
	}
	if !direct {
		if err := meta.WriteName(abs, s.suffix, name); err != nil {
			return err
		}
	}
	if err := os.Mkdir(abs, perm); err != nil {
		if !direct {
			_ = meta.DeleteName(abs, s.suffix)
		}
		return err
	}
	s.invalidateDir(parentStorage)
	return nil
}

func (s *Store) Rename(srcViewRel, dstViewRel string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	srcEntry, err := s.ResolveEntry(srcViewRel)
	if err != nil {
		return err
	}
	srcStorageRel := srcEntry.StorageRel
	srcAbs, err := s.abs(srcStorageRel)
	if err != nil {
		return err
	}

	dstParentStorage, dstName, err := s.parentStorageAndName(dstViewRel)
	if err != nil {
		return err
	}

	var dstStorageRel string
	dstEntry, dstErr := s.EntryInStorageDir(dstParentStorage, dstName)
	dstExists := dstErr == nil
	if dstExists {
		dstStorageRel = dstEntry.StorageRel
	} else if errors.Is(dstErr, ErrNotFound) {
		dstStorageName, _, err := s.allocateName(dstParentStorage, dstName, srcEntry.IsDir)
		if err != nil {
			return err
		}
		dstStorageRel = joinRel(dstParentStorage, dstStorageName)
	} else {
		return dstErr
	}

	dstAbs, err := s.abs(dstStorageRel)
	if err != nil {
		return err
	}
	needMeta := !s.namer.IsSafe(dstName)
	if needMeta && !dstExists {
		if err := meta.WriteName(dstAbs, s.suffix, dstName); err != nil {
			return err
		}
	}

	if err := os.Rename(srcAbs, dstAbs); err != nil {
		if needMeta && !dstExists {
			_ = meta.DeleteName(dstAbs, s.suffix)
		}
		return err
	}

	if needMeta {
		if err := meta.WriteName(dstAbs, s.suffix, dstName); err != nil {
			return err
		}
	} else {
		if err := meta.DeleteName(dstAbs, s.suffix); err != nil {
			return err
		}
	}
	if srcAbs != dstAbs {
		if err := meta.DeleteName(srcAbs, s.suffix); err != nil {
			return err
		}
	}
	s.invalidateDir(parentOfStorageRel(srcStorageRel))
	s.invalidateDir(dstParentStorage)
	return nil
}

func (s *Store) Remove(viewRel string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	storageRel, err := s.Resolve(viewRel)
	if err != nil {
		return err
	}
	abs, err := s.abs(storageRel)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil {
		return err
	}
	s.invalidateDir(parentOfStorageRel(storageRel))
	return meta.DeleteName(abs, s.suffix)
}

func (s *Store) Truncate(viewRel string, size int64) error {
	storageRel, err := s.Resolve(viewRel)
	if err != nil {
		return err
	}
	abs, err := s.abs(storageRel)
	if err != nil {
		return err
	}
	if err := os.Truncate(abs, size); err != nil {
		return err
	}
	s.invalidateDir(parentOfStorageRel(storageRel))
	return nil
}

func (s *Store) Chtimes(viewRel string, atime, mtime time.Time) error {
	storageRel, err := s.Resolve(viewRel)
	if err != nil {
		return err
	}
	abs, err := s.abs(storageRel)
	if err != nil {
		return err
	}
	if err := os.Chtimes(abs, atime, mtime); err != nil {
		return err
	}
	s.invalidateDir(parentOfStorageRel(storageRel))
	return nil
}

func (s *Store) AbsStoragePath(storageRel string) (string, error) {
	return s.abs(storageRel)
}

func (s *Store) ensureVirtualMissing(parentStorage, name string) error {
	if _, err := s.EntryInStorageDir(parentStorage, name); err == nil {
		return ErrExists
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

func (s *Store) EntryInStorageDir(parentStorage, virtualName string) (Entry, error) {
	parentStorage, err := cleanStorageRel(parentStorage)
	if err != nil {
		return Entry{}, err
	}
	if entry, ok := s.cachedEntry(parentStorage, virtualName); ok {
		return entry, nil
	}
	if entry, err := s.directEntryInStorageDir(parentStorage, virtualName); err == nil {
		return entry, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Entry{}, err
	}

	entries, err := s.ListStorage(parentStorage)
	if err != nil {
		return Entry{}, err
	}
	for _, e := range entries {
		if e.VirtualName == virtualName {
			return e, nil
		}
	}
	return Entry{}, ErrNotFound
}

func (s *Store) cachedEntries(storageDirRel string) ([]Entry, bool) {
	if !s.cacheEnabled {
		return nil, false
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	cached, ok := s.dirCache[storageDirRel]
	if !ok || time.Now().After(cached.expires) {
		if ok {
			delete(s.dirCache, storageDirRel)
		}
		return nil, false
	}
	return cloneEntries(cached.entries), true
}

func (s *Store) cachedEntry(storageDirRel, virtualName string) (Entry, bool) {
	if !s.cacheEnabled {
		return Entry{}, false
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	cached, ok := s.dirCache[storageDirRel]
	if !ok || time.Now().After(cached.expires) {
		if ok {
			delete(s.dirCache, storageDirRel)
		}
		return Entry{}, false
	}
	entry, ok := cached.byName[virtualName]
	return entry, ok
}

func (s *Store) setCachedEntries(storageDirRel string, entries []Entry) {
	if !s.cacheEnabled {
		return
	}
	byName := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		byName[entry.VirtualName] = entry
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.dirCache[storageDirRel] = cachedDir{
		expires: time.Now().Add(s.cacheTTL),
		entries: cloneEntries(entries),
		byName:  byName,
	}
}

func (s *Store) invalidateDir(storageDirRel string) {
	if !s.cacheEnabled {
		return
	}
	storageDirRel, err := cleanStorageRel(storageDirRel)
	if err != nil {
		return
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	delete(s.dirCache, storageDirRel)
}

func cloneEntries(entries []Entry) []Entry {
	out := make([]Entry, len(entries))
	copy(out, entries)
	return out
}

func (s *Store) directEntryInStorageDir(parentStorage, virtualName string) (Entry, error) {
	if !s.namer.IsSafe(virtualName) {
		return Entry{}, ErrNotFound
	}
	parentAbs, err := s.abs(parentStorage)
	if err != nil {
		return Entry{}, err
	}
	storageAbs := filepath.Join(parentAbs, virtualName)
	info, err := os.Lstat(storageAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return Entry{}, ErrNotFound
		}
		return Entry{}, err
	}
	if _, err := os.Lstat(meta.SidecarPath(storageAbs, s.suffix)); err == nil {
		return Entry{}, ErrNotFound
	} else if !os.IsNotExist(err) {
		return Entry{}, err
	}
	return Entry{
		StorageName: virtualName,
		StorageRel:  joinRel(parentStorage, virtualName),
		VirtualName: virtualName,
		IsDir:       info.IsDir(),
		Info:        info,
	}, nil
}

func (s *Store) parentStorageAndName(viewRel string) (string, string, error) {
	rel, err := cleanViewRel(viewRel)
	if err != nil {
		return "", "", err
	}
	if rel == "" {
		return "", "", ErrInvalid
	}
	parentView, name := path.Split(rel)
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
		return "", "", ErrInvalid
	}
	parentStorage, err := s.Resolve(strings.TrimSuffix(parentView, "/"))
	if err != nil {
		return "", "", err
	}
	return parentStorage, name, nil
}

func (s *Store) allocateName(parentStorage, virtualName string, isDir bool) (string, bool, error) {
	base, direct := s.namer.StorageName(virtualName, isDir)
	if direct {
		if err := s.storageNameAvailable(parentStorage, base); err != nil {
			return "", false, err
		}
		return base, true, nil
	}

	stem, ext := splitGeneratedName(base, isDir)
	for i := 0; i < 10_000; i++ {
		candidate := base
		if i > 0 {
			candidate = stem + "-" + strconv.Itoa(i+1) + ext
		}
		if len([]byte(candidate)) > s.namer.MaxBytes {
			return "", false, fmt.Errorf("%w: generated name too long", ErrCollision)
		}
		if err := s.storageNameAvailable(parentStorage, candidate); err == nil {
			return candidate, false, nil
		} else if !errors.Is(err, ErrExists) {
			return "", false, err
		}
	}
	return "", false, fmt.Errorf("%w: exhausted suffixes", ErrCollision)
}

func splitGeneratedName(name string, isDir bool) (string, string) {
	if isDir {
		return name, ""
	}
	ext := filepath.Ext(name)
	if ext == "" {
		return name, ""
	}
	return strings.TrimSuffix(name, ext), ext
}

func (s *Store) storageNameAvailable(parentStorage, storageName string) error {
	parentAbs, err := s.abs(parentStorage)
	if err != nil {
		return err
	}
	abs := filepath.Join(parentAbs, storageName)
	if _, err := os.Lstat(abs); err == nil {
		return ErrExists
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Lstat(meta.SidecarPath(abs, s.suffix)); err == nil {
		return ErrExists
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) abs(rel string) (string, error) {
	clean, err := cleanStorageRel(rel)
	if err != nil {
		return "", err
	}
	p := filepath.Join(s.root, filepath.FromSlash(clean))
	p = filepath.Clean(p)
	if p != s.root && !strings.HasPrefix(p, s.root+string(os.PathSeparator)) {
		return "", ErrInvalid
	}
	return p, nil
}

func cleanViewRel(rel string) (string, error) {
	rel = filepath.ToSlash(rel)
	rel = path.Clean("/" + rel)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." {
		return "", nil
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." || strings.ContainsRune(part, 0) {
			return "", ErrInvalid
		}
	}
	return rel, nil
}

func cleanStorageRel(rel string) (string, error) {
	rel = filepath.ToSlash(rel)
	rel = path.Clean("/" + rel)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." {
		return "", nil
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." || strings.Contains(part, "/") || strings.ContainsRune(part, 0) {
			return "", ErrInvalid
		}
	}
	return rel, nil
}

func joinRel(parent, name string) string {
	if parent == "" {
		return name
	}
	return path.Join(parent, name)
}

func parentOfStorageRel(storageRel string) string {
	parent, _ := path.Split(storageRel)
	return strings.TrimSuffix(parent, "/")
}
