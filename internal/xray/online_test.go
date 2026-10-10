package xray

import (
	"testing"
)

func TestParseOnlineIPList(t *testing.T) {
	sample := []byte(`{"name":"user>>>alice>>>online","ips":{"112.82.222.50":"1791590000","2001:db8::1":1791590030,"127.0.0.1":"1791590020","not-an-ip":"1791590040"}}`)
	ips, err := ParseOnlineIPList(sample)
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 3 {
		t.Fatalf("got %v", ips)
	}
	if ips[0].Address != "112.82.222.50" {
		t.Fatalf("unexpected order %+v", ips)
	}
	if ips[0].LastSeen.Unix() != 1791590000 {
		t.Fatal("timestamp parsing")
	}
	if !ips[1].IsLocal {
		t.Fatal("loopback not labeled")
	}
}
func TestParseOnlineEmptyAndInvalid(t *testing.T) {
	ips, err := ParseOnlineIPList([]byte(`{"name":"foo"}`))
	if err != nil || len(ips) != 0 {
		t.Fatalf("ips=%v err=%v", ips, err)
	}
	if _, err = ParseOnlineIPList([]byte(`{`)); err == nil {
		t.Fatal("expected invalid json")
	}
}

func TestParseOnlineUsers(t *testing.T) {
	users, err := ParseOnlineUsers([]byte(`{"users":["user>>>alice@example.com>>>online","user>>>bob>>>online","not-an-online-map","user>>>alice@example.com>>>online"]}`))
	if err != nil || len(users) != 2 || users[0] != "alice@example.com" || users[1] != "bob" {
		t.Fatalf("users=%v err=%v", users, err)
	}
}
