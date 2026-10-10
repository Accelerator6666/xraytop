package tui

import (
	"github.com/Accelerator6666/xraytop/internal/collector"
	"github.com/Accelerator6666/xraytop/internal/xray"
	"strings"
	"testing"
	"time"
)

func TestRender(t *testing.T) {
	s := Screen{Width: 80, Height: 20, Snapshot: collector.Snapshot{At: time.Unix(0, 0)}}
	v := s.Paint()
	if !strings.Contains(v, "XrayTop") || !strings.Contains(v, "StatsService") || !strings.Contains(v, "Quit") {
		t.Fatalf("unexpected render: %q", v)
	}
	if spark([]float64{0, 1, 2}, 3) == "" {
		t.Fatal("empty graph")
	}
}

func TestProtocolAndTransportViews(t *testing.T) {
	endpoints := []xray.MonitoredEndpoint{{Endpoint: xray.Endpoint{Tag: "grpc01", Protocol: "VLESS", Transport: "gRPC", Security: "TLS"}, Traffic: xray.Traffic{Name: "grpc01", Downlink: 2048}, HasCounters: true}}
	for _, view := range []int{1, 3, 4} {
		screen := Screen{Width: 110, Height: 24, View: view, Snapshot: collector.Snapshot{At: time.Unix(0, 0), Inbounds: endpoints}}
		out := screen.Paint()
		want := "gRPC"
		if view == 3 {
			want = "VLESS"
		}
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s at view=%d: %q", want, view, out)
		}
	}
}

func TestOnlineUsersView(t *testing.T) {
	at := time.Unix(1791592000, 0)
	snap := collector.Snapshot{At: at, APIEnabled: true, Online: xray.OnlineSnapshot{
		CheckedAt: at,
		Users:     []xray.OnlineUser{{Email: "alice@example.com", IPs: []xray.OnlineIP{{Address: "112.82.222.50", LastSeen: at.Add(-5 * time.Second)}}}},
	}}
	output := Screen{Width: 110, Height: 20, View: 5, Snapshot: snap}.Paint()
	for _, want := range []string{"ONLINE USERS", "alice@example.com", "112.82.222.50", "5s ago", "[6] Users"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output: %s", want, output)
		}
	}
	snap.Online = xray.OnlineSnapshot{}
	snap.Catalog.OnlinePolicyConfigured = false
	output = Screen{Width: 110, Height: 20, View: 5, Snapshot: snap}.Paint()
	if !strings.Contains(output, "statsUserOnline") {
		t.Error("missing policy guidance")
	}
	if safeUser("alice\x1b[31m") == "alice\x1b[31m" {
		t.Fatal("unsafe escape preserved")
	}
}

func TestRecentSourceView(t *testing.T) {
	snapshot := collector.Snapshot{At: time.Unix(1791602300, 0), Recent: xray.RecentActivity{
		CheckedAt: time.Unix(1791602300, 0), Events: 3,
		Sources: []xray.RecentSource{{IP: "192.0.2.5", Inbound: "vless-ws", Accepted: 3, LastSeen: time.Unix(1791602211, 0)}},
	}}
	out := (Screen{Width: 110, Height: 24, View: 6, Snapshot: snapshot}).Paint()
	for _, want := range []string{"RECENT ACCEPTED", "192.0.2.5", "vless-ws", "NOT live sessions"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
}
