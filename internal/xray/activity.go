package xray

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// RecentSource is evidence of recently accepted traffic, NOT a currently
// connected session and NOT a confirmed user identity.
type RecentSource struct {
	IP        string
	Inbound   string
	Accepted  int
	FirstSeen time.Time
	LastSeen  time.Time
}

type RecentActivity struct {
	Sources   []RecentSource
	CheckedAt time.Time
	Events    int
	Truncated bool // upstream journal cursor batch overflow; last five-minute window may be incomplete
}

// ReadRecentActivity reads only systemd journal entries from the local host.
// It never modifies Xray settings or queries a remote address.
func ReadRecentActivity(ctx context.Context, unit string) (RecentActivity, error) {
	result := RecentActivity{CheckedAt: time.Now()}
	if unit == "" {
		return result, errors.New("local journal monitoring disabled")
	}
	if !validJournalUnit(unit) {
		return result, errors.New("invalid local systemd unit name")
	}
	cmd := exec.CommandContext(ctx, "journalctl", "--unit", unit, "--since", "5 minutes ago", "--no-pager", "--output=short-unix", "--lines=500")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		reason := strings.TrimSpace(stderr.String())
		if len(reason) > 160 {
			reason = reason[:160]
		}
		return result, fmt.Errorf("journalctl: %s: %w", reason, err)
	}
	if len(output) > 2<<20 {
		return result, errors.New("journal output exceeds 2 MiB")
	}
	result.Sources, result.Events = ParseRecentActivity(output)
	return result, nil
}

func validJournalUnit(name string) bool {
	if len(name) < 1 || len(name) > 100 || name[0] == '-' {
		return false
	}
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || strings.ContainsRune("-_.@", ch) {
			continue
		}
		return false
	}
	return true
}

// ParseRecentActivity accepts only "accepted" entries carrying a valid source
// IP and inbound tag. Rejected authentication attempts are deliberately excluded.
// There is no attempt to map journal lines to user emails or to infer a live
// socket: that relationship is not present in standard Xray access logs.
func ParseRecentActivity(output []byte) ([]RecentSource, int) {
	grouped := make(map[string]*RecentSource)
	events := 0
	for _, raw := range bytes.Split(output, []byte{'\n'}) {
		line := string(raw)
		first, rest, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		stamp, err := strconv.ParseFloat(first, 64)
		if err != nil || stamp <= 0 {
			continue
		}
		fromIndex := strings.Index(rest, " from ")
		if fromIndex < 0 {
			continue
		}
		endpoint, remaining, ok := strings.Cut(rest[fromIndex+len(" from "):], " accepted ")
		if !ok {
			continue
		}
		host, _, err := net.SplitHostPort(endpoint)
		if err != nil {
			continue
		}
		ip := net.ParseIP(host)
		if ip == nil {
			continue
		}
		tagStart := strings.LastIndex(remaining, " [")
		if tagStart < 0 {
			continue
		}
		tagTail := remaining[tagStart+2:]
		tag, _, ok := strings.Cut(tagTail, " >> ")
		if !ok {
			continue
		}
		tag = strings.TrimSpace(tag)
		if tag == "" || len(tag) > 100 {
			continue
		}
		// Restrict untrusted tags to printable, non-control Unicode.
		if strings.IndexFunc(tag, func(r rune) bool { return unicode.IsControl(r) }) != -1 {
			continue
		}
		key := ip.String() + "\x00" + tag
		seen := time.Unix(int64(stamp), int64((stamp-float64(int64(stamp)))*1e9))
		if e, ok := grouped[key]; ok {
			e.Accepted++
			if e.FirstSeen.IsZero() || seen.Before(e.FirstSeen) {
				e.FirstSeen = seen
			}
			if seen.After(e.LastSeen) {
				e.LastSeen = seen
			}
		} else {
			grouped[key] = &RecentSource{IP: ip.String(), Inbound: tag, Accepted: 1, FirstSeen: seen, LastSeen: seen}
		}
		events++
	}
	out := make([]RecentSource, 0, len(grouped))
	for _, row := range grouped {
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].LastSeen.Equal(out[j].LastSeen) {
			return out[i].LastSeen.After(out[j].LastSeen)
		}
		if out[i].Accepted != out[j].Accepted {
			return out[i].Accepted > out[j].Accepted
		}
		if out[i].IP != out[j].IP {
			return out[i].IP < out[j].IP
		}
		return out[i].Inbound < out[j].Inbound
	})
	return out, events
}
