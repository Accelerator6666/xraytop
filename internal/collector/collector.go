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
	Recent       xray.RecentActivity
	RecentError  string
	Traffic      xray.Breakdown
	Catalog      xray.Catalog
	ConfigError  string
	Inbounds     []xray.MonitoredEndpoint
	Outbounds    []xray.MonitoredEndpoint
}

type Collector struct {
	ProcRoot    string
	ConfigFiles []string
	ConfigDir   string
	Client      xray.Client
	JournalUnit string
	sampler     xray.Sampler
	previous    system.CPUSample
	lastTraffic xray.Breakdown
	recentCache xray.RecentActivity
	recentError string
	nextRecent  time.Time
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
	// Only use explicitly supplied local config paths, or discover the paths
	// from the running Xray process. Never infer a proxy protocol from a tag.
	paths := append([]string(nil), c.ConfigFiles...)
	dir := c.ConfigDir
	if len(paths) == 0 && dir == "" && snap.PID != 0 {
		var e error
		paths, dir, e = xray.DiscoverConfigPaths(root, snap.PID)
		if e != nil {
			snap.ConfigError = "discover config: " + e.Error()
		}
	}
	if len(paths) > 0 || dir != "" {
		var e error
		snap.Catalog, e = xray.LoadCatalog(paths, dir)
		if e != nil {
			snap.ConfigError = e.Error()
		}
	}
	snap.Traffic = c.lastTraffic
	if snap.APIEnabled {
		stats, e := c.Client.Query(ctx)
		if e != nil {
			snap.APIError = e.Error()
			// Keep last cumulative counters for context but invalidate live rates.
			snap.Traffic.HasRates = false
			snap.Traffic.UpRate, snap.Traffic.DownRate = 0, 0
			for i := range snap.Traffic.Inbound {
				snap.Traffic.Inbound[i].UpRate, snap.Traffic.Inbound[i].DownRate = 0, 0
			}
			for i := range snap.Traffic.Outbound {
				snap.Traffic.Outbound[i].UpRate, snap.Traffic.Outbound[i].DownRate = 0, 0
			}
		} else {
			snap.Traffic = c.sampler.Update(time.Now(), stats)
			c.lastTraffic = snap.Traffic
		}
	}
	// IP-first monitoring needs no user email and does not query online-user RPCs.
	// The source-IP journal cache refreshes independently of StatsService.
	if c.JournalUnit != "" && !now.Before(c.nextRecent) && ctx.Err() == nil {
		recent, err := xray.ReadRecentActivity(ctx, c.JournalUnit)
		if err == nil {
			c.recentCache, c.recentError = recent, ""
		} else {
			c.recentCache, c.recentError = xray.RecentActivity{}, err.Error()
		}
		c.nextRecent = time.Now().Add(10 * time.Second)
	}
	snap.Recent, snap.RecentError = c.recentCache, c.recentError
	snap.Inbounds = xray.JoinEndpoints(snap.Catalog.Inbounds, snap.Traffic.Inbound)
	snap.Outbounds = xray.JoinEndpoints(snap.Catalog.Outbounds, snap.Traffic.Outbound)
	return snap
}
