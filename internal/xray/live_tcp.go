package xray

import (
    "bufio"
    "errors"
    "fmt"
    "net/netip"
    "os"
    "path/filepath"
    "sort"
    "strconv"
    "strings"
    "time"
)

// TCPPeer is a host TCP socket peer on explicitly selected ingress ports.
// It is NOT evidence that the peer has authenticated to Xray.
type TCPPeer struct {
    IP string
    Connections int
    Ports []uint16
}
type LiveTCP struct {
    CheckedAt time.Time
    Ports []uint16
    Peers []TCPPeer
    Connections int
}
type ObservedClient struct {
    IP string
    TCPConnections int
    Ports []uint16
    Accepted int
    Inbounds []string
    Transports []string
    LastSeen time.Time
}

func ParseIngressPorts(raw string) ([]uint16, error) {
    if strings.TrimSpace(raw)=="" {return nil,nil}
    set:=make(map[uint16]bool)
    for _,item:=range strings.Split(raw,",") {
        item=strings.TrimSpace(item)
        n,err:=strconv.ParseUint(item,10,16)
        if err!=nil || n==0 {return nil,fmt.Errorf("invalid ingress port %q",item)}
        set[uint16(n)]=true
    }
    out:=make([]uint16,0,len(set))
    for p:=range set {out=append(out,p)}
    sort.Slice(out,func(i,j int)bool{return out[i]<out[j]})
    return out,nil
}

// ReadLiveTCP inspects the local Linux kernel's ESTABLISHED sockets. Unless
// ingress ports are specified, no sockets are enumerated. No Xray config changes.
func ReadLiveTCP(procRoot string, ports []uint16) (LiveTCP,error) {
    result:=LiveTCP{CheckedAt:time.Now(),Ports:append([]uint16(nil),ports...)}
    if len(ports)==0 {return result,nil}
    if procRoot=="" {procRoot="/proc"}
    allowed:=make(map[uint16]bool)
    for _,p:=range ports {allowed[p]=true}
    peers:=make(map[string]*TCPPeer)
    opened:=0
    var errs []error
    for _,family:=range []struct{name string;ipv6 bool}{{"tcp",false},{"tcp6",true}} {
        f,err:=os.Open(filepath.Join(procRoot,"net",family.name))
        if err!=nil {errs=append(errs,fmt.Errorf("%s: %w",family.name,err));continue}
        opened++
        scan:=bufio.NewScanner(f)
        scan.Buffer(make([]byte,4096),256*1024)
        first:=true
        for scan.Scan() {
            if first {first=false;continue}
            local,remote,state,ok:=parseTCPEntry(scan.Text(),family.ipv6)
            if !ok || state!=1 || !allowed[local.Port()] {continue}
            addr:=remote.Addr().Unmap()
            if !addr.IsValid() || addr.IsUnspecified() {continue}
            ip:=addr.String()
            peer:=peers[ip]
            if peer==nil {peer=&TCPPeer{IP:ip};peers[ip]=peer}
            peer.Connections++
            result.Connections++
            found:=false
            for _,port:=range peer.Ports {if port==local.Port() {found=true;break}}
            if !found {peer.Ports=append(peer.Ports,local.Port())}
        }
        if err:=scan.Err();err!=nil {errs=append(errs,fmt.Errorf("scan %s: %w",family.name,err))}
        _=f.Close()
    }
    for _,peer:=range peers {
        sort.Slice(peer.Ports,func(i,j int)bool{return peer.Ports[i]<peer.Ports[j]})
        result.Peers=append(result.Peers,*peer)
    }
    sort.Slice(result.Peers,func(i,j int)bool{
        a,b:=result.Peers[i],result.Peers[j]
        if a.Connections!=b.Connections {return a.Connections>b.Connections}
        return a.IP<b.IP
    })
    if opened==0 {return result,fmt.Errorf("proc TCP tables unavailable: %w",errors.Join(errs...))}
    if len(errs)>0 {return result,errors.Join(errs...)}
    return result,nil
}
func parseTCPEntry(s string,ipv6 bool)(netip.AddrPort,netip.AddrPort,uint8,bool){
    f:=strings.Fields(s)
    if len(f)<4 {return netip.AddrPort{},netip.AddrPort{},0,false}
    local,e1:=parseTCPEndpoint(f[1],ipv6)
    remote,e2:=parseTCPEndpoint(f[2],ipv6)
    state,e3:=strconv.ParseUint(f[3],16,8)
    return local,remote,uint8(state),e1==nil && e2==nil && e3==nil
}
func parseTCPEndpoint(raw string,ipv6 bool)(netip.AddrPort,error){
    a,p,ok:=strings.Cut(raw,":")
    if !ok {return netip.AddrPort{},errors.New("missing proc endpoint port")}
    port,err:=strconv.ParseUint(p,16,16)
    if err!=nil {return netip.AddrPort{},err}
    if !ipv6 {
        if len(a)!=8 {return netip.AddrPort{},errors.New("invalid IPv4 proc address")}
        n,err:=strconv.ParseUint(a,16,32)
        if err!=nil {return netip.AddrPort{},err}
        addr:=netip.AddrFrom4([4]byte{byte(n),byte(n>>8),byte(n>>16),byte(n>>24)})
        return netip.AddrPortFrom(addr,uint16(port)),nil
    }
    if len(a)!=32 {return netip.AddrPort{},errors.New("invalid IPv6 proc address")}
    var bytes [16]byte
    for i:=0;i<4;i++ {
        n,err:=strconv.ParseUint(a[i*8:(i+1)*8],16,32)
        if err!=nil {return netip.AddrPort{},err}
        for j:=0;j<4;j++ {bytes[i*4+j]=byte(n>>(8*j))}
    }
    return netip.AddrPortFrom(netip.AddrFrom16(bytes),uint16(port)),nil
}

// Joining by source IP is only a UI convenience. It NEVER proves that an
// ESTABLISHED ingress socket corresponds to an accepted Xray request.
func MergeObservedClients(sources []RecentSource,catalog Catalog,live LiveTCP) []ObservedClient {
    groups:=make(map[string]*ObservedClient)
    for _,src:=range GroupSourcesByIP(sources,catalog) {
        groups[src.IP]=&ObservedClient{IP:src.IP,Accepted:src.Accepted,Inbounds:src.Inbounds,Transports:src.Transports,LastSeen:src.LastSeen}
    }
    for _,peer:=range live.Peers {
        row:=groups[peer.IP]
        if row==nil {row=&ObservedClient{IP:peer.IP};groups[peer.IP]=row}
        row.TCPConnections+=peer.Connections
        row.Ports=append([]uint16(nil),peer.Ports...)
    }
    out:=make([]ObservedClient,0,len(groups))
    for _,row:=range groups {out=append(out,*row)}
    sort.Slice(out,func(i,j int)bool {
        a,b:=out[i],out[j]
        if (a.TCPConnections>0)!=(b.TCPConnections>0) {return a.TCPConnections>0}
        if a.TCPConnections!=b.TCPConnections {return a.TCPConnections>b.TCPConnections}
        if !a.LastSeen.Equal(b.LastSeen) {return a.LastSeen.After(b.LastSeen)}
        return a.IP<b.IP
    })
    return out
}
