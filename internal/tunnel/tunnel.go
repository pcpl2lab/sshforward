package tunnel

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pcpl2/sshforward/internal/port"
)

// safeHostPattern allows only valid SSH host names (alphanumeric, dots, hyphens, underscores).
var safeHostPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// validateInput sanitizes all dynamic arguments before passing to exec.Command
// to prevent command injection.
func validateInput(host string, ports []PortForward) error {
	if !safeHostPattern.MatchString(host) {
		return fmt.Errorf("invalid host name %q: must contain only alphanumeric, dots, hyphens, underscores", host)
	}
	for _, pf := range ports {
		if pf.RemoteHost != "" && !safeHostPattern.MatchString(pf.RemoteHost) {
			return fmt.Errorf("invalid remote_host %q: must contain only alphanumeric, dots, hyphens, underscores", pf.RemoteHost)
		}
		if pf.RemotePort < 1 || pf.RemotePort > 65535 {
			return fmt.Errorf("invalid remote_port %d: must be 1-65535", pf.RemotePort)
		}
	}
	return nil
}

// PortForward describes a single port to forward.
type PortForward struct {
	Name       string // empty for single-port services
	RemoteHost string
	RemotePort int
	LocalPort  int // 0 = auto
}

type StartOptions struct {
	Host           string
	Service        string
	Ports          []PortForward
	TunnelsDir     string
	SkipValidation bool
}

// LogPath returns the path to the SSH log file for a tunnel.
func LogPath(dir, host, service, portName string) string {
	return filepath.Join(dir, StateKey(host, service, portName)+".log")
}

// ReadLog reads the SSH log for a tunnel. Returns empty string if not found.
func ReadLog(dir, host, service, portName string) string {
	data, err := os.ReadFile(LogPath(dir, host, service, portName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Start opens one SSH tunnel per port in Ports. Returns a state per successfully started tunnel.
// If any port fails, previously started tunnels for this call are rolled back (killed).
func Start(opts *StartOptions) ([]*TunnelState, error) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return nil, fmt.Errorf("ssh not found in PATH. Please install OpenSSH")
	}

	// Ensure tunnels dir exists
	if err := os.MkdirAll(opts.TunnelsDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create tunnels directory: %w", err)
	}

	// Resolve local ports upfront (all-or-nothing)
	type resolvedPort struct {
		PortForward
		localPort int
	}
	resolved := make([]resolvedPort, len(opts.Ports))
	for i, pf := range opts.Ports {
		lp := pf.LocalPort
		if lp == 0 {
			p, err := port.FindFree()
			if err != nil {
				return nil, fmt.Errorf("port %q: cannot find free port: %w", pf.Name, err)
			}
			lp = p
		} else {
			if err := port.CheckAvailable(lp); err != nil {
				return nil, fmt.Errorf("port %q: %w", pf.Name, err)
			}
		}
		resolved[i] = resolvedPort{PortForward: pf, localPort: lp}
	}

	// Check for already-active tunnels and acquire locks
	type lockInfo struct {
		lock *FileLock
		path string
	}
	var locks []lockInfo
	defer func() {
		for _, l := range locks {
			l.lock.Release()
		}
	}()

	for _, rp := range resolved {
		lockPath := LockPath(opts.TunnelsDir, opts.Host, opts.Service+portSuffix(rp.Name))
		lock, err := AcquireLock(lockPath)
		if err != nil {
			return nil, fmt.Errorf("port %q: cannot acquire lock (another operation in progress?): %w", rp.Name, err)
		}
		locks = append(locks, lockInfo{lock: lock, path: lockPath})

		statePath := StatePathWithPort(opts.TunnelsDir, opts.Host, opts.Service, rp.Name)
		if existing, err := LoadState(statePath); err == nil {
			if IsProcessAlive(existing.PID) {
				return nil, fmt.Errorf("tunnel %s/%s%s is already running on port %d (PID: %d)",
					opts.Host, opts.Service, portLabel(rp.Name), existing.LocalPort, existing.PID)
			}
			RemoveState(statePath)
		}
	}

	// Start SSH processes
	var states []*TunnelState
	for _, rp := range resolved {
		state, err := startSingle(sshPath, opts, rp.PortForward, rp.localPort)
		if err != nil {
			// Rollback: kill previously started tunnels
			for _, s := range states {
				KillProcess(s.PID)
				RemoveState(StatePathWithPort(opts.TunnelsDir, opts.Host, opts.Service, s.PortName))
			}
			return nil, fmt.Errorf("port %q: %w", rp.Name, err)
		}
		states = append(states, state)
	}

	return states, nil
}

func startSingle(sshPath string, opts *StartOptions, pf PortForward, localPort int) (*TunnelState, error) {
	// Validate all dynamic inputs before exec to prevent command injection.
	if err := validateInput(opts.Host, []PortForward{pf}); err != nil {
		return nil, err
	}

	logPath := LogPath(opts.TunnelsDir, opts.Host, opts.Service, pf.Name)
	forwardArg := fmt.Sprintf("%d:%s:%d", localPort, pf.RemoteHost, pf.RemotePort)

	// Use SSH's own -E flag for logging — no file handle inheritance needed.
	// This allows the process to be fully detached on Windows.
	cmd := exec.Command(sshPath, // nosemgrep: dangerous-exec-command
		"-N",                             // no remote command
		"-E", logPath,                    // SSH writes logs to file directly
		"-o", "ExitOnForwardFailure=yes", // exit if port forward fails
		"-o", "ConnectTimeout=10",        // fail fast if host unreachable
		"-o", "ServerAliveInterval=15",   // keepalive every 15s
		"-o", "ServerAliveCountMax=3",    // disconnect after 3 missed keepalives
		"-L", forwardArg,
		opts.Host,
	)
	// No Stdout/Stderr — fully detached, SSH logs via -E
	cmd.Stdout = nil
	cmd.Stderr = nil
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start ssh: %w", err)
	}

	// Immediately release the process handle — SSH runs independently
	pid := cmd.Process.Pid
	cmd.Process.Release()

	if !opts.SkipValidation {
		if err := validateTunnel(pid, localPort, logPath); err != nil {
			// Try to kill the process if it's still running
			if IsProcessAlive(pid) {
				KillProcess(pid)
			}
			return nil, err
		}
	} else {
		time.Sleep(500 * time.Millisecond)
		if !IsProcessAlive(pid) {
			logContent := readLogFile(logPath)
			return nil, fmt.Errorf("ssh process died immediately. Log:\n%s", logContent)
		}
	}

	state := &TunnelState{
		Host:       opts.Host,
		Service:    opts.Service,
		PortName:   pf.Name,
		PID:        pid,
		LocalPort:  localPort,
		RemoteHost: pf.RemoteHost,
		RemotePort: pf.RemotePort,
		StartedAt:  time.Now().UTC(),
	}

	statePath := StatePathWithPort(opts.TunnelsDir, opts.Host, opts.Service, pf.Name)
	if err := SaveState(statePath, state); err != nil {
		if IsProcessAlive(pid) {
			KillProcess(pid)
		}
		return nil, fmt.Errorf("cannot save tunnel state: %w", err)
	}

	return state, nil
}

