//go:build windows

package tunnel

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock   = 0x02
	lockfileFailImmediately = 0x01
)

func lockFile(f *os.File) error {
	var overlapped syscall.Overlapped
	r1, _, err := procLockFileEx.Call(
		f.Fd(),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		1, 0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 == 0 {
		return err
	}
	return nil
}

func unlockFile(f *os.File) error {
	var overlapped syscall.Overlapped
	r1, _, err := procUnlockFileEx.Call(
		f.Fd(),
		0,
		1, 0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 == 0 {
		return err
	}
	return nil
}
