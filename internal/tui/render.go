package tui

import (
	"fmt"
	"strings"
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
	green    = "\x1b[32m"
	red      = "\x1b[31m"
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
	add(s.style(" XrayTop", bold+cyan) + "   local Xray monitor  |  " + s.Snapshot.At.Format("15:04:05"))
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
	add(" StatsService: " + apiState)
	if s.Snapshot.APIError != "" {
		add(s.style(" Warning: "+s.Snapshot.APIError, yellow))
	}
	add(ruler)
	if s.View == 0 {
		add(s.style(" TRAFFIC  (inbound counters; total not duplicated with outbound)", bold))
		if !s.Snapshot.Traffic.HasRates {
			add(" Up: --/s     Down: --/s     (waiting for second API sample)")
		} else {
			add(fmt.Sprintf(" Up: %-16s Down: %-16s", xray.FormatRate(s.Snapshot.Traffic.UpRate), xray.FormatRate(s.Snapshot.Traffic.DownRate)))
		}
		add(" Up   " + spark(s.History.Up, max(4, inner-10)))
		add(" Down " + spark(s.History.Down, max(4, inner-10)))
		add(fmt.Sprintf(" Inbound cumulative: up %s  |  down %s", xray.FormatBytes(s.Snapshot.Traffic.UpTotal), xray.FormatBytes(s.Snapshot.Traffic.DownTotal)))
		add(ruler)
		add(s.style(" TOP INBOUNDS  (cumulative inbound traffic)", bold))
		add(" TAG" + spaces(max(1, inner-41)) + "UPLINK       DOWNLINK")
		maxRows := max(1, height-len(lines)-5)
		if maxRows > 10 {
			maxRows = 10
		}
		addTrafficRows(&lines, s.Snapshot.Traffic.Inbound, maxRows, inner)
	} else {
		label := "INBOUNDS"
		rows := s.Snapshot.Traffic.Inbound
		if s.View == 2 {
			label = "OUTBOUNDS"
			rows = s.Snapshot.Traffic.Outbound
		}
		add(s.style(" "+label+"  (cumulative bytes, read-only)", bold))
		add(" TAG" + spaces(max(1, inner-41)) + "UPLINK       DOWNLINK")
		maxRows := max(1, height-len(lines)-5)
		addTrafficRows(&lines, rows, maxRows, inner)
	}
	for len(lines) < height-2 {
		add("")
	}
	if len(lines) > height-2 {
		lines = lines[:height-2]
	}
	add(ruler)
	add(" [1] Overview  [2] Inbounds  [3] Outbounds  [r] Refresh  [q] Quit")
	if s.Color {
		return "\x1b[H" + strings.Join(lines, "\x1b[K\r\n") + "\x1b[J"
	}
	return strings.Join(lines, "\n")
}

func addTrafficRows(lines *[]string, rows []xray.Traffic, limit, width int) {
	if len(rows) == 0 {
		*lines = append(*lines, " No traffic counters. Enable stats + inbound/outbound policy in Xray.")
		return
	}
	tagWidth := max(12, width-29)
	for i, row := range rows {
		if i >= limit {
			break
		}
		name := truncate(" "+row.Name, tagWidth)
		*lines = append(*lines, fmt.Sprintf("%-*s %-12s %-12s", tagWidth, name, xray.FormatBytes(row.Uplink), xray.FormatBytes(row.Downlink)))
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
func spaces(n int) string { return strings.Repeat(" ", n) }
func max(a, b int) int {
	if a > b {
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