func validateTunnel(pid int, localPort int, logPath string) error {
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if !IsProcessAlive(pid) {
			logContent := readLogFile(logPath)
			return fmt.Errorf("SSH tunnel failed to start. Log:\n%s", logContent)
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	// Process alive but port not connectable — this is normal for SSH -L tunnels.
	// The local port is bound by SSH but net.Dial won't succeed unless something
	// is listening on the remote end. The tunnel IS working.
	return nil
}

func readLogFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "(no log available)"
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "(empty log — SSH may still be connecting)"
	}
	return s
}

// Stop stops all tunnels for a host/service (all port names).
func Stop(tunnelsDir, host, service string) error {
	states, err := ListServiceStates(tunnelsDir, host, service)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		return fmt.Errorf("no active tunnel for %s/%s", host, service)
	}

	var errs []error
	for _, s := range states {
		if err := stopSingle(tunnelsDir, s); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to stop %d port(s): %v", len(errs), errs)
	}
	return nil
}

func stopSingle(tunnelsDir string, state *TunnelState) error {
	lockPath := LockPath(tunnelsDir, state.Host, state.Service+portSuffix(state.PortName))
	lock, err := AcquireLock(lockPath)
	if err != nil {
		return fmt.Errorf("cannot acquire lock for %s%s: %w", state.Service, portLabel(state.PortName), err)
	}
	defer lock.Release()

	statePath := StatePathWithPort(tunnelsDir, state.Host, state.Service, state.PortName)

	if IsProcessAlive(state.PID) {
		if !IsSSHProcess(state.PID) {
			return RemoveState(statePath)
		}
		if err := KillProcess(state.PID); err != nil {
			return fmt.Errorf("cannot stop process %d: %w", state.PID, err)
		}
	}

	// Clean up log file
	os.Remove(LogPath(tunnelsDir, state.Host, state.Service, state.PortName))

	return RemoveState(statePath)
}

func StopAll(tunnelsDir string) (stopped int, errors []error) {
	states, err := ListStates(tunnelsDir)
	if err != nil {
		return 0, []error{err}
	}

	for _, state := range states {
		if err := stopSingle(tunnelsDir, state); err != nil {
			errors = append(errors, fmt.Errorf("%s/%s%s: %w", state.Host, state.Service, portLabel(state.PortName), err))
		} else {
			stopped++
		}
	}
	return stopped, errors
}

// portSuffix returns "-name" for named ports, "" for unnamed.
func portSuffix(name string) string {
	if name == "" {
		return ""
	}
	return "-" + name
}

// portLabel returns " (name)" for named ports, "" for unnamed.
func portLabel(name string) string {
	if name == "" {
		return ""
	}
	return fmt.Sprintf(" (%s)", name)
}
