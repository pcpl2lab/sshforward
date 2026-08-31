package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
)

type FileLock struct {
	file *os.File
	path string
}

func LockPath(dir, host, service string) string {
	return filepath.Join(dir, fmt.Sprintf("%s-%s.lock", host, service))
}

func AcquireLock(path string) (*FileLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("cannot create lock directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("cannot open lock file: %w", err)
	}

	if err := lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("cannot acquire lock: %w", err)
	}

	return &FileLock{file: f, path: path}, nil
}

func (l *FileLock) Release() error {
	if l.file == nil {
		return nil
	}
	_ = unlockFile(l.file) // best-effort: Close below releases the lock anyway
	err := l.file.Close()
	l.file = nil
	_ = os.Remove(l.path) // best-effort cleanup of the lock file
	return err
}
