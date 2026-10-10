package xray

import (
	"strings"
	"testing"
)

func TestParseRecentActivity(t *testing.T) {
	sample := `1791602211.100100 host xray[1]: 2026/10/10 03:17:44 from 112.82.222.50:0 accepted tcp:github.com:443 [vless-ws >> direct]
1791602212.220101 host xray[1]: 2026/10/10 03:17:44 from 112.82.222.50:0 accepted tcp:1.1.1.1:443 [vless-ws >> direct]
1791602213.300000 host xray[1]: 2026/10/10 03:17:44 from [2001:db8::8]:0 accepted tcp:[2606:4700::1111]:80 [vless-grpc >> direct]
1791602214.300000 host xray[1]: from 114.227.107.223:0 rejected proxy/vless/encoding: invalid request user id: something
1791602215.300000 host xray[1]: from 114.227.107.223:0 accepted tcp:foo [broken]
1791602216.300000 host xray[1]: from not-an-ip:0 accepted tcp:foo [vless-ws >> direct]
`
	rows, total := ParseRecentActivity([]byte(sample))
	if total != 3 || len(rows) != 2 {
		t.Fatalf("total=%d rows=%+v", total, rows)
	}
	if rows[0].IP != "2001:db8::8" || rows[0].Inbound != "vless-grpc" {
		t.Fatalf("ipv6 %+v", rows[0])
	}
	if rows[1].Accepted != 2 || rows[1].IP != "112.82.222.50" {
		t.Fatalf("ipv4 %+v", rows[1])
	}
	if strings.Contains(rows[0].Inbound+rows[1].Inbound, "rejected") {
		t.Fatal("rejected attempt included")
	}
}

func TestValidJournalUnit(t *testing.T) {
	for _, u := range []string{"xray", "xray.service", "xray@one.service"} {
		if !validJournalUnit(u) {
			t.Fatalf("valid unit %q rejected", u)
		}
	}
	for _, u := range []string{"", "-xray", "xray;rm -rf /", "../../xray", "xray\n"} {
		if validJournalUnit(u) {
			t.Fatalf("invalid unit %q accepted", u)
		}
	}
}
