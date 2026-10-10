package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Accelerator6666/xraytop/internal/collector"
	"github.com/Accelerator6666/xraytop/internal/xray"
)

type History struct{ Up, Down []float64 }

func (h *History) Add(up, down float64) {
	const limit = 120
	h.Up = append(h.Up, up)
	h.Down = append(h.Down, down)
	if len(h.Up) > limit {
		h.Up = h.Up[len(h.Up)-limit:]
	}
	if len(h.Down) > limit {
		h.Down = h.Down[len(h.Down)-limit:]
	}
}

type Screen struct {
	Snapshot      collector.Snapshot
	History       History
	View          int
	Width, Height int
	Color         bool
}

const (
	colorOff = "\x1b[0m"
	cyan     = "\x1b[36m"
	yellow   = "\x1b[33m"
	bold     = "\x1b[1m"
)

func (s Screen) Paint() string {
	width := s.Width
	if width < 40 {
		width = 40
	}
	if width > 180 {
		width = 180
	}
	inner := width - 2
	height := s.Height
	if height < 13 {
		height = 13
	}
	var lines []string
	add := func(line string) { lines = append(lines, truncate(line, inner)) }
	ruler := strings.Repeat("─", inner)
	add(s.style(" XrayTop", bold+cyan) + "   local multi-protocol monitor  |  " + s.Snapshot.At.Format("15:04:05"))
	add(ruler)
	status := "RUNNING"
	if s.Snapshot.PID == 0 {
		status = "NOT FOUND"
	}
	add(fmt.Sprintf(" Xray: %-11s  PID: %-7d  instances: %-3d  CPU: %5.1f%%  RSS: %s", status, s.Snapshot.PID, s.Snapshot.ProcessCount, s.Snapshot.CPU, xray.FormatBytes(s.Snapshot.RSS)))
	apiState := "DISABLED (--api '')"
	if s.Snapshot.APIEnabled {
		apiState = "ONLINE"
		if s.Snapshot.APIError != "" {
			apiState = "UNAVAILABLE"
		}
	}
	add(fmt.Sprintf(" StatsService: %s  |  config: %d inbounds / %d outbounds", apiState, len(s.Snapshot.Catalog.Inbounds), len(s.Snapshot.Catalog.Outbounds)))
	if s.Snapshot.APIError != "" {
		add(s.style(" API warning: "+s.Snapshot.APIError, yellow))
	}
	if s.Snapshot.ConfigError != "" {
		add(s.style(" Config warning: "+s.Snapshot.ConfigError, yellow))
	}
	add(ruler)
	switch s.View {
	case 0:
		add(s.style(" TRAFFIC  (inbound counters; not duplicated with outbound)", bold))
		if !s.Snapshot.Traffic.HasRates {
			add(" Up: --/s     Down: --/s     (waiting for second API sample)")
		} else {
			add(fmt.Sprintf(" Up: %-16s Down: %-16s", xray.FormatRate(s.Snapshot.Traffic.UpRate), xray.FormatRate(s.Snapshot.Traffic.DownRate)))
		}
		add(" Up   " + spark(s.History.Up, max(4, inner-10)))
		add(" Down " + spark(s.History.Down, max(4, inner-10)))
		add(fmt.Sprintf(" Inbound total: up %s | down %s", xray.FormatBytes(s.Snapshot.Traffic.UpTotal), xray.FormatBytes(s.Snapshot.Traffic.DownTotal)))
		add(ruler)
		add(s.style(" TOP INBOUNDS  (proxy / transport / security)", bold))
		add(endpointHeader(inner))
		addEndpointRows(&lines, s.Snapshot.Inbounds, max(1, min(10, height-len(lines)-4)), inner, false)
	case 1, 2:
		rows, title := s.Snapshot.Inbounds, "INBOUNDS"
		if s.View == 2 {
			rows = s.Snapshot.Outbounds
			title = "OUTBOUNDS"
		}
		add(s.style(" "+title+"  (all configured tags, plus stats-only tags)", bold))
		add(endpointHeader(inner))
		addEndpointRows(&lines, rows, max(1, height-len(lines)-4), inner, s.View == 1)
	case 6:
		add(s.style(" RECENT ACCEPTED SOURCE IPs  (last 5 minutes; NOT live sessions)", bold))
		add(" Observed IP                              Inbound tag                     Accepted   Last seen")
		if s.Snapshot.RecentError != "" {
			add(s.style(" Journal: "+s.Snapshot.RecentError, yellow))
		} else if s.Snapshot.Recent.CheckedAt.IsZero() {
			add(" Recent-source journal collector disabled or waiting for first sample.")
		} else if len(s.Snapshot.Recent.Sources) == 0 {
			add(" No recent accepted log entries. This does NOT prove no clients connected.")
			add(" Enable Xray access logs in journald or pass --journal-unit <unit>.")
		} else {
			for i, row := range s.Snapshot.Recent.Sources {
				if i >= max(1, height-len(lines)-5) {
					break
				}
				add(fmt.Sprintf(" %-40s %-30s %8d   %s", truncate(row.IP, 40), truncate(row.Inbound, 30), row.Accepted, row.LastSeen.Local().Format("15:04:05")))
			}
			add(fmt.Sprintf(" %d accepted events / %d source+inbound groups (journal, last 5m)", s.Snapshot.Recent.Events, len(s.Snapshot.Recent.Sources)))
			add(" Recent activity is not online status; a single IP may represent multiple users.")
		}
	case 5:
		add(s.style(" ONLINE USERS / SOURCE IP  (Xray live online map)", bold))
		add(" User (client email)                         Source IP                 Last activity")
		if !s.Snapshot.APIEnabled {
			add(" StatsService disabled; user/IP data unavailable.")
		} else if s.Snapshot.OnlineError != "" {
			add(s.style(" Online API: "+s.Snapshot.OnlineError, yellow))
		} else if len(s.Snapshot.Online.Users) == 0 {
			add(" No Xray online-map entries. This does NOT prove no clients connected.")
			if !s.Snapshot.Catalog.OnlinePolicyConfigured {
				add(" Hint: enable policy.levels.<level>.statsUserOnline in Xray config.")
			}
			add(" Hint: assign a unique email to each Xray inbound client.")
			add(" Reverse proxies must pass the real client IP to Xray.")
		} else {
			remaining := max(1, height-len(lines)-6)
			count := 0
			for _, user := range s.Snapshot.Online.Users {
				for j, ip := range user.IPs {
					if count >= remaining {
						break
					}
					name := "  ↳"
					if j == 0 {
						name = safeUser(user.Email)
					}
					addr := ip.Address
					if ip.IsLocal {
						addr += " (proxy/local)"
					}
					age := "-"
					if !ip.LastSeen.IsZero() {
						ago := s.Snapshot.At.Sub(ip.LastSeen)
						if ago < 0 {
							ago = 0
						}
						age = fmt.Sprintf("%s (%s ago)", ip.LastSeen.Local().Format("15:04:05"), ago.Round(time.Second))
					}
					add(fmt.Sprintf(" %-40s %-28s %s", truncate(name, 40), truncate(addr, 28), age))
					count++
				}
				if count >= remaining {
					break
				}
			}
			if s.Snapshot.Online.Failed > 0 || s.Snapshot.Online.Skipped > 0 {
				add(fmt.Sprintf(" Partial data: %d per-user queries failed, %d users skipped", s.Snapshot.Online.Failed, s.Snapshot.Online.Skipped))
			}
			add(" Xray-reported source IP; reverse proxies may mask the real address.")
		}
	case 3, 4:
		key, title := "protocol", "PROXY PROTOCOLS"
		if s.View == 4 {
			key, title = "TRANSPORTS", "TRANSPORT METHODS"
		}
		add(s.style(" "+title+"  (inbound-only; no outbound double count)", bold))
		add(groupHeader(inner))
		groups := xray.AggregateBy(s.Snapshot.Inbounds, key)
		addGroupRows(&lines, groups, max(1, height-len(lines)-4), inner, s.Snapshot.Traffic.HasRates)
	}
	for len(lines) < height-2 {
		add("")
	}
	if len(lines) > height-2 {
		lines = lines[:height-2]
	}
	add(ruler)
	footer := " [1] Overview [2] In [3] Out [4] Protocols [5] Transports [6] Users [7] Recent IPs [q] Quit"
	if inner < 100 {
		footer = " [1]Home [2]In [3]Out [4]Proto [5]Trans [6]Users [7]IPs [q] Quit"
	}
	add(footer)
	if s.Color {
		return "\x1b[H" + strings.Join(lines, "\x1b[K\r\n") + "\x1b[J"
	}
	return strings.Join(lines, "\n")
}

