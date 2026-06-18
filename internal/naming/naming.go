package naming

import (
	"crypto/sha256"
	"encoding/base32"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type Namer struct {
	MaxBytes              int
	HashLength            int
	MetadataSuffix        string
	PreserveSafeExtension bool
}

func New(maxBytes, hashLength int, metadataSuffix string, preserveExt bool) Namer {
	if maxBytes <= 0 {
		maxBytes = 120
	}
	if hashLength <= 0 {
		hashLength = 16
	}
	if metadataSuffix == "" {
		metadataSuffix = ".meta"
	}
	return Namer{
		MaxBytes:              maxBytes,
		HashLength:            hashLength,
		MetadataSuffix:        metadataSuffix,
		PreserveSafeExtension: preserveExt,
	}
}

func (n Namer) IsSafe(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if !utf8.ValidString(name) {
		return false
	}
	if len([]byte(name)) > n.MaxBytes {
		return false
	}
	if strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return false
	}
	if strings.HasSuffix(name, " ") || strings.HasSuffix(name, ".") {
		return false
	}
	if strings.HasSuffix(strings.ToLower(name), strings.ToLower(n.MetadataSuffix)) {
		return false
	}
	for _, r := range name {
		if r < 0x20 || strings.ContainsRune(`<>:"|?*`, r) {
			return false
		}
	}
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	switch strings.ToUpper(stem) {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	if len(stem) == 4 {
		prefix := strings.ToUpper(stem[:3])
		last := stem[3]
		if (prefix == "COM" || prefix == "LPT") && last >= '1' && last <= '9' {
			return false
		}
	}
	return true
}

func (n Namer) StorageName(original string, isDir bool) (name string, direct bool) {
	if n.IsSafe(original) {
		return original, true
	}

	base := n.Hash(original)
	if !isDir && n.PreserveSafeExtension {
		if ext := n.safeExtension(original); ext != "" && len([]byte(base+ext)) <= n.MaxBytes {
			return base + ext, false
		}
	}
	return base, false
}

func (n Namer) Hash(name string) string {
	sum := sha256.Sum256([]byte(name))
	encoded := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]))
	if n.HashLength > len(encoded) {
		return encoded
	}
	return encoded[:n.HashLength]
}

func (n Namer) safeExtension(name string) string {
	ext := filepath.Ext(name)
	if ext == "" || ext == name || len([]byte(ext)) > 20 {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(ext), strings.ToLower(n.MetadataSuffix)) {
		return ""
	}
	for _, r := range ext {
		if r < 0x20 || strings.ContainsRune(`<>:"|?*/\`, r) {
			return ""
		}
	}
	if strings.HasSuffix(ext, " ") || strings.HasSuffix(ext, ".") {
		return ""
	}
	return ext
}
