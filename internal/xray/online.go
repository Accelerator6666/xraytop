package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OnlineIP is an address reported by Xray's per-user online map, not an
// independently verified physical client address. A reverse proxy may mask it.
type OnlineIP struct {
	Address  string
	LastSeen time.Time
	IsLocal  bool
}

type OnlineUser struct {
	Email string
	IPs   []OnlineIP
}

type OnlineSnapshot struct {
	Users     []OnlineUser
	CheckedAt time.Time
	Skipped   int
	Failed    int
}

// QueryOnline reads the live Xray online map without resetting any counters.
// 26.3.27 has GetAllOnlineUsers and GetStatsOnlineIpList, but not the newer
// GetUsersStats RPC. Use bundled Xray CLI for compatibility with this release.
// We limit the number of users, parallel invocations, and execution time.
func (c Client) QueryOnline(ctx context.Context) (OnlineSnapshot, error) {
	result := OnlineSnapshot{CheckedAt: time.Now()}
	if c.Server == "" {
		return result, errors.New("Xray API disabled")
	}
	if err := validateLocalAddress(c.Server); err != nil {
		return result, err
	}
	data, err := c.onlineCommand(ctx, "statsgetallonlineusers")
	if err != nil {
		return result, fmt.Errorf("online users: %w", err)
	}
	users, err := ParseOnlineUsers(data)
	if err != nil {
		return result, fmt.Errorf("decode online users: %w", err)
	}
	const maxUsers = 64
	if len(users) > maxUsers {
		result.Skipped = len(users) - maxUsers
		users = users[:maxUsers]
	}
	result.Users = make([]OnlineUser, len(users))
	limiter := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i, email := range users {
		wg.Add(1)
		go func(index int, name string) {
			defer wg.Done()
			select {
			case limiter <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-limiter }()
			raw, err := c.onlineCommand(ctx, "statsonlineiplist", "-email", name)
			if err != nil {
				mu.Lock()
				result.Failed++
				mu.Unlock()
				return
			}
			ips, err := ParseOnlineIPList(raw)
			if err != nil {
				mu.Lock()
				result.Failed++
				mu.Unlock()
				return
			}
			result.Users[index] = OnlineUser{Email: name, IPs: ips}
		}(i, email)
	}
	wg.Wait()
	compact := result.Users[:0]
	for _, user := range result.Users {
		if user.Email != "" && len(user.IPs) > 0 {
			compact = append(compact, user)
		}
	}
	result.Users = compact
	return result, nil
}

func (c Client) onlineCommand(ctx context.Context, command string, args ...string) ([]byte, error) {
	binary := c.Binary
	if binary == "" {
		binary = "xray"
	}
	cmdArgs := append([]string{"api", command, "-s", c.Server, "-t", "2"}, args...)
	cmd := exec.CommandContext(ctx, binary, cmdArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 160 {
			msg = msg[:160]
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s: %s", command, msg)
	}
	if len(data) > 1024*1024 {
		return nil, errors.New("online API response too large")
	}
	return data, nil
}

// ParseOnlineIPList supports both protojson encodings: int64 timestamp as
// quoted string and as JSON number. Invalid IPs are ignored, never printed.
func ParseOnlineIPList(data []byte) ([]OnlineIP, error) {
	var response struct {
		IPs map[string]json.RawMessage `json:"ips"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	result := make([]OnlineIP, 0, len(response.IPs))
	for rawIP, rawTime := range response.IPs {
		ip := net.ParseIP(strings.Trim(rawIP, "[]"))
		if ip == nil {
			continue
		}
		var secText string
		if len(rawTime) > 0 && rawTime[0] == '"' {
			if err := json.Unmarshal(rawTime, &secText); err != nil {
				continue
			}
		} else {
			secText = string(rawTime)
		}
		sec, err := strconv.ParseInt(secText, 10, 64)
		if err != nil || sec < 0 {
			continue
		}
		result = append(result, OnlineIP{Address: ip.String(), LastSeen: time.Unix(sec, 0), IsLocal: ip.IsLoopback()})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Address < result[j].Address })
	return result, nil
}

// Xray 26.3.27 returns full OnlineMap keys (user>>>EMAIL>>>online),
// not plain emails. Strip the namespace before passing -email to the CLI.
func ParseOnlineUsers(data []byte) ([]string, error) {
	var response struct {
		Users []string `json:"users"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	users := make([]string, 0, len(response.Users))
	for _, key := range response.Users {
		if !strings.HasPrefix(key, "user>>>") || !strings.HasSuffix(key, ">>>online") {
			continue
		}
		email := strings.TrimSuffix(strings.TrimPrefix(key, "user>>>"), ">>>online")
		// email is a user identifier, not a filepath or shell command.
		if email == "" || len(email) > 256 || strings.Contains(email, ">>>") || seen[email] {
			continue
		}
		seen[email] = true
		users = append(users, email)
	}
	sort.Strings(users)
	return users, nil
}
