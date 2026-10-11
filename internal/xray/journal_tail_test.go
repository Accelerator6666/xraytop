package xray

import (
 "testing"
 "time"
)
func TestJournalParseAndGrouping(t *testing.T){
 now:=time.Now()
 l1,ok:=parseAcceptedJournalMessage("2026/10/11 from 192.0.2.4:43112 accepted tcp:www.example [vless-ws >> direct]",now)
 if !ok||l1.IP!="192.0.2.4"||l1.Inbound!="vless-ws"{t.Fatalf("%+v %v",l1,ok)}
 l2,ok:=parseAcceptedJournalMessage("2026/10/11 from 192.0.2.4:43113 accepted tcp:example.net [vless-ws >> direct]",now)
 if !ok{t.Fatal("second event rejected")}
 res:=buildRecentActivity([]RecentSource{l1,l2},now)
 if res.Events!=2||len(res.Sources)!=1||res.Sources[0].Accepted!=2{t.Fatalf("%+v",res)}
 res=buildRecentActivity([]RecentSource{l1},now.Add(7*time.Minute))
 if res.Events!=0{t.Fatalf("expired %+v",res)}
 if _,ok:=parseAcceptedJournalMessage("from 192.0.2.4:12 rejected tcp:x [ws >> direct]",now);ok{t.Fatal("rejected should not count")}
}
