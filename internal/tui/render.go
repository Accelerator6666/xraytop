package tui

import (
	"fmt"
	"strings"
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
	ClientFilter string
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

	case 5:
		add(s.style(" CLIENTS BY SOURCE IP  (Xray accepted / NGINX HTTP / TCP evidence, not identities)", bold))
		add(" Source IP                               Type  TCP   Xray HTTP  TCP ports         Xray inbound / HTTP route      Seen")
		if !s.Snapshot.Nginx.CheckedAt.IsZero() {
			add(fmt.Sprintf(" NGINX auto: %d verified proxy ports / %d Xray inbound routes", len(s.Snapshot.Nginx.Ports), len(s.Snapshot.Nginx.Routes)))
		}
		if s.Snapshot.NginxError != "" { add(s.style(" NGINX discovery: "+s.Snapshot.NginxError, yellow)) }
		if s.Snapshot.LiveError != "" { add(s.style(" TCP warning: "+s.Snapshot.LiveError,yellow)) }
		if s.Snapshot.RecentError != "" { add(s.style(" Journal warning: "+s.Snapshot.RecentError,yellow)) }
		if s.Snapshot.AccessError != "" { add(s.style(" NGINX access: "+s.Snapshot.AccessError, yellow)) }
		if len(s.Snapshot.Live.Ports)==0 { add(" No TCP ingress ports selected. Check NGINX auto discovery or use --ingress-ports.") }
		filter:=s.ClientFilter
		if filter=="" { filter="all" }
		clients:=xray.MergeObservedClientsWithHTTP(s.Snapshot.Recent.Sources,s.Snapshot.Catalog,s.Snapshot.Live,s.Snapshot.Access)
		xrayCount,httpCount,tcpCount:=0,0,0
		for _,client:=range clients {
			switch client.Evidence() {
			case "XRAY": xrayCount++
			case "HTTP": httpCount++
			case "TCP": tcpCount++
			}
		}
		add(fmt.Sprintf(" Filter: %s (press f: all -> xray -> http-only -> tcp-only)  Sources: XRAY %d / HTTP %d / TCP %d", filter,xrayCount,httpCount,tcpCount))
		shown:=0
		limit:=max(1,height-len(lines)-7)
		for _,client:=range clients {
			if !xray.MatchObservedFilter(client,filter) {continue}
			if shown>=limit {break}
			meta:=strings.Join(client.Inbounds,",")
			if len(client.Transports)>0 {meta+=" ("+strings.Join(client.Transports,",")+")"}
			if meta=="" && len(client.HTTPRoutes)>0 {meta="HTTP:"+strings.Join(client.HTTPRoutes,",")}
			if meta=="" {meta="-"}
			lastSeen:=client.LastSeen
			if client.HTTPLastSeen.After(lastSeen) {lastSeen=client.HTTPLastSeen}
			last:="-"
			if !lastSeen.IsZero() {last=lastSeen.Local().Format("15:04:05")}
			var ports []string
			for _,p:=range client.Ports {ports=append(ports,fmt.Sprint(p))}
			portText:=strings.Join(ports,",")
			if portText=="" {portText="-"}
			add(fmt.Sprintf(" %-39s %-4s %4d %6d %4d  %-17s %-29s %s",truncate(client.IP,39),client.Evidence(),client.TCPConnections,client.Accepted,client.HTTPRequests,truncate(portText,17),truncate(meta,29),last))
			shown++
		}
		if shown==0 {add(" No observed source IPs match this filter. This does not prove a client is offline.")}
		add(fmt.Sprintf(" %d observed IPs | %d TCP ESTABLISHED sockets | %d accepted journal events | %d matched NGINX HTTP requests",len(clients),s.Snapshot.Live.Connections,s.Snapshot.Recent.Events,s.Snapshot.Access.Matched))
		add(" XRAY=accepted proxy activity; HTTP=matched NGINX request only; TCP=network socket only.")
		add(" TCP counts ingress network sockets, NOT Xray user sessions; source IP is not unique identity.")
	case 6:
		add(s.style(" SOURCE IP + INBOUND  (5-minute accepted activity; NOT online status)", bold))
		add(" Source IP                                Inbound tag                     Events   TCP    Last seen")
		if s.Snapshot.RecentError != "" {
			add(s.style(" Journal: "+s.Snapshot.RecentError, yellow))
		} else if s.Snapshot.Recent.CheckedAt.IsZero() {
			add(" Journal source-IP collector disabled or waiting for first sample.")
		} else if len(s.Snapshot.Recent.Sources) == 0 {
			add(" No recent accepted log entries. This does NOT prove no clients connected.")
			add(" Check journalctl -u xray, or use --journal-unit <unit>.")
		} else {
			for i, row := range s.Snapshot.Recent.Sources {
				if i >= max(1, height-len(lines)-5) {
					break
				}
				tcpCount:=0
				for _,peer:=range s.Snapshot.Live.Peers { if peer.IP==row.IP {tcpCount=peer.Connections;break} }
				add(fmt.Sprintf(" %-40s %-30s %8d %5d   %s",truncate(row.IP,40),truncate(row.Inbound,30),row.Accepted,tcpCount,row.LastSeen.Local().Format("15:04:05")))
			}
			add(fmt.Sprintf(" %d accepted events / %d IP+inbound groups (last 5m)", s.Snapshot.Recent.Events, len(s.Snapshot.Recent.Sources)))
			add(" TCP is a per-IP host ingress socket count, NOT an authenticated per-Inbound session count.")
		}
	case 7:
		add(s.style(" NGINX PROXY HTTP REQUESTS  (last 5 min, matched URL locations; NOT auth)", bold))
		add(" Source IP                               Inbound / transport           HTTP   101/200   Last seen")
		if s.Snapshot.AccessError != "" {add(s.style(" NGINX access: "+s.Snapshot.AccessError, yellow))}
		if s.Snapshot.Access.CheckedAt.IsZero() {add(" No NGINX access sample. Enable --nginx-auto and readable --nginx-access-log.")}
		if len(s.Snapshot.Access.Requests)==0 {add(" No recent requests matched a uniquely identified Xray location.")}
		limit:=max(1,height-len(lines)-6)
		for i,req:=range s.Snapshot.Access.Requests {
			if i>=limit {break}
			meta:=req.Inbound+" / "+req.Transport
			add(fmt.Sprintf(" %-39s %-29s %5d %8d   %s",truncate(req.IP,39),truncate(meta,29),req.Requests,req.SuccessHTTP,req.LastSeen.Local().Format("15:04:05")))
		}
		add(fmt.Sprintf(" Matched %d / %d recent HTTP requests (last 2 MiB of access.log)",s.Snapshot.Access.Matched,s.Snapshot.Access.Total))
		if s.Snapshot.Access.Truncated {add(" Bounded log tail: earlier requests may be omitted, even within the 5-min window.")}
		add(" 101/200 means successful HTTP handshake/response, NOT an authenticated VLESS session.")
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
	footer := " [1] Overview [2] In [3] Out [4] Protocols [5] Transports [6] IP Clients [7] IP+Inbound [8] NGINX [f] Filter [q] Quit"
	if inner < 100 {
		footer = " [1]Home [2]In [3]Out [4]Proto [5]Trans [6]Clients [7]IPs [8]HTTP [f]Filter [q] Quit"
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
