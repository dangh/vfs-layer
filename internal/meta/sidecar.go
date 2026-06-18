package meta

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid metadata")

const TempPrefix = ".vfs-meta-"

func SidecarPath(storagePath, suffix string) string {
	return storagePath + suffix
}

func IsSidecarName(name, suffix string) bool {
	return strings.HasSuffix(name, suffix)
}

func IsImplementationTempName(name string) bool {
	return strings.HasPrefix(name, TempPrefix)
}

func ReadName(storagePath, suffix string) (string, error) {
	b, err := os.ReadFile(SidecarPath(storagePath, suffix))
	if err != nil {
		return "", err
	}
	s := string(b)
	if !utf8.ValidString(s) {
		return "", ErrInvalid
	}
	s = strings.TrimSuffix(s, "\n")
	s = strings.TrimSuffix(s, "\r")
	if !validBasename(s) {
		return "", ErrInvalid
	}
	return s, nil
}

func WriteName(storagePath, suffix, name string) error {
	if !validBasename(name) {
		return ErrInvalid
	}

	sidecar := SidecarPath(storagePath, suffix)
	dir := filepath.Dir(sidecar)
	base := filepath.Base(sidecar)
	tmp := filepath.Join(dir, fmt.Sprintf("%s%s.%d.tmp", TempPrefix, base, os.Getpid()))

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString(name + "\n")
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(tmp)
		return writeErr
	}
	if syncErr != nil {
		_ = os.Remove(tmp)
		return syncErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, sidecar); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return syncDir(dir)
}

func DeleteName(storagePath, suffix string) error {
	err := os.Remove(SidecarPath(storagePath, suffix))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func validBasename(name string) bool {
	return name != "" &&
		name != "." &&
		name != ".." &&
		!strings.Contains(name, "/") &&
		!strings.ContainsRune(name, 0)
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
