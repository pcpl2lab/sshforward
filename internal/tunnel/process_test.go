package tunnel

import (
	"os"
	"testing"
)

func TestIsProcessAlive_Self(t *testing.T) {
	pid := os.Getpid()
	if !IsProcessAlive(pid) {
		t.Error("current process should be alive")
	}
}

func TestIsProcessAlive_Invalid(t *testing.T) {
	if IsProcessAlive(999999999) {
		t.Error("invalid PID should not be alive")
	}
}
