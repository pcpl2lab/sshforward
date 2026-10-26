//go:build !windows

package tunnel

import (
	"testing"
	"time"
)

func TestProcStartTime(t *testing.T) {
	// comm with spaces and a ')' must not shift the field count; starttime is
	// 250 ticks (2.5s) after a boot at btime.
	stat := []byte("4242 (ssh (tunnel) x) S 1 4242 4242 0 -1 4194560 100 0 0 0 0 0 0 0 20 0 1 0 250 1000 200")
	sys := []byte("cpu  1 2 3 4\nintr 0\nbtime 1700000000\nprocesses 10\n")

	got, err := procStartTime(stat, sys)
	if err != nil {
		t.Fatalf("procStartTime: %v", err)
	}
	want := time.Unix(1700000000, 0).Add(2500 * time.Millisecond)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestProcStartTime_Malformed(t *testing.T) {
	sys := []byte("btime 1700000000\n")
	for _, stat := range []string{"", "4242 ssh S 1", "4242 (ssh) S 1 2 3"} {
		if _, err := procStartTime([]byte(stat), sys); err == nil {
			t.Errorf("expected an error for stat %q", stat)
		}
	}
	if _, err := procStartTime([]byte("4242 (ssh) S 1 4242 4242 0 -1 4194560 100 0 0 0 0 0 0 0 20 0 1 0 250 1000 200"), []byte("cpu 1\n")); err == nil {
		t.Error("expected an error when btime is missing")
	}
}
