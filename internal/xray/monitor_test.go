package xray

import "testing"

func TestJoinAndAggregateNotDoubleCount(t *testing.T) {
	endpoints := []Endpoint{
		{Tag: "ws", Protocol: "VLESS", Transport: "WebSocket"},
		{Tag: "grpc", Protocol: "VLESS", Transport: "gRPC"},
		{Tag: "unused", Protocol: "Trojan", Transport: "RAW"},
	}
	in := []Traffic{
		{Name: "ws", Uplink: 10, Downlink: 100, UpRate: 5, DownRate: 10},
		{Name: "grpc", Uplink: 30, Downlink: 200, UpRate: 3, DownRate: 20},
		{Name: "orphan", Downlink: 50, DownRate: 5},
	}
	rows := JoinEndpoints(endpoints, in)
	if len(rows) != 4 {
		t.Fatalf("got %d rows", len(rows))
	}
	foundConfigOnly, foundOrphan := false, false
	for _, row := range rows {
		if row.Tag == "unused" {
			foundConfigOnly = true
			if row.HasCounters {
				t.Fatal("config only has counters")
			}
		}
		if row.Tag == "orphan" {
			foundOrphan = true
			if row.Protocol != "Unknown" || !row.HasCounters {
				t.Fatalf("orphan row %+v", row)
			}
		}
	}
	if !foundConfigOnly || !foundOrphan {
		t.Fatalf("incomplete joined rows: %+v", rows)
	}
	protocols := AggregateBy(rows, "protocol")
	trans := AggregateBy(rows, "transport")
	var totalP, totalT uint64
	var rate float64
	for _, p := range protocols {
		totalP += p.Downlink
		rate += p.DownRate
	}
	for _, v := range trans {
		totalT += v.Downlink
	}
	if totalP != 350 || totalT != 350 || rate != 35 {
		t.Fatalf("unexpected totals protocol=%d transport=%d rate=%.1f", totalP, totalT, rate)
	}
}
