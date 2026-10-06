// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package treecopy implements ports.TreeCloner: copy-on-write cloning of
// a directory tree where the filesystem supports it — clonefile(2) on
// macOS (APFS), the FICLONE ioctl per file on Linux (Btrfs, XFS) — and a
// regular copy otherwise (and always on Windows). Symbolic links are
// recreated as links, never followed, and permission bits are preserved.
package treecopy

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// Method names, as ports.TreeCloner reports them.
const (
	MethodClone   = ports.CloneMethodClone
	MethodReflink = ports.CloneMethodReflink
	MethodCopy    = ports.CloneMethodCopy
)

// Copier implements ports.TreeCloner.
type Copier struct{}

var _ ports.TreeCloner = Copier{}

// New returns the Copier.
func New() Copier { return Copier{} }

// CloneTree copies src to dst (which must not exist) with the fastest
// method available, removing a partial dst on failure.
func (Copier) CloneTree(src, dst domain.Path) (string, error) {
	s, d := filepath.FromSlash(string(src)), filepath.FromSlash(string(dst))
	if _, err := os.Lstat(d); err == nil {
		return "", fmt.Errorf("copy to %s: %w", dst, fs.ErrExist)
	}
	method, err := cloneTree(s, d)
	if err != nil {
		_ = os.RemoveAll(d)
		return "", err
	}
	return method, nil
}

// fileCloner tries a copy-on-write clone of one regular file into the
// already created, empty dst; false means "not supported here, copy".
type fileCloner func(src, dst *os.File) bool

// copyTree recreates the tree at src under dst: directories with their
// permission bits, symbolic links with their raw target text (and on
// Windows, where a junction reads as an irregular file, the link it
// resolves to), regular files cloned by clone when it succeeds and
// copied otherwise. Other file types (sockets, devices) are skipped.
// It reports whether clone succeeded for any file.
func copyTree(src, dst string, clone fileCloner) error {
	_, err := copyTreeCounting(src, dst, clone)
	return err
}

func copyTreeCounting(src, dst string, clone fileCloner) (cloned int, err error) {
	if _, err := os.Lstat(dst); err == nil {
		return 0, fmt.Errorf("copy to %s: %w", dst, fs.ErrExist)
	}
	err = filepath.WalkDir(src, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		switch {
		case mode&fs.ModeSymlink != 0 || (runtime.GOOS == "windows" && mode&fs.ModeIrregular != 0):
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case mode.IsDir():
			if err := os.Mkdir(target, mode.Perm()|0o700); err != nil {
				return err
			}
			return nil
		case mode.IsRegular():
			ok, err := copyFile(p, target, mode.Perm(), clone)
			if ok {
				cloned++
			}
			return err
		}
		return nil
	})
	return cloned, err
}

// copyFile writes src's content to the new file dst with perm (chmod'ed
// explicitly, so the umask cannot drop the executable bit).
func copyFile(src, dst string, perm fs.FileMode, clone fileCloner) (bool, error) {
	in, err := os.Open(src)
	if err != nil {
		return false, err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm|0o200)
	if err != nil {
		return false, err
	}
	cloned := clone != nil && clone(in, out)
	if !cloned {
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return false, err
		}
	}
	if err := out.Close(); err != nil {
		return cloned, err
	}
	return cloned, os.Chmod(dst, perm)
}
