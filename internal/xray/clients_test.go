package xray

import (
	"reflect"
	"testing"
	"time"
)

func TestGroupSourcesByIP(t *testing.T) {
	now := time.Unix(1800000000, 0)
	catalog := Catalog{Inbounds: []Endpoint{
		{Tag: "vless-ws", Transport: "WebSocket"},
		{Tag: "vless-grpc", Transport: "gRPC"},
	}}
	rows := []RecentSource{
		{IP: "192.0.2.3", Inbound: "vless-ws", Accepted: 4, FirstSeen: now.Add(-5 * time.Minute), LastSeen: now.Add(-60 * time.Second)},
		{IP: "192.0.2.3", Inbound: "vless-grpc", Accepted: 2, FirstSeen: now.Add(-3 * time.Minute), LastSeen: now.Add(-10 * time.Second)},
		{IP: "2001:db8::9", Inbound: "vless-ws", Accepted: 3, FirstSeen: now.Add(-2 * time.Minute), LastSeen: now.Add(-30 * time.Second)},
		{IP: "198.51.100.20", Inbound: "unknown", Accepted: 1, FirstSeen: now.Add(-time.Hour), LastSeen: now.Add(-time.Hour)},
	}
	out := GroupSourcesByIP(rows, catalog)
	if len(out) != 3 || out[0].IP != "192.0.2.3" || out[0].Accepted != 6 || out[0].FirstSeen != now.Add(-5*time.Minute) || out[0].LastSeen != now.Add(-10*time.Second) {
		t.Fatalf("bad grouped clients: %+v", out)
	}
	if !reflect.DeepEqual(out[0].Inbounds, []string{"vless-grpc", "vless-ws"}) || !reflect.DeepEqual(out[0].Transports, []string{"WebSocket", "gRPC"}) {
		t.Fatalf("bad tag/transport merge: %+v", out[0])
	}
	if len(out[2].Transports) != 0 || !reflect.DeepEqual(out[2].Inbounds, []string{"unknown"}) {
		t.Fatalf("metadata incorrectly inferred: %+v", out[2])
	}
}
func TestGroupSourcesByIPNoData(t *testing.T) {
	if len(GroupSourcesByIP(nil, Catalog{})) != 0 {
		t.Fatal("expected empty list")
	}
}
