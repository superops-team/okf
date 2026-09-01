// Package renameio is a minimal, cross-platform fork of github.com/google/renameio v1.0.1.
//
// The upstream v1.0.1 ships tempfile.go with a `// +build !windows` build tag,
// which leaves TempFile undefined on Windows and breaks downstream packages that
// compile all files unconditionally (e.g. github.com/coder/hnsw). This fork
// removes that tag and implements the same PendingFile semantics with the
// standard library only, so it builds on every GOOS.
//
// License: Apache License 2.0 (see LICENSE in upstream).
package renameio

import (
	"os"
	"path/filepath"
)

// PendingFile is a pending temporary file, waiting to replace the destination
// path in a call to CloseAtomicallyReplace.
type PendingFile struct {
	*os.File

	path   string
	done   bool
	closed bool
}

// Cleanup is a no-op if CloseAtomicallyReplace succeeded, and otherwise closes
// and removes the temporary file.
func (t *PendingFile) Cleanup() error {
	if t.done {
		return nil
	}
	var closeErr error
	if !t.closed {
		closeErr = t.Close()
	}
	if err := os.Remove(t.Name()); err != nil {
		return err
	}
	return closeErr
}

// CloseAtomicallyReplace closes the temporary file and atomically replaces the
// destination file with it. On platforms where os.Rename cannot replace an
// existing file, the caller is responsible for removing the destination first.
func (t *PendingFile) CloseAtomicallyReplace() error {
	if err := t.Sync(); err != nil {
		return err
	}
	t.closed = true
	if err := t.Close(); err != nil {
		return err
	}
	if err := os.Rename(t.Name(), t.path); err != nil {
		return err
	}
	t.done = true
	return nil
}

// TempFile wraps os.CreateTemp for the use case of atomically creating or
// replacing the destination file at path. If dir is empty, os.TempDir() is used.
// The file's permissions are 0600 by default; use Chmod on the returned
// PendingFile to change them.
func TempFile(dir, path string) (*PendingFile, error) {
	if dir == "" {
		dir = os.TempDir()
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path))
	if err != nil {
		return nil, err
	}
	return &PendingFile{File: f, path: path}, nil
}
