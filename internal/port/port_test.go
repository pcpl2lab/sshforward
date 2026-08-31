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
	if p < MinUnprivilegedPort || p > 65535 {
		t.Errorf("got port %d, want one in %d-65535", p, MinUnprivilegedPort)
	}
}

func TestFindFreePort_NeverReserved(t *testing.T) {
	// Automatic selection must stay out of the 1-1023 system range.
	for i := range 20 {
		p, err := FindFree()
		if err != nil {
			t.Fatalf("draw %d: unexpected error: %v", i, err)
		}
		if p < MinUnprivilegedPort {
			t.Fatalf("draw %d: got reserved port %d, want >= %d", i, p, MinUnprivilegedPort)
		}
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

func TestReserve_HoldsThePortUntilReleased(t *testing.T) {
	res, err := Reserve(0)
	if err != nil {
		t.Fatalf("Reserve(0) failed: %v", err)
	}
	defer res.Release()

	// The whole point of a reservation: nobody else can take the port while it
	// is held, which is what closes the window between picking and binding.
	if err := CheckAvailable(res.Port()); err == nil {
		t.Errorf("port %d must not be available while reserved", res.Port())
	}

	res.Release()
	if err := CheckAvailable(res.Port()); err != nil {
		t.Errorf("port %d must be available after Release: %v", res.Port(), err)
	}
}

func TestReserve_ConcurrentReservationsGetDistinctPorts(t *testing.T) {
	a, err := Reserve(0)
	if err != nil {
		t.Fatalf("first Reserve(0) failed: %v", err)
	}
	defer a.Release()

	b, err := Reserve(0)
	if err != nil {
		t.Fatalf("second Reserve(0) failed: %v", err)
	}
	defer b.Release()

	if a.Port() == b.Port() {
		t.Errorf("both reservations got port %d; a multi-port service would collide with itself", a.Port())
	}
}

func TestReserve_OccupiedPortIsRejected(t *testing.T) {
	held, err := Reserve(0)
	if err != nil {
		t.Fatalf("Reserve(0) failed: %v", err)
	}
	defer held.Release()

	if _, err := Reserve(held.Port()); err == nil {
		t.Errorf("Reserve(%d) must fail while the port is held", held.Port())
	}
}

func TestReserve_ReleaseIsIdempotent(t *testing.T) {
	res, err := Reserve(0)
	if err != nil {
		t.Fatalf("Reserve(0) failed: %v", err)
	}
	p := res.Port()

	res.Release()
	res.Release() // must not panic or close a second time

	if res.Port() != p {
		t.Errorf("got port %d after Release, want the reserved %d", res.Port(), p)
	}
}

func TestReserve_NeverReserved(t *testing.T) {
	for i := range 20 {
		res, err := Reserve(0)
		if err != nil {
			t.Fatalf("draw %d: Reserve(0) failed: %v", i, err)
		}
		if res.Port() < MinUnprivilegedPort {
			res.Release()
			t.Fatalf("draw %d: got reserved port %d, want >= %d", i, res.Port(), MinUnprivilegedPort)
		}
		res.Release()
	}
}
