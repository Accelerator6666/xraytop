package xray

import (
    "os"
    "path/filepath"
    "testing"
)

func TestIngressPorts(t *testing.T) {
    p,err:=ParseIngressPorts("443,8964,443,65535")
    if err!=nil || len(p)!=3 || p[0]!=443 || p[1]!=8964 || p[2]!=65535 {t.Fatalf("ports=%v err=%v",p,err)}
    for _,v:=range []string{"0","65536","80,","foo","-1"} {if _,err:=ParseIngressPorts(v);err==nil {t.Errorf("accepted %q",v)}}
}
func TestLinuxIPv4AndIPv6Sockets(t *testing.T) {
    d:=t.TempDir()
    if err:=os.MkdirAll(filepath.Join(d,"net"),0700);err!=nil {t.Fatal(err)}
    tcp:=" sl local_address rem_address st\n"+
        "0: 0100007F:01BB 190200C0:D431 01\n"+
        "1: 0100007F:01BB 190200C0:D432 01\n"+
        "2: 0100007F:0016 190200C0:D433 01\n"+
        "3: 0100007F:01BB 190200C0:D434 0A\n"
    tcp6:=" sl local_address rem_address st\n"+
        "0: 00000000000000000000000001000000:01BB B80D0120000000000000000001000000:04D2 01\n"
    if err:=os.WriteFile(filepath.Join(d,"net","tcp"),[]byte(tcp),0600);err!=nil {t.Fatal(err)}
    if err:=os.WriteFile(filepath.Join(d,"net","tcp6"),[]byte(tcp6),0600);err!=nil {t.Fatal(err)}
    s,err:=ReadLiveTCP(d,[]uint16{443})
    if err!=nil || s.Connections!=3 || len(s.Peers)!=2 {t.Fatalf("snapshot=%+v err=%v",s,err)}
    v4,v6:=false,false
    for _,peer:=range s.Peers {
        if peer.IP=="192.0.2.25" && peer.Connections==2 {v4=true}
        if peer.IP=="2001:db8::1" && peer.Connections==1 {v6=true}
    }
    if !v4 || !v6 {t.Errorf("decoded peers: %+v",s.Peers)}
}
func TestTCPUnavailable(t *testing.T) {
    if _,err:=ReadLiveTCP(t.TempDir(),[]uint16{443});err==nil {t.Fatal("missing proc tables should error")}
    if _,err:=ReadLiveTCP(t.TempDir(),nil);err!=nil {t.Fatal(err)}
}
func TestIPJoinIsNotIdentity(t *testing.T) {
    recent:=[]RecentSource{{IP:"192.0.2.25",Inbound:"vless-ws",Accepted:4},{IP:"198.51.100.15",Inbound:"vless-grpc",Accepted:1}}
    live:=LiveTCP{Peers:[]TCPPeer{{IP:"192.0.2.25",Connections:2},{IP:"2001:db8::2",Connections:1}}}
    joined:=MergeObservedClients(recent,Catalog{},live)
    if len(joined)!=3 || joined[0].IP!="192.0.2.25" || joined[0].Accepted!=4 || joined[0].TCPConnections!=2 {t.Fatalf("%+v",joined)}
}
