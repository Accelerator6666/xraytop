package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Stats represents cumulative traffic counters returned by StatsService.
type Stats map[string]uint64

// Client uses Xray-core's bundled gRPC CLI, without needing to install protoc
// generated sources or import Xray-core as a dependency. No shells are used.
type Client struct {
	Binary string
	Server string
}

func (c Client) Query(ctx context.Context) (Stats, error) {
	if c.Server == "" {
		return nil, errors.New("Xray API disabled")
	}
	if err := validateLocalAddress(c.Server); err != nil {
		return nil, err
	}
	binary := c.Binary
	if binary == "" {
		binary = "xray"
	}
	cmd := exec.CommandContext(ctx, binary, "api", "statsquery", "-s", c.Server, "-t", "2")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		reason := strings.TrimSpace(stderr.String())
		if len(reason) > 180 {
			reason = reason[:180]
		}
		if reason != "" {
			return nil, fmt.Errorf("statsquery: %s: %w", reason, err)
		}
		return nil, fmt.Errorf("statsquery: %w", err)
	}
	return ParseStats(out.Bytes())
}

func validateLocalAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("API address must be host:port: %w", err)
	}
	if port == "" {
		return errors.New("API port is missing")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return errors.New("invalid API port")
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("only loopback API addresses are allowed (127.0.0.1 / ::1 / localhost)")
	}
	return nil
}

func ParseStats(data []byte) (Stats, error) {
	var response struct {
		Stat []struct {
			Name  string      `json:"name"`
			Value json.Number `json:"value"`
		} `json:"stat"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&response); err != nil {
		return nil, fmt.Errorf("decode stats: %w", err)
	}
	result := make(Stats, len(response.Stat))
	for _, s := range response.Stat {
		// Proto3 JSON omits zero-valued scalars. Xray statsquery may return
		// a counter with only its name. Treat absent or null value as zero.
		if s.Value == "" {
			result[s.Name] = 0
			continue
		}
		value, err := strconv.ParseInt(s.Value.String(), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid counter for %q: %w", s.Name, err)
		}
		if value < 0 {
			continue
		}
		result[s.Name] = uint64(value)
	}
	return result, nil
}

type Traffic struct {
	Name             string
	Uplink, Downlink uint64
	UpRate, DownRate float64
}
type Breakdown struct {
	Inbound, Outbound  []Traffic
	UpRate, DownRate   float64
	UpTotal, DownTotal uint64
	HasRates           bool
}

// Sampler compares cumulative counters without resetting them: XrayTop never
// interferes with other StatsService consumers. Rates are derived from inbound
// counters only, avoiding double counting inbound/outbound for the same traffic.
type Sampler struct {
	previous Stats
	at       time.Time
}

func (s *Sampler) Update(now time.Time, stats Stats) Breakdown {
	result := Breakdown{}
	elapsed := now.Sub(s.at).Seconds()
	result.HasRates = s.previous != nil && elapsed > 0
	inbound := map[string]*Traffic{}
	outbound := map[string]*Traffic{}
	for name, value := range stats {
		parts := strings.Split(name, ">>>")
		if len(parts) != 4 || parts[2] != "traffic" {
			continue
		}
		var group map[string]*Traffic
		switch parts[0] {
		case "inbound":
			group = inbound
		case "outbound":
			group = outbound
		default:
			continue
		}
		item := group[parts[1]]
		if item == nil {
			item = &Traffic{Name: parts[1]}
			group[parts[1]] = item
		}
		var rate float64
		if old, ok := s.previous[name]; result.HasRates && ok && value >= old {
			rate = float64(value-old) / elapsed
		}
		switch parts[3] {
		case "uplink":
			item.Uplink = value
			item.UpRate = rate
		case "downlink":
			item.Downlink = value
			item.DownRate = rate
		}
	}
	for _, item := range inbound {
		result.Inbound = append(result.Inbound, *item)
		result.UpTotal += item.Uplink
		result.DownTotal += item.Downlink
		result.UpRate += item.UpRate
		result.DownRate += item.DownRate
	}
	for _, item := range outbound {
		result.Outbound = append(result.Outbound, *item)
	}
	sortTraffic(result.Inbound)
	sortTraffic(result.Outbound)
	s.previous = stats
	s.at = now
	return result
}

// Helpers in a separate file are kept deliberately small and dependency-free.
func FormatRate(n float64) string {
	if n < 0 {
		n = 0
	}
	return FormatBytes(uint64(n)) + "/s"
}

func FormatBytes(n uint64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	size := float64(n)
	for _, unit := range []string{"KiB", "MiB", "GiB", "TiB"} {
		size /= 1024
		if size < 1024 || unit == "TiB" {
			return fmt.Sprintf("%.1f %s", size, unit)
		}
	}
	return "?"
}
