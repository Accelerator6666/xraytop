package xray

import (
	"strings"
	"testing"
)

func TestNGINXProxyAutoDiscovery(t *testing.T) {
	cat := Catalog{Inbounds: []Endpoint{
		{Tag:"vless-grpc", Listen:"unix:/dev/shm/Xray-VLESS-gRPC.socket", Transport:"gRPC"},
		{Tag:"vless-ws", Listen:"unix:/dev/shm/Xray-VLESS-WS.socket", Transport:"WebSocket"},
	}}
	config := "\n# configuration file /etc/nginx/nginx.conf:\nhttp {\n" +
		"upstream xray_vless_ws_9930 { server unix:/dev/shm/Xray-VLESS-WS.socket; }\n" +
		"server {\nlisten 1443 ssl http2 so_keepalive=on;\nlisten 2443 ssl http2 so_keepalive=on;\n" +
		"location /ws { proxy_pass http://xray_vless_ws_9930; }\n" +
		"location /grpc { grpc_pass unix:/dev/shm/Xray-VLESS-gRPC.socket; }\n}\n" +
		"server { listen 443 ssl; location / { proxy_pass http://ordinary_website; } }\n}\n"
	ingress := ParseNginxIngress([]byte(config),cat)
	if len(ingress.Ports)!=2 || ingress.Ports[0]!=1443 || ingress.Ports[1]!=2443 {
		t.Fatalf("ports=%v",ingress.Ports)
	}
	if len(ingress.Routes)!=4 { t.Fatalf("routes=%v",ingress.Routes) }
	for _,route := range ingress.Routes {
		if route.Port==443 { t.Fatal("included unrelated website") }
	}
}

func TestNGINXGRPCOnlyAndIPv6(t *testing.T) {
	cat := Catalog{Inbounds: []Endpoint{
		{Tag:"vless-grpc",Listen:"unix:/dev/shm/Xray-VLESS-gRPC.socket",Transport:"gRPC"},
	}}
	data := "http { server { listen [::]:6443 ssl; listen 0.0.0.0:7443 ssl; "+
		"location /grpc { grpc_pass unix:/dev/shm/Xray-VLESS-gRPC.socket; } } }"
	got := ParseNginxIngress([]byte(data),cat)
	if len(got.Ports)!=2 || got.Ports[0]!=6443 || got.Ports[1]!=7443 {
		t.Fatalf("ports=%v",got.Ports)
	}
	if len(got.Routes)!=2 {t.Fatalf("routes=%v",got.Routes)}
}

func TestNGINXNoMatchingBackend(t *testing.T) {
	cat:=Catalog{Inbounds:[]Endpoint{{Tag:"ws",Listen:"unix:/dev/shm/real.sock"}}}
	data:="http { server { listen 443 ssl; location / { proxy_pass http://fake_xray_name; } } }"
	got:=ParseNginxIngress([]byte(data),cat)
	if len(got.Ports)!=0 || len(got.Routes)!=0 {t.Fatalf("false match %+v",got)}
}

func TestNGINXLexQuotedComments(t *testing.T) {
	got:=nginxLex([]byte("server { location \"/ws#tag\" { proxy_pass 'http://real'; } # ignored\nlisten 443; }"))
	joined:=strings.Join(got," ")
	if !strings.Contains(joined,"/ws#tag") || strings.Contains(joined,"ignored") {t.Fatal(joined)}
}
