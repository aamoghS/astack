package main

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func isolateWorkdir(src string) (string, error) {
	dst, err := os.MkdirTemp("", "astack-iso-")
	if err != nil {
		return "", err
	}
	if err := copyTree(src, dst); err != nil {
		os.RemoveAll(dst)
		return "", err
	}
	return dst, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		base := filepath.Base(path)
		if skipName(base) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}

func skipName(base string) bool {
	switch base {
	case ".git", "node_modules", "bin", lockName:
		return true
	}
	return strings.HasPrefix(base, ".astack-prompt")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func treeBytes(root string) int64 {
	var n int64
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		st, err := os.Stat(path)
		if err != nil {
			return nil
		}
		n += st.Size()
		return nil
	})
	return n
}
