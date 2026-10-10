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
