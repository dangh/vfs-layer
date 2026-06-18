package ingest

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"vfs-layer/internal/storage"
)

type Entry struct {
	SourcePath string `json:"source_path"`
	DestPath   string `json:"dest_path"`
	Type       string `json:"type"`
	Size       int64  `json:"size,omitempty"`
	ModTime    string `json:"mod_time,omitempty"`
}

func Run(store *storage.Store, manifestPath string) error {
	f, err := os.Open(manifestPath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return fmt.Errorf("%s:%d: %w", manifestPath, lineNo, err)
		}
		if err := apply(store, entry); err != nil {
			return fmt.Errorf("%s:%d: %w", manifestPath, lineNo, err)
		}
	}
	return scanner.Err()
}

func apply(store *storage.Store, entry Entry) error {
	if entry.DestPath == "" {
		return errors.New("dest_path is required")
	}
	switch entry.Type {
	case "dir", "directory":
		if err := ensureParents(store, entry.DestPath); err != nil {
			return err
		}
		if err := store.Mkdir(entry.DestPath, 0o755); err != nil && !errors.Is(err, storage.ErrExists) {
			return err
		}
		return setModTime(store, entry.DestPath, entry.ModTime)
	case "file", "":
		if err := ensureParents(store, entry.DestPath); err != nil {
			return err
		}
		f, _, err := store.Open(entry.DestPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if entry.SourcePath != "" {
			src, err := os.Open(entry.SourcePath)
			if err != nil {
				_ = f.Close()
				return err
			}
			_, copyErr := io.Copy(f, src)
			closeSrcErr := src.Close()
			if copyErr != nil {
				_ = f.Close()
				return copyErr
			}
			if closeSrcErr != nil {
				_ = f.Close()
				return closeSrcErr
			}
		} else if entry.Size > 0 {
			if err := f.Truncate(entry.Size); err != nil {
				_ = f.Close()
				return err
			}
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		return setModTime(store, entry.DestPath, entry.ModTime)
	default:
		return fmt.Errorf("unsupported entry type %q", entry.Type)
	}
}

func ensureParents(store *storage.Store, destPath string) error {
	dir := path.Dir(path.Clean("/" + destPath))
	if dir == "/" || dir == "." {
		return nil
	}
	cur := ""
	for _, part := range strings.Split(strings.TrimPrefix(dir, "/"), "/") {
		if part == "" {
			continue
		}
		if cur == "" {
			cur = part
		} else {
			cur = path.Join(cur, part)
		}
		if err := store.Mkdir(cur, 0o755); err != nil && !errors.Is(err, storage.ErrExists) {
			return err
		}
	}
	return nil
}

func setModTime(store *storage.Store, destPath, value string) error {
	if value == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return err
	}
	return store.Chtimes(destPath, t, t)
}
