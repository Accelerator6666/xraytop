package xray

import (
	"math"
	"testing"
	"time"
)

func TestParseStats(t *testing.T) {
	s, err := ParseStats([]byte(`{"stat":[{"name":"inbound>>>grpc>>>traffic>>>downlink","value":"1234"},{"name":"inbound>>>grpc>>>traffic>>>uplink","value":42}]}`))
	if err != nil || s["inbound>>>grpc>>>traffic>>>downlink"] != 1234 || s["inbound>>>grpc>>>traffic>>>uplink"] != 42 {
		t.Fatalf("parsed=%v err=%v", s, err)
	}
	if _, err := ParseStats([]byte(`{"stat":[{"name":"bad","value":"oops"}]}`)); err == nil {
		t.Fatal("expected invalid counter error")
	}
}
// Xray 26.3.27 omits the value field for idle counters.
func TestParseStatsOmittedZeroValues(t *testing.T) {
	data := []byte(`{"stat":[
	  {"name":"outbound>>>blocked>>>traffic>>>uplink"},
	  {"name":"outbound>>>tw>>>traffic>>>downlink"},
	  {"name":"outbound>>>jp>>>traffic>>>uplink","value":null},
	  {"name":"inbound>>>vless-ws>>>traffic>>>uplink","value":42898},
	  {"name":"inbound>>>vless-ws>>>traffic>>>downlink","value":50733},
	  {"name":"inbound>>>vless-grpc>>>traffic>>>uplink","value":5181},
	  {"name":"inbound>>>vless-grpc>>>traffic>>>downlink","value":5832},
	  {"name":"outbound>>>direct>>>traffic>>>uplink","value":"44570"}
	]}`)
	stats, err := ParseStats(data)
	if err != nil {
		t.Fatalf("parse Xray 26.3.27 statsquery: %v", err)
	}
	if len(stats) != 8 {
		t.Fatalf("got %d counters, want 8", len(stats))
	}
	for _, name := range []string{
		"outbound>>>blocked>>>traffic>>>uplink",
		"outbound>>>tw>>>traffic>>>downlink",
		"outbound>>>jp>>>traffic>>>uplink",
	} {
		if v, ok := stats[name]; !ok || v != 0 {
			t.Errorf("zero counter %q: value=%d present=%v", name, v, ok)
		}
	}
	if stats["outbound>>>direct>>>traffic>>>uplink"] != 44570 {
		t.Fatal("nonzero outbound data lost")
	}
	var sampler Sampler
	summary := sampler.Update(time.Unix(100, 0), stats)
	if summary.UpTotal != 48079 || summary.DownTotal != 56565 {
		t.Errorf("got totals up=%d down=%d, want 48079/56565", summary.UpTotal, summary.DownTotal)
	}
	if len(summary.Outbound) != 4 {
		t.Errorf("got %d outbound rows, want 4", len(summary.Outbound))
	}
}

func TestSampler(t *testing.T) {
	first := Stats{"inbound>>>A>>>traffic>>>downlink": 100, "outbound>>>B>>>traffic>>>downlink": 100, "inbound>>>A>>>traffic>>>uplink": 30}
	second := Stats{"inbound>>>A>>>traffic>>>downlink": 250, "outbound>>>B>>>traffic>>>downlink": 250, "inbound>>>A>>>traffic>>>uplink": 50}
	var s Sampler
	now := time.Unix(100, 0)
	if s.Update(now, first).HasRates {
		t.Fatal("first sample is not a rate")
	}
	b := s.Update(now.Add(3*time.Second), second)
	if !b.HasRates || math.Abs(b.DownRate-50) > 0.01 || math.Abs(b.UpRate-20.0/3) > 0.01 || b.DownTotal != 250 {
		t.Fatalf("bad sample: %+v", b)
	}
	reset := s.Update(now.Add(4*time.Second), Stats{"inbound>>>A>>>traffic>>>downlink": 10})
	if reset.DownRate != 0 {
		t.Fatalf("reset yielded burst: %v", reset.DownRate)
	}
}
func TestLoopbackOnly(t *testing.T) {
	for _, address := range []string{"127.0.0.1:10085", "localhost:10085", "[::1]:10085"} {
		if err := validateLocalAddress(address); err != nil {
			t.Errorf("%s: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:10085", "8.8.8.8:10085", "example.com:10085", "localhost:99999"} {
		if err := validateLocalAddress(address); err == nil {
			t.Errorf("%s should be rejected", address)
		}
	}
}
