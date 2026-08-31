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

// lookSSHPath resolves the ssh client. It is a variable so tests can substitute
// a stand-in binary: the real client's lifetime depends on whether an sshd
// happens to answer, which would make start/stop tests pass or fail by accident
// of the environment rather than by the code under test.
var lookSSHPath = func() (string, error) { return exec.LookPath("ssh") }

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
	sshPath, err := lookSSHPath()
	if err != nil {
		return nil, fmt.Errorf("ssh not found in PATH. Please install OpenSSH")
	}

	// Ensure tunnels dir exists
	if err := os.MkdirAll(opts.TunnelsDir, 0700); err != nil {
		return nil, fmt.Errorf("cannot create tunnels directory: %w", err)
	}

	// Resolve local ports upfront (all-or-nothing). Each port stays reserved —
	// held open by a listener of ours — until the ssh process that will bind it
	// is spawned, so neither a concurrent sshforward run nor a second port of
	// this same service can take it in the meantime.
	type resolvedPort struct {
		PortForward
		reservation *port.Reservation
	}
	resolved := make([]resolvedPort, 0, len(opts.Ports))
	defer func() {
		for _, rp := range resolved {
			rp.reservation.Release() // no-op once handed over to ssh
		}
	}()
	for _, pf := range opts.Ports {
		res, err := port.Reserve(pf.LocalPort)
		if err != nil {
			return nil, fmt.Errorf("port %q: %w", pf.Name, err)
		}
		resolved = append(resolved, resolvedPort{PortForward: pf, reservation: res})
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
			// IsTunnelAlive, not IsProcessAlive: a recycled PID belonging to an
			// unrelated process must not block a start. Matches list and stop.
			if IsTunnelAlive(existing.PID) {
				return nil, fmt.Errorf("tunnel %s/%s%s is already running on port %d (PID: %d)",
					opts.Host, opts.Service, portLabel(rp.Name), existing.LocalPort, existing.PID)
			}
			_ = RemoveState(statePath) // stale state from a dead tunnel
		}
	}

	// Start SSH processes
	var states []*TunnelState
	for _, rp := range resolved {
		state, err := startSingle(sshPath, opts, rp.PortForward, rp.reservation)
		if err != nil {
			// Rollback: kill previously started tunnels (best-effort)
			for _, s := range states {
				_ = KillProcess(s.PID)
				_ = RemoveState(StatePathWithPort(opts.TunnelsDir, opts.Host, opts.Service, s.PortName))
			}
			return nil, fmt.Errorf("port %q: %w", rp.Name, err)
		}
		states = append(states, state)
	}

	return states, nil
}

// buildSSHArgs constructs the argument list for the detached ssh process.
// BatchMode=yes turns interactive prompts (unknown host, password) into
// immediate failures logged to -E, instead of silently hanging a process
// that has no stdin. StrictHostKeyChecking=accept-new auto-trusts new hosts
// but still refuses on a changed key.
func buildSSHArgs(logPath, forwardArg, host string) []string {
	return []string{
		"-N",          // no remote command
		"-E", logPath, // SSH writes logs to file directly
		"-o", "BatchMode=yes", // fail fast, never prompt (no stdin when detached)
		"-o", "StrictHostKeyChecking=accept-new", // trust new hosts, reject changed keys
		"-o", "ExitOnForwardFailure=yes", // exit if port forward fails
		"-o", "ConnectTimeout=10", // fail fast if host unreachable
		"-o", "ServerAliveInterval=15", // keepalive every 15s
		"-o", "ServerAliveCountMax=3", // disconnect after 3 missed keepalives
		"-L", forwardArg,
		host,
	}
}

func startSingle(sshPath string, opts *StartOptions, pf PortForward, res *port.Reservation) (*TunnelState, error) {
	// Validate all dynamic inputs before exec to prevent command injection.
	if err := validateInput(opts.Host, []PortForward{pf}); err != nil {
		return nil, err
	}

	localPort := res.Port()
	logPath := LogPath(opts.TunnelsDir, opts.Host, opts.Service, pf.Name)
	forwardArg := fmt.Sprintf("%d:%s:%d", localPort, pf.RemoteHost, pf.RemotePort)

	// Use SSH's own -E flag for logging — no file handle inheritance needed.
	// This allows the process to be fully detached on Windows.
	cmd := exec.Command(sshPath, buildSSHArgs(logPath, forwardArg, opts.Host)...) // nosemgrep: dangerous-exec-command
	// No Stdout/Stderr — fully detached, SSH logs via -E
	cmd.Stdout = nil
	cmd.Stderr = nil
	setSysProcAttr(cmd)

	// Hand the port over as late as possible. Everything up to here has held it
	// reserved; what remains is the sliver between this close and the child's
	// own bind(), which ExitOnForwardFailure turns into a clean, logged failure.
	res.Release()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start ssh: %w", err)
	}

	// Immediately release the process handle — SSH runs independently
	pid := cmd.Process.Pid
	cmd.Process.Release()

	if !opts.SkipValidation {
		if err := validateTunnel(pid, localPort, logPath); err != nil {
			// Try to kill the process if it's still running (best-effort)
			if IsProcessAlive(pid) {
				_ = KillProcess(pid)
			}
			return nil, err
		}
	} else {
		time.Sleep(500 * time.Millisecond)
		if !IsProcessAlive(pid) {
			logContent := readLogFile(logPath)
			return nil, fmt.Errorf("ssh process died immediately%s. Log:\n%s", bindFailureHint(logContent), logContent)
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
			_ = KillProcess(pid)
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
			return fmt.Errorf("SSH tunnel failed to start%s. Log:\n%s", bindFailureHint(logContent), logContent)
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

// bindFailureHint recognises the one race a reservation cannot close: another
// process claiming the local port in the instant between sshforward releasing
// it and ssh calling bind(). Retrying is all the user can do about it.
func bindFailureHint(log string) string {
	l := strings.ToLower(log)
	if strings.Contains(l, "address already in use") || strings.Contains(l, "cannot listen to port") {
		return " because the local port was taken by another process just before ssh could bind it — retry"
	}
	return ""
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
		// Wait for the process to actually go away before dropping the state
		// file: reporting success while ssh still held the local port would
		// leave an untracked tunnel behind.
		if err := TerminateAndWait(state.PID); err != nil {
			return err
		}
	}

	// Clean up log file (best-effort — it may not exist)
	_ = os.Remove(LogPath(tunnelsDir, state.Host, state.Service, state.PortName))

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
