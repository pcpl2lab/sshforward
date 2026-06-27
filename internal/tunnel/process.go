package tunnel

// IsProcessAlive checks if a process with the given PID exists.
// IsSSHProcess checks if the PID belongs to an ssh process (prevents PID recycling kills).
// KillProcess sends a termination signal to the process.
// All have platform-specific implementations in process_unix.go and process_windows.go.
