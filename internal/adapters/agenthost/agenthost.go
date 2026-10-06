// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package agenthost implements the agent skill installation ports against
// the real machine: ports.AgentFS over the os package (directory symlinks
// included) and ports.CommandRunner over os/exec with a timeout.
package agenthost

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// FS implements ports.AgentFS.
type FS struct{}

// NewFS returns the real-filesystem AgentFS.
func NewFS() *FS { return &FS{} }

var _ ports.AgentFS = (*FS)(nil)

func native(p domain.Path) string { return filepath.FromSlash(string(p)) }

func (FS) Lstat(p domain.Path) (ports.Entry, error) {
	info, err := os.Lstat(native(p))
	if errors.Is(err, fs.ErrNotExist) {
		return ports.Entry{Kind: ports.EntryMissing}, nil
	}
	if err != nil {
		return ports.Entry{}, err
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(native(p))
		if err != nil {
			return ports.Entry{}, err
		}
		return ports.Entry{Kind: ports.EntrySymlink, LinkTarget: filepath.ToSlash(target)}, nil
	case info.IsDir():
		return ports.Entry{Kind: ports.EntryDir}, nil
	}
	return ports.Entry{Kind: ports.EntryFile}, nil
}

func (FS) Exists(p domain.Path) (bool, error) {
	_, err := os.Stat(native(p))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (FS) EvalSymlinks(p domain.Path) (domain.Path, error) {
	real, err := filepath.EvalSymlinks(native(p))
	if err != nil {
		return "", err
	}
	return domain.Path(filepath.ToSlash(real)), nil
}

func (FS) MkdirAll(p domain.Path) error { return os.MkdirAll(native(p), 0o755) }

// Symlink creates a directory symlink. On Windows, where creating one
// needs Developer Mode or an elevated process, a failure is reported as
// ports.ErrSymlinkUnsupported so the caller copies instead (directory
// junctions would need golang.org/x/sys/windows; not implemented).
func (FS) Symlink(target, link domain.Path) error {
	err := os.Symlink(native(target), native(link))
	if err != nil && runtime.GOOS == "windows" && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%w: %v", ports.ErrSymlinkUnsupported, err)
	}
	return err
}

func (FS) Rename(from, to domain.Path) error { return os.Rename(native(from), native(to)) }

func (FS) RemoveLink(p domain.Path) error {
	info, err := os.Lstat(native(p))
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return fmt.Errorf("%s: %w", p, ports.ErrNotASymlink)
	}
	return os.Remove(native(p))
}

func (FS) RemoveAll(p domain.Path) error { return os.RemoveAll(native(p)) }

func (FS) ReadFile(p domain.Path) ([]byte, error) { return os.ReadFile(native(p)) }

func (FS) WriteFile(p domain.Path, data []byte, perm fs.FileMode) error {
	return os.WriteFile(native(p), data, perm)
}

// CopyDir copies regular files and directories from src to dst; dst must
// not exist. Symlinks inside src are not followed.
func (FS) CopyDir(src, dst domain.Path) error {
	s, d := native(src), native(dst)
	if _, err := os.Lstat(d); err == nil {
		return fmt.Errorf("copy to %s: %w", dst, fs.ErrExist)
	}
	return filepath.WalkDir(s, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(s, path)
		if err != nil {
			return err
		}
		target := filepath.Join(d, rel)
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, 0o755)
		case entry.Type().IsRegular():
			return copyFile(path, target)
		}
		return nil
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// Runner implements ports.CommandRunner; every Run is bounded by Timeout.
// Programs are found by Resolver (the user's shell's rules, never a
// binary inside node_modules), not by os/exec's own lookup.
type Runner struct {
	Timeout  time.Duration
	Resolver Resolver
}

// NewRunner returns a Runner whose commands are killed after timeout.
func NewRunner(timeout time.Duration) *Runner {
	return &Runner{Timeout: timeout, Resolver: NewResolver()}
}

var _ ports.CommandRunner = (*Runner)(nil)

func (r *Runner) LookPath(name string) (string, bool) {
	return r.Resolver.LookPath(name)
}

func (r *Runner) Run(ctx context.Context, name string, args ...string) (ports.CommandResult, error) {
	return r.RunIn(ctx, "", name, args...)
}

func (r *Runner) RunIn(ctx context.Context, dir, name string, args ...string) (ports.CommandResult, error) {
	inv, err := r.Resolver.Command(name, args)
	if err != nil {
		return ports.CommandResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, inv.Path, inv.Args...)
	applyCmdLine(cmd, inv.CmdLine)
	cmd.Dir = filepath.FromSlash(dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	res := ports.CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if ctx.Err() != nil {
		return res, fmt.Errorf("%s: %w", name, ctx.Err())
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, err
}
