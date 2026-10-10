package xray

import "sort"

// MonitoredEndpoint combines metadata from local config with StatsService bytes.
// HasCounters distinguishes a configured listener with no counters from zero traffic.
type MonitoredEndpoint struct {
	Endpoint
	Traffic
	HasCounters bool
}

// JoinEndpoints keeps config-only entries and stats-only entries. It never
// guesses a protocol from a tag, socket, port, or API transport.
func JoinEndpoints(endpoints []Endpoint, rows []Traffic) []MonitoredEndpoint {
	byTag := make(map[string]Traffic, len(rows))
	for _, row := range rows {
		byTag[row.Name] = row
	}
	seen := make(map[string]bool, len(endpoints))
	joined := make([]MonitoredEndpoint, 0, len(endpoints)+len(rows))
	for _, endpoint := range endpoints {
		item := MonitoredEndpoint{Endpoint: endpoint, Traffic: Traffic{Name: endpoint.Tag}}
		if endpoint.Tag != "" {
			if row, ok := byTag[endpoint.Tag]; ok {
				item.Traffic, item.HasCounters = row, true
				seen[endpoint.Tag] = true
			}
		}
		joined = append(joined, item)
	}
	for _, row := range rows {
		if !seen[row.Name] {
			joined = append(joined, MonitoredEndpoint{
				Endpoint: Endpoint{Tag: row.Name, Protocol: "Unknown", Transport: "Unknown", Security: "-", Listen: "-"},
				Traffic:  row, HasCounters: true,
			})
		}
	}
	sort.SliceStable(joined, func(i, j int) bool {
		a, b := joined[i], joined[j]
		aa, bb := a.Uplink+a.Downlink, b.Uplink+b.Downlink
		if aa != bb {
			return aa > bb
		}
		return a.Tag < b.Tag
	})
	return joined
}

type Aggregate struct {
	Name             string
	Endpoints        int
	WithCounters     int
	Uplink, Downlink uint64
	UpRate, DownRate float64
}

// AggregateBy groups inbound counters by proxy protocol OR transport.
// Summing the same inbound in both categories is for alternative breakdowns;
// callers must never add protocol + transport totals together.
func AggregateBy(rows []MonitoredEndpoint, key string) []Aggregate {
	groups := make(map[string]*Aggregate)
	for _, item := range rows {
		name := item.Transport
		if key == "protocol" {
			name = item.Protocol
		}
		if name == "" {
			name = "Unknown"
		}
		group := groups[name]
		if group == nil {
			group = &Aggregate{Name: name}
			groups[name] = group
		}
		group.Endpoints++
		if item.HasCounters {
			group.WithCounters++
			group.Uplink += item.Uplink
			group.Downlink += item.Downlink
			group.UpRate += item.UpRate
			group.DownRate += item.DownRate
		}
	}
	out := make([]Aggregate, 0, len(groups))
	for _, group := range groups {
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool {
		ai, aj := out[i].Uplink+out[i].Downlink, out[j].Uplink+out[j].Downlink
		if ai != aj {
			return ai > aj
		}
		return out[i].Name < out[j].Name
	})
	return out
}
