//go:build linux || darwin || freebsd || openbsd || netbsd
// +build linux darwin freebsd openbsd netbsd

package okf

import (
	"fmt"
	"golang.org/x/sys/unix"
)

// unixFileLock 是 Unix 平台的 flock 实现
type unixFileLock struct {
	fd int
}

func newFileLock(path string) (FileLock, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CREAT, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}
	return &unixFileLock{fd: fd}, nil
}

func (l *unixFileLock) Lock() error {
	return unix.Flock(l.fd, unix.LOCK_EX)
}

func (l *unixFileLock) Unlock() error {
	return unix.Flock(l.fd, unix.LOCK_UN)
}

func (l *unixFileLock) Close() error {
	return unix.Close(l.fd)
}
