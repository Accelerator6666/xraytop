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

func TestIPFirstClientsView(t *testing.T) {
	at := time.Unix(1791602300, 0)
	snap := collector.Snapshot{At: at, Recent: xray.RecentActivity{
		CheckedAt: at, Events: 6,
		Sources: []xray.RecentSource{
			{IP: "192.0.2.1", Inbound: "vless-ws", Accepted: 4, FirstSeen: at.Add(-80 * time.Second), LastSeen: at.Add(-7 * time.Second)},
			{IP: "192.0.2.1", Inbound: "vless-grpc", Accepted: 2, FirstSeen: at.Add(-60 * time.Second), LastSeen: at.Add(-6 * time.Second)},
		},
	}, Catalog: xray.Catalog{Inbounds: []xray.Endpoint{
		{Tag: "vless-ws", Transport: "WebSocket"}, {Tag: "vless-grpc", Transport: "gRPC"},
	}}}
	output := Screen{Width: 130, Height: 24, View: 5, Snapshot: snap}.Paint()
	for _, want := range []string{"CLIENTS BY SOURCE IP", "192.0.2.1", "vless-grpc,vless-ws", "6 accepted journal events", "[6] IP Clients", "NOT Xray user sessions"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q: %s", want, output)
		}
	}
	if strings.Contains(output, "email") || strings.Contains(output, "statsUserOnline") {
		t.Fatalf("email tracking leaked into IP-first view: %s", output)
	}
	snap.Recent = xray.RecentActivity{CheckedAt: at}
	output = Screen{Width: 100, Height: 20, View: 5, Snapshot: snap}.Paint()
	if !strings.Contains(output, "does not prove") {
		t.Error("missing empty journal caveat")
	}
	if safeUser("alice\x1b[31m") == "alice\x1b[31m" {
		t.Fatal("unsafe escape preserved")
	}
}

func TestClientIPFilters(t *testing.T) {
    at:=time.Unix(1791602300,0)
    snap:=collector.Snapshot{
        At:at,
        Recent:xray.RecentActivity{Events:2,Sources:[]xray.RecentSource{{IP:"192.0.2.1",Inbound:"vless-ws",Accepted:2,LastSeen:at}}},
        Live:xray.LiveTCP{Peers:[]xray.TCPPeer{{IP:"203.0.113.5",Connections:15,Ports:[]uint16{6443}}},Connections:15,Ports:[]uint16{6443}},
        Access:xray.NginxAccess{Matched:4,Requests:[]xray.NginxRequest{{IP:"198.51.100.9",Inbound:"vless-grpc",Transport:"gRPC",Requests:4,LastSeen:at}}},
    }
    all:=(Screen{Width:180,Height:30,View:5,Snapshot:snap,ClientFilter:"all"}).Paint()
    for _,needle:=range []string{"192.0.2.1","198.51.100.9","203.0.113.5","XRAY","HTTP","TCP","6443","Sources: XRAY 1 / HTTP 1 / TCP 1"} {
        if !strings.Contains(all,needle) {t.Errorf("missing %q: %s",needle,all)}
    }
    for _,tc:=range []struct{filter,ip,excluded string}{
        {"xray","192.0.2.1","198.51.100.9"},
        {"http","198.51.100.9","203.0.113.5"},
        {"tcp","203.0.113.5","192.0.2.1"},
    } {
        got:=(Screen{Width:180,Height:30,View:5,Snapshot:snap,ClientFilter:tc.filter}).Paint()
        if !strings.Contains(got,tc.ip)||strings.Contains(got,tc.excluded){t.Fatalf("wrong filter %s: %s",tc.filter,got)}
    }
}
func TestNGINXAutoStatus(t *testing.T) {
	snap := collector.Snapshot{At:time.Unix(100,0), Nginx:xray.NginxIngress{CheckedAt:time.Unix(100,0), Ports:[]uint16{1443,2443}, Routes:[]xray.NginxRoute{{Port:1443,Inbound:"vless-grpc"},{Port:2443,Inbound:"vless-grpc"}}}}
	out := (Screen{Width:130,Height:24,View:5,Snapshot:snap}).Paint()
	if !strings.Contains(out,"NGINX auto: 2 verified proxy ports / 2 Xray inbound routes") { t.Fatalf("missing auto detection: %s",out) }
}

func TestNginxAccessView(t *testing.T) {
	at:=time.Unix(1791602300,0)
	snap:=collector.Snapshot{At:at,Access:xray.NginxAccess{CheckedAt:at,Total:4,Matched:2,Requests:[]xray.NginxRequest{{IP:"192.0.2.9",Inbound:"vless-ws",Transport:"WebSocket",Requests:2,SuccessHTTP:1,LastSeen:at}}}}
	out:=(Screen{Width:145,Height:25,View:7,Snapshot:snap}).Paint()
	for _,want:=range []string{"NGINX PROXY HTTP REQUESTS","192.0.2.9","WebSocket","Matched 2 / 4","NOT an authenticated VLESS"} {if !strings.Contains(out,want){t.Fatalf("missing %q: %s",want,out)}}
}

func TestRecentSourceView(t *testing.T) {
	snapshot := collector.Snapshot{At: time.Unix(1791602300, 0), Recent: xray.RecentActivity{
		CheckedAt: time.Unix(1791602300, 0), Events: 3,
		Sources: []xray.RecentSource{{IP: "192.0.2.5", Inbound: "vless-ws", Accepted: 3, LastSeen: time.Unix(1791602211, 0)}},
	}}
	out := (Screen{Width: 110, Height: 24, View: 6, Snapshot: snapshot}).Paint()
	for _, want := range []string{"SOURCE IP + INBOUND", "192.0.2.5", "vless-ws", "NOT online status"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
}

func TestFooterAlwaysFitsWithQuitVisible(t *testing.T){
 for width:=38;width<=178;width++{
   footer:=footerForWidth(width,false)
   if !strings.Contains(footer,"[q] Quit") && !strings.Contains(footer,"[q]Quit"){
      t.Fatalf("width=%d lost Quit: %q",width,footer)
   }
   if len([]rune(footer))>width{
      t.Fatalf("width=%d footer longer (%d): %q",width,len([]rune(footer)),footer)
   }
   for _,view:=range []int{0,8}{
      out:=Screen{Width:width+2,Height:24,View:view,Snapshot:collector.Snapshot{At:time.Unix(0,0)}}.Paint()
      lines:=strings.Split(out,"\n")
      if !strings.Contains(lines[len(lines)-1],"[q]"){
        t.Errorf("width=%d view=%d footer clipped: %q",width,view,lines[len(lines)-1])
      }
   }
 }
}
func TestSettingsFooterAlwaysFits(t *testing.T){
 for width:=38;width<=178;width++ {
   got:=footerForWidth(width,true)
   if len([]rune(got))>width||!strings.Contains(got,"[z]"){
      t.Errorf("width=%d footer=%q",width,got)
   }
 }
}
