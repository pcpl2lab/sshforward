package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LogRetention is how long an ssh log outlives its tunnel. The log is the only
// record of why a start failed or a tunnel dropped, so it is kept well past
// the point where anyone would still be looking into it.
const LogRetention = 14 * 24 * time.Hour

// archiveTimeLayout stamps an archived log with when its run last wrote to it.
const archiveTimeLayout = "20060102-150405"

// ArchivedLog is an ssh log no state file refers to: left by a start that
// failed, a tunnel that died, or an earlier run of a tunnel started again.
type ArchivedLog struct {
	Path    string
	ModTime time.Time
}

// Read returns the log's content, or "" if it is gone or empty.
func (a ArchivedLog) Read() string {
	data, err := os.ReadFile(a.Path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// archiveLog moves the log of a previous run out of the way before a tunnel is
// started again. ssh -E appends, so the new run's log would otherwise carry
// the old run's output. An empty log holds nothing worth keeping.
func archiveLog(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Size() == 0 {
		return os.Remove(path)
	}
	archived := strings.TrimSuffix(path, ".log") + "." + info.ModTime().Format(archiveTimeLayout) + ".log"
	// A concurrent `list` may have expired the log since the Stat above; it is
	// out of the way either way.
	if err := os.Rename(path, archived); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ListArchivedLogs returns the logs in dir that no state file refers to,
// oldest first. A non-empty key (see StateKey) limits them to that tunnel or
// service, including all of its ports.
func ListArchivedLogs(dir, key string) ([]ArchivedLog, error) {
	logs, err := orphanLogs(dir)
	if err != nil {
		return nil, err
	}
	if key != "" {
		matched := logs[:0]
		for _, l := range logs {
			rest, ok := strings.CutPrefix(filepath.Base(l.Path), key)
			if ok && (strings.HasPrefix(rest, ".") || strings.HasPrefix(rest, "-")) {
				matched = append(matched, l)
			}
		}
		logs = matched
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].ModTime.Before(logs[j].ModTime) })
	return logs, nil
}

// CleanOldLogs removes logs that no state file refers to once they are older
// than LogRetention. Returns the number of logs removed.
func CleanOldLogs(dir string) (int, error) {
	logs, err := orphanLogs(dir)
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-LogRetention)
	removed := 0
	for _, l := range logs {
		if l.ModTime.Before(cutoff) && os.Remove(l.Path) == nil {
			removed++
		}
	}
	return removed, nil
}

// orphanLogs returns the logs in dir without a matching state file.
func orphanLogs(dir string) ([]ArchivedLog, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read tunnels directory: %w", err)
	}

	var logs []ArchivedLog
	for _, entry := range entries {
		key, ok := strings.CutSuffix(entry.Name(), ".log")
		if entry.IsDir() || !ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, key+".json")); err == nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		logs = append(logs, ArchivedLog{Path: filepath.Join(dir, entry.Name()), ModTime: info.ModTime()})
	}
	return logs, nil
}
