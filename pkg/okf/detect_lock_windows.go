//go:build windows

package okf

import (
	"fmt"
	"golang.org/x/sys/windows"
)

// windowsFileLock 是 Windows 平台的 LockFileEx 实现
type windowsFileLock struct {
	fd windows.Handle
}

func newFileLock(path string) (FileLock, error) {
	fd, err := windows.Open(path, windows.O_RDONLY|windows.O_CREAT, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}
	return &windowsFileLock{fd: fd}, nil
}

func (l *windowsFileLock) Lock() error {
	var overlapped windows.Overlapped
	return windows.LockFileEx(l.fd, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped)
}

func (l *windowsFileLock) Unlock() error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(l.fd, 0, 1, 0, &overlapped)
}
