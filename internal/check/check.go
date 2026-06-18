package check

import (
	"fmt"
	"os"
	"path/filepath"

	"vfs-layer/internal/meta"
	"vfs-layer/internal/storage"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type Issue struct {
	Severity Severity
	Path     string
	Message  string
}

func Scan(store *storage.Store) ([]Issue, error) {
	var issues []Issue
	err := scanDir(store, "", &issues)
	return issues, err
}

func HasErrors(issues []Issue) bool {
	for _, issue := range issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}

func RemoveOrphanSidecars(store *storage.Store) ([]string, error) {
	var removed []string
	err := filepath.WalkDir(store.Root(), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !meta.IsSidecarName(d.Name(), store.MetadataSuffix()) {
			return nil
		}
		base := p[:len(p)-len(store.MetadataSuffix())]
		if _, err := os.Lstat(base); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.Remove(p); err != nil {
			return err
		}
		removed = append(removed, p)
		return nil
	})
	return removed, err
}

func scanDir(store *storage.Store, storageRel string, issues *[]Issue) error {
	dirAbs, err := store.AbsStoragePath(storageRel)
	if err != nil {
		return err
	}
	dirEntries, err := os.ReadDir(dirAbs)
	if err != nil {
		return err
	}

	virtualNames := map[string]string{}
	for _, de := range dirEntries {
		name := de.Name()
		full := filepath.Join(dirAbs, name)
		displayPath := full

		if meta.IsImplementationTempName(name) {
			continue
		}
		if meta.IsSidecarName(name, store.MetadataSuffix()) {
			base := full[:len(full)-len(store.MetadataSuffix())]
			if _, err := os.Lstat(base); os.IsNotExist(err) {
				*issues = append(*issues, Issue{
					Severity: SeverityWarning,
					Path:     displayPath,
					Message:  "orphan sidecar metadata",
				})
			} else if err != nil {
				return err
			}
			continue
		}

		virtual := name
		if metaName, err := meta.ReadName(full, store.MetadataSuffix()); err == nil {
			virtual = metaName
		} else if err != nil && !os.IsNotExist(err) {
			*issues = append(*issues, Issue{
				Severity: SeverityError,
				Path:     meta.SidecarPath(full, store.MetadataSuffix()),
				Message:  fmt.Sprintf("invalid sidecar metadata: %v", err),
			})
		}

		if previous, ok := virtualNames[virtual]; ok {
			*issues = append(*issues, Issue{
				Severity: SeverityError,
				Path:     displayPath,
				Message:  fmt.Sprintf("virtual name collision with %s", previous),
			})
		} else {
			virtualNames[virtual] = displayPath
		}

		if de.IsDir() {
			childRel := name
			if storageRel != "" {
				childRel = filepath.ToSlash(filepath.Join(storageRel, name))
			}
			if err := scanDir(store, childRel, issues); err != nil {
				return err
			}
		}
	}
	return nil
}
