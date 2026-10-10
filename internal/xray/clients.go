package xray

import (
	"sort"
	"strings"
	"time"
)

// SourceClient is a group of accepted journal events keyed only by the source
// IP reported by Xray. A public IP may represent multiple users (NAT), and
// accepted events are not equivalent to concurrent or authenticated sessions.
type SourceClient struct {
	IP         string
	Inbounds   []string
	Transports []string
	Accepted   int
	FirstSeen  time.Time
	LastSeen   time.Time
}

// GroupSourcesByIP consolidates all inbound tags for an IP, keeping traffic
// transport names as metadata only. No user email, UUID or secret is used.
func GroupSourcesByIP(sources []RecentSource, catalog Catalog) []SourceClient {
	transportByTag := make(map[string]string, len(catalog.Inbounds))
	for _, in := range catalog.Inbounds {
		if in.Tag != "" && in.Transport != "" {
			transportByTag[in.Tag] = in.Transport
		}
	}
	groups := make(map[string]*SourceClient, len(sources))
	inboundSets := make(map[string]map[string]struct{})
	transportSets := make(map[string]map[string]struct{})
	for _, source := range sources {
		if source.IP == "" || source.Accepted <= 0 {
			continue
		}
		g, ok := groups[source.IP]
		if !ok {
			g = &SourceClient{IP: source.IP}
			groups[source.IP] = g
			inboundSets[source.IP] = make(map[string]struct{})
			transportSets[source.IP] = make(map[string]struct{})
		}
		g.Accepted += source.Accepted
		if g.FirstSeen.IsZero() || (!source.FirstSeen.IsZero() && source.FirstSeen.Before(g.FirstSeen)) {
			g.FirstSeen = source.FirstSeen
		}
		if source.LastSeen.After(g.LastSeen) {
			g.LastSeen = source.LastSeen
		}
		if source.Inbound != "" {
			inboundSets[source.IP][source.Inbound] = struct{}{}
			if tr, ok := transportByTag[source.Inbound]; ok && tr != "-" {
				transportSets[source.IP][tr] = struct{}{}
			}
		}
	}
	out := make([]SourceClient, 0, len(groups))
	for _, group := range groups {
		for tag := range inboundSets[group.IP] {
			group.Inbounds = append(group.Inbounds, tag)
		}
		for transport := range transportSets[group.IP] {
			group.Transports = append(group.Transports, transport)
		}
		sort.Strings(group.Inbounds)
		sort.Strings(group.Transports)
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].LastSeen.Equal(out[j].LastSeen) {
			return out[i].LastSeen.After(out[j].LastSeen)
		}
		if out[i].Accepted != out[j].Accepted {
			return out[i].Accepted > out[j].Accepted
		}
		return strings.Compare(out[i].IP, out[j].IP) < 0
	})
	return out
}
