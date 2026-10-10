package system

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Process contains process data observable through Linux procfs.
type Process struct {
	PID      int
	Name     string
	RSSBytes uint64
}

// FindXrayProcesses returns processes with the exact Linux comm name "xray".
// Unreadable entries are skipped: unprivileged users may not see every process.
func FindXrayProcesses(procRoot string) ([]Process, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}
	var processes []Process
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		base := filepath.Join(procRoot, entry.Name())
		rawName, err := os.ReadFile(filepath.Join(base, "comm"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(rawName))
		if name != "xray" {
			continue
		}
		p := Process{PID: pid, Name: name}
		rawStatm, err := os.ReadFile(filepath.Join(base, "statm"))
		if err == nil {
			fields := strings.Fields(string(rawStatm))
			if len(fields) >= 2 {
				if residentPages, e := strconv.ParseUint(fields[1], 10, 64); e == nil {
					p.RSSBytes = residentPages * uint64(os.Getpagesize())
				}
			}
		}
		processes = append(processes, p)
	}
	sort.Slice(processes, func(i, j int) bool { return processes[i].PID < processes[j].PID })
	return processes, nil
}

func FormatBytes(n uint64) string {
	const unit = uint64(1024)
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= float64(unit)
		if value < 1024 || suffix == "TiB" {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return "?"
}
