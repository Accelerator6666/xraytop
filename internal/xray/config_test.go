package xray

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestMultiProtocolCatalog(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.json")
	writeFixture(t, file, `{
      // Config may contain comments, secrets must never enter the catalog.
      "inbounds": [
        {"tag":"grpc-in", "protocol":"vless", "listen":"/dev/shm/Xray-VLESS-gRPC.socket,0666", "port":0, "streamSettings":{"network":"grpc"}, "settings":{"clients":[{"id":"SENSITIVE-UUID"}]}},
        {"tag":"ws-in", "protocol":"vless", "listen":"/dev/shm/Xray-VLESS-WS.socket", "streamSettings":{"network":"ws", "security":"tls", "wsSettings":{"path":"/foo"}}},
        {"tag":"xhttp-in", "protocol":"trojan", "listen":"127.0.0.1", "port":23456, "streamSettings":{"method":"xhttp", "network":"grpc", "security":"reality"}},
        {"tag":"h2-in", "protocol":"vmess", "port":"20001-20005", "streamSettings":{"network":"h2"}}
      ],
      "outbounds": [{"tag":"direct", "protocol":"freedom"}],
      "log": {"access": "https://not-a-comment.example/foo//bar"}
    }`)
	cat, err := LoadCatalog([]string{file}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Inbounds) != 4 || len(cat.Outbounds) != 1 {
		t.Fatalf("catalog: %+v", cat)
	}
	if got := cat.Inbounds[0]; got.Protocol != "VLESS" || got.Transport != "gRPC" || got.Listen != "unix:/dev/shm/Xray-VLESS-gRPC.socket" {
		t.Fatalf("grpc: %+v", got)
	}
	if got := cat.Inbounds[1]; got.Transport != "WebSocket" || got.Security != "TLS" {
		t.Fatalf("websocket: %+v", got)
	}
	if got := cat.Inbounds[2]; got.Transport != "XHTTP" || got.Security != "REALITY" || got.Listen != "127.0.0.1:23456" {
		t.Fatalf("xhttp: %+v", got)
	}
	if got := cat.Inbounds[3]; got.Transport != "HTTP/2 (legacy)" || got.Listen != ":20001-20005" {
		t.Fatalf("vmess: %+v", got)
	}
	if cat.Outbounds[0].Transport != "-" {
		t.Fatalf("freedom outbound erroneously attributed proxy transport: %+v", cat.Outbounds[0])
	}
	if strings.Contains(strings.Join([]string{cat.Source, cat.Inbounds[0].Listen, cat.Inbounds[0].Tag}, " "), "SENSITIVE") {
		t.Fatal("credentials leaked")
	}
}

func TestConfdirTagReplacement(t *testing.T) {
	d := t.TempDir()
	writeFixture(t, filepath.Join(d, "01-first.json"), `{"inbounds":[{"tag":"same","protocol":"vless","streamSettings":{"network":"grpc"}}, {"tag":"keep","protocol":"vmess"}]}`)
	writeFixture(t, filepath.Join(d, "02-second.json"), `{"inbounds":[{"tag":"same","protocol":"trojan","streamSettings":{"method":"websocket"}}]}`)
	writeFixture(t, filepath.Join(d, "ignored.txt"), `not JSON`)
	cat, err := LoadCatalog(nil, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Inbounds) != 2 || cat.Inbounds[0].Protocol != "Trojan" || cat.Inbounds[0].Transport != "WebSocket" {
		t.Fatalf("merge %+v", cat)
	}
}

func TestDiscoverConfigPaths(t *testing.T) {
	proc := t.TempDir()
	base := filepath.Join(proc, "42")
	if err := os.Mkdir(base, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/srv/xray", filepath.Join(base, "cwd")); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(base, "cmdline"), "/usr/bin/xray\x00run\x00-c\x00one.json\x00--config=two.json\x00-confdir=confs\x00")
	files, dir, err := DiscoverConfigPaths(proc, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0] != "/srv/xray/one.json" || files[1] != "/srv/xray/two.json" || dir != "/srv/xray/confs" {
		t.Fatalf("files=%v dir=%q", files, dir)
	}
}

func TestInvalidConfigReportsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.json")
	writeFixture(t, path, `{"inbounds": [ this-is-not-json ]}`)
	if _, err := LoadCatalog([]string{path}, ""); err == nil {
		t.Fatal("expected parse error")
	}
}
