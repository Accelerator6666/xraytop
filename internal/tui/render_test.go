package tui

import (
	"github.com/Accelerator6666/xraytop/internal/collector"
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
