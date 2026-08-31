package port

import (
	"fmt"
	"net"
	"strconv"
)

// MinUnprivilegedPort is the lowest port automatic selection will hand out.
// Ports 1-1023 are the IANA system ("well-known") range: they are assigned to
// standard services and binding them requires elevated privileges on Unix.
const MinUnprivilegedPort = 1024

// reserveAttempts bounds the retry loop for automatic selection. The kernel
// allocates from the ephemeral range, which sits far above the reserved range
// on every supported platform, so a retry is a guard rather than the expected
// path.
const reserveAttempts = 10

// Reservation holds a local TCP port open so that nothing else can claim it
// between the moment sshforward picks it and the moment ssh binds it. Release
// hands the port over and must be called as late as possible — ideally right
// before the ssh process is spawned.
type Reservation struct {
	ln   net.Listener
	port int
}

// Port returns the reserved port number. It stays valid after Release.
func (r *Reservation) Port() int {
	return r.port
}

// Release gives the port up. It is safe to call more than once.
func (r *Reservation) Release() {
	if r == nil || r.ln == nil {
		return
	}
	r.ln.Close()
	r.ln = nil
}

// Reserve claims p, or an arbitrary free port at or above MinUnprivilegedPort
// when p is 0. The caller owns the returned Reservation and must Release it.
func Reserve(p int) (*Reservation, error) {
	if p != 0 {
		ln, err := listen(p)
		if err != nil {
			return nil, fmt.Errorf("port %d is already in use", p)
		}
		return &Reservation{ln: ln, port: p}, nil
	}

	var last int
	for range reserveAttempts {
		ln, err := listen(0)
		if err != nil {
			return nil, fmt.Errorf("cannot find free port: %w", err)
		}
		got, err := portOf(ln)
		if err != nil {
			ln.Close()
			return nil, err
		}
		if got >= MinUnprivilegedPort {
			return &Reservation{ln: ln, port: got}, nil
		}
		// Never hand out a reserved system port; drop it and draw again.
		ln.Close()
		last = got
	}
	return nil, fmt.Errorf("cannot find a free port at or above %d (last candidate: %d)", MinUnprivilegedPort, last)
}

// FindFree returns a free local TCP port at or above MinUnprivilegedPort.
// The port is released before returning, so it is only a hint; use Reserve when
// the port must still be free by the time it is bound.
func FindFree() (int, error) {
	res, err := Reserve(0)
	if err != nil {
		return 0, err
	}
	defer res.Release()
	return res.Port(), nil
}

// CheckAvailable reports whether p can be bound right now. Like FindFree this
// is only a hint; Reserve is the race-free alternative.
func CheckAvailable(p int) error {
	res, err := Reserve(p)
	if err != nil {
		return err
	}
	res.Release()
	return nil
}

func listen(p int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
}

func portOf(ln net.Listener) (int, error) {
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return 0, fmt.Errorf("cannot parse address: %w", err)
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("cannot parse port: %w", err)
	}
	return p, nil
}
