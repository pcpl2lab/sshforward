package port

import (
	"fmt"
	"net"
	"strconv"
)

func FindFree() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("cannot find free port: %w", err)
	}
	defer ln.Close()

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

func CheckAvailable(p int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
	if err != nil {
		return fmt.Errorf("port %d is already in use", p)
	}
	ln.Close()
	return nil
}
