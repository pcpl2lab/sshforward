package port

import (
	"net"
	"strconv"
	"testing"
)

func TestFindFreePort(t *testing.T) {
	p, err := FindFree()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p < 1024 || p > 65535 {
		t.Errorf("port %d out of expected range", p)
	}
}

func TestFindFreePort_IsActuallyFree(t *testing.T) {
	p, err := FindFree()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
	if err != nil {
		t.Fatalf("port %d reported free but cannot bind: %v", p, err)
	}
	ln.Close()
}

func TestCheckAvailable_Free(t *testing.T) {
	p, _ := FindFree()
	if err := CheckAvailable(p); err != nil {
		t.Errorf("expected port %d to be available: %v", p, err)
	}
}

func TestCheckAvailable_Occupied(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	p, _ := strconv.Atoi(portStr)

	if err := CheckAvailable(p); err == nil {
		t.Errorf("expected port %d to be reported as occupied", p)
	}
}
