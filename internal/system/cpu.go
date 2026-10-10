package system

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// CPUSample contains monotonically increasing kernel ticks from procfs.
type CPUSample struct {
	Total   uint64
	Process uint64
	PID     int
}

// ReadCPUSample returns process and system ticks. It correctly handles names
// containing whitespace and parentheses in /proc/PID/stat.
func ReadCPUSample(procRoot string, pid int) (CPUSample, error) {
	if pid <= 0 {
		return CPUSample{}, fmt.Errorf("invalid pid %d", pid)
	}
	b, err := os.ReadFile(filepath.Join(procRoot, "stat"))
	if err != nil {
		return CPUSample{}, err
	}
	var total uint64
	lines := strings.SplitN(string(b), "\n", 2)
	parts := strings.Fields(lines[0])
	if len(parts) < 5 || parts[0] != "cpu" {
		return CPUSample{}, fmt.Errorf("unexpected /proc/stat")
	}
	for _, v := range parts[1:] {
		n, e := strconv.ParseUint(v, 10, 64)
		if e != nil {
			return CPUSample{}, fmt.Errorf("invalid CPU ticks: %w", e)
		}
		total += n
	}
	b, err = os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return CPUSample{}, err
	}
	suffix := string(b)
	closeParen := strings.LastIndex(suffix, ")")
	if closeParen < 0 {
		return CPUSample{}, fmt.Errorf("malformed process stat")
	}
	// fields[0] is state (field 3 of /proc/PID/stat).
	proc := strings.Fields(suffix[closeParen+1:])
	if len(proc) <= 12 {
		return CPUSample{}, fmt.Errorf("short process stat")
	}
	utime, e1 := strconv.ParseUint(proc[11], 10, 64)
	stime, e2 := strconv.ParseUint(proc[12], 10, 64)
	if e1 != nil || e2 != nil {
		return CPUSample{}, fmt.Errorf("invalid process ticks")
	}
	return CPUSample{Total: total, Process: utime + stime, PID: pid}, nil
}

// ProcessCPUPercent reports percent of one logical CPU, as conventional tools do.
// It may exceed 100% for multi-threaded processes.
func ProcessCPUPercent(previous, current CPUSample) float64 {
	if current.PID <= 0 || current.PID != previous.PID || current.Total <= previous.Total || current.Process < previous.Process {
		return 0
	}
	ncpu := runtime.NumCPU()
	if ncpu < 1 {
		ncpu = 1
	}
	return float64(current.Process-previous.Process) / float64(current.Total-previous.Total) * float64(ncpu) * 100
}