func endpointHeader(width int) string {
	if width < 87 {
		return " TAG / PROTOCOL / TRANSPORT / SEC                 UP           DOWN"
	}
	return fmt.Sprintf(" %-*s %-12s %-14s %-9s %10s %10s", max(12, width-66), "TAG", "PROTOCOL", "TRANSPORT", "SECURITY", "UP", "DOWN")
}
func addEndpointRows(lines *[]string, rows []xray.MonitoredEndpoint, limit, width int, showListen bool) {
	if len(rows) == 0 {
		*lines = append(*lines, " No inbounds/outbounds found. Use --config or --config-dir for metadata.")
		return
	}
	for i, row := range rows {
		if i >= limit {
			break
		}
		up, down := "-", "-"
		if row.HasCounters {
			up, down = xray.FormatBytes(row.Uplink), xray.FormatBytes(row.Downlink)
		}
		tag := row.Tag
		if tag == "" {
			tag = "(untagged)"
		}
		if width < 87 {
			// Compact terminals: show metadata on one line, counters on another.
			*lines = append(*lines, truncate(" "+tag+"  "+row.Protocol+" / "+row.Transport+" / "+row.Security, width))
			*lines = append(*lines, truncate("   "+row.Listen+"  up "+up+" / down "+down, width))
			continue
		}
		nameWidth := max(12, width-66)
		name := truncate(tag, nameWidth)
		line := fmt.Sprintf(" %-*s %-12s %-14s %-9s %10s %10s", nameWidth, name, truncate(row.Protocol, 12), truncate(row.Transport, 14), truncate(row.Security, 9), up, down)
		*lines = append(*lines, truncate(line, width))
		// Only metadata is read from Xray config. No UUIDs, passwords or keys.
		if showListen && row.Listen != "" && row.Listen != "-" {
			*lines = append(*lines, truncate("   listen: "+row.Listen, width))
		}
	}
}
func groupHeader(width int) string {
	if width < 84 {
		return fmt.Sprintf(" %-*s %5s %5s %11s %11s", max(9, width-40), "GROUP", "TAGS", "STATS", "UP/s", "DOWN/s")
	}
	return fmt.Sprintf(" %-*s %5s %5s %11s %11s %11s %11s", max(9, width-64), "GROUP", "TAGS", "STATS", "UP/s", "DOWN/s", "UP TOTAL", "DOWN TOTAL")
}
func addGroupRows(lines *[]string, groups []xray.Aggregate, limit, width int, hasRates bool) {
	if len(groups) == 0 {
		*lines = append(*lines, " No protocol/transport information. Supply --config or --config-dir.")
		return
	}
	for i, group := range groups {
		if i >= limit {
			break
		}
		upTotal, downTotal, upRate, downRate := "-", "-", "--/s", "--/s"
		if group.WithCounters > 0 {
			upTotal, downTotal = xray.FormatBytes(group.Uplink), xray.FormatBytes(group.Downlink)
			if hasRates {
				upRate, downRate = xray.FormatRate(group.UpRate), xray.FormatRate(group.DownRate)
			}
		}
		if width < 84 {
			nameWidth := max(9, width-40)
			*lines = append(*lines, truncate(fmt.Sprintf(" %-*s %5d %5d %11s %11s", nameWidth, truncate(group.Name, nameWidth), group.Endpoints, group.WithCounters, upRate, downRate), width))
			continue
		}
		nameWidth := max(9, width-64)
		*lines = append(*lines, truncate(fmt.Sprintf(" %-*s %5d %5d %11s %11s %11s %11s", nameWidth, truncate(group.Name, nameWidth), group.Endpoints, group.WithCounters, upRate, downRate, upTotal, downTotal), width))
	}
}
func truncate(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	if maxRunes < 2 {
		return "…"
	}
	return string([]rune(s)[:maxRunes-1]) + "…"
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func (s Screen) style(text, style string) string {
	if !s.Color {
		return text
	}
	return style + text + colorOff
}
func spark(values []float64, width int) string {
	if len(values) == 0 {
		return strings.Repeat("·", width)
	}
	if width > len(values) {
		width = len(values)
	}
	values = values[len(values)-width:]
	var peak float64
	for _, v := range values {
		if v > peak {
			peak = v
		}
	}
	const bars = "▁▂▃▄▅▆▇█"
	var b strings.Builder
	if peak <= 0 {
		return strings.Repeat("▁", width)
	}
	for _, v := range values {
		n := int(v / peak * 7)
		if n < 0 {
			n = 0
		}
		if n > 7 {
			n = 7
		}
		b.WriteRune([]rune(bars)[n])
	}
	return b.String()
}

// safeUser prevents control sequences in untrusted email metadata from being
// interpreted as terminal commands. Only printable characters are displayed.
func safeUser(email string) string {
	var b strings.Builder
	for _, r := range email {
		if unicode.IsControl(r) || r == '\x1b' || r == '\u009b' {
			b.WriteRune('?')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
