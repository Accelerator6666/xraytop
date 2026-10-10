package collector

import (
	"context"
	"time"

	"github.com/Accelerator6666/xraytop/internal/system"
	"github.com/Accelerator6666/xraytop/internal/xray"
)

type Snapshot struct {
	At           time.Time
	PID          int
	ProcessCount int
	RSS          uint64
	CPU          float64
	APIError     string
	APIEnabled   bool
	Traffic      xray.Breakdown
}

type Collector struct {
	ProcRoot    string
	Client      xray.Client
	sampler     xray.Sampler
	previous    system.CPUSample
	lastTraffic xray.Breakdown
}

func (c *Collector) Sample(ctx context.Context) Snapshot {
	now := time.Now()
	snap := Snapshot{At: now, APIEnabled: c.Client.Server != ""}
	root := c.ProcRoot
	if root == "" {
		root = "/proc"
	}
	procs, err := system.FindXrayProcesses(root)
	if err != nil {
		snap.APIError = "procfs: " + err.Error()
	}
	snap.ProcessCount = len(procs)
	if len(procs) > 0 {
		p := procs[0]
		snap.PID = p.PID
		snap.RSS = p.RSSBytes
		current, e := system.ReadCPUSample(root, p.PID)
		if e == nil {
			snap.CPU = system.ProcessCPUPercent(c.previous, current)
			c.previous = current
		}
	} else {
		c.previous = system.CPUSample{}
	}
	snap.Traffic = c.lastTraffic
	if snap.APIEnabled {
		stats, e := c.Client.Query(ctx)
		if e != nil {
			snap.APIError = e.Error()
		} else {
			snap.Traffic = c.sampler.Update(time.Now(), stats)
			c.lastTraffic = snap.Traffic
		}
	}
	return snap
}
