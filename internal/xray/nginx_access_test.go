package xray

import (
	"strings"
	"testing"
	"time"
)

func TestNginxCombinedLogClassification(t *testing.T) {
	now:=time.Date(2026,10,10,11,43,0,0,time.UTC)
	// The URL path/query and referer must not be retained in parsed output.
	routes:=[]NginxLocation{
		{Pattern:"/GET",Inbound:"vless-ws",Transport:"WebSocket"},
		{Pattern:"/Tun",Inbound:"vless-grpc",Transport:"gRPC"},
	}
	lines:=[]string{
		`112.82.222.50 - - [10/Oct/2026:11:42:30 +0000] "GET /GET?secret=hidden HTTP/1.1" 101 0 "-" "private user-agent"`,
		`180.116.188.24 - - [10/Oct/2026:11:42:10 +0000] "POST /Tun HTTP/2.0" 200 132 "-" "grpc"`,
		`198.51.100.1 - - [10/Oct/2026:11:42:10 +0000] "GET /website HTTP/2.0" 200 132 "-" "browser"`,
		`112.82.222.50 - - [10/Oct/2026:11:42:20 +0000] "GET /GET HTTP/1.1" 403 0 "-" "browser"`,
		`112.82.222.50 - - [10/Oct/2026:11:00:00 +0000] "GET /GET HTTP/1.1" 101 0 "-" "old"`,
	}
	got:=ParseNginxAccess([]byte(strings.Join(lines,"\n")),routes,now)
	if got.Total!=4 || got.Matched!=3 || len(got.Requests)!=2 {t.Fatalf("unexpected %+v",got)}
	if got.Requests[0].IP!="112.82.222.50" || got.Requests[0].Requests!=2 || got.Requests[0].SuccessHTTP!=1 {t.Fatalf("ws %+v",got.Requests[0])}
	if got.Requests[1].Inbound!="vless-grpc" || got.Requests[1].SuccessHTTP!=1 {t.Fatalf("grpc %+v",got.Requests[1])}
}
func TestNginxAmbiguousMappingNotAttributed(t *testing.T) {
	routes:=[]NginxLocation{{Pattern:"/GET",Inbound:"one",Transport:"WebSocket"},{Pattern:"/GET",Inbound:"two",Transport:"WebSocket"}}
	if _,ok:=matchNginxPath("/GET","GET",101,routes);ok {t.Fatal("ambiguous URI falsely attributed")}
}
func TestNginxLongestLocation(t *testing.T) {
	routes:=[]NginxLocation{{Pattern:"/",Inbound:"web",Transport:"Other"},{Pattern:"/Tun",Inbound:"grpc",Transport:"gRPC"}}
	got,ok:=matchNginxPath("/Tun","POST",200,routes)
	if !ok || got.Inbound!="grpc" {t.Fatalf("%+v %v",got,ok)}
}
func TestNginxLocationsFromConfig(t *testing.T) {
	cat:=Catalog{Inbounds:[]Endpoint{{Tag:"ws",Listen:"unix:/ws.sock",Transport:"WebSocket"},{Tag:"grpc",Listen:"unix:/grpc.sock",Transport:"gRPC"}}}
	conf:=`http { upstream web { server unix:/ws.sock; } server {listen 1443 ssl; location /GET {proxy_pass http://web;} location /Tun {grpc_pass unix:/grpc.sock;} } }`
	got:=ParseNginxIngress([]byte(conf),cat)
	if len(got.Locations)!=2 {t.Fatalf("locations %v",got.Locations)}
	if len(got.Routes)!=2 {t.Fatalf("routes %v",got.Routes)}
}
