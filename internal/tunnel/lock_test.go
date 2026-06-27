package tunnel

import (
	"path/filepath"
	"testing"
)

func TestLockAndUnlock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	lock, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer lock.Release()
}
