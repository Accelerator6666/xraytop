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
    LastSeen time.Time // last accepted Xray event; never populated by HTTP alone
    HTTPRequests int
    HTTPSuccess int
    HTTPLastSeen time.Time
    HTTPRoutes []string // observed HTTP route metadata, NOT authenticated inbounds
}

// Evidence intentionally distinguishes proxy activity from HTTP requests and
// network-level TCP sockets; it never claims a unique person or live session.
func (c ObservedClient) Evidence() string {
    if c.Accepted > 0 { return "XRAY" }
    if c.HTTPRequests > 0 { return "HTTP" }
    if c.TCPConnections > 0 { return "TCP" }
    return "-"
}

// MatchObservedFilter is used by the interactive f key and --clients-filter.
// The HTTP and TCP filters select their *exclusive* observation tier.
func MatchObservedFilter(c ObservedClient, filter string) bool {
    switch filter {
    case "", "all": return true
    case "xray": return c.Accepted > 0
    case "http": return c.Accepted == 0 && c.HTTPRequests > 0
    case "tcp": return c.Accepted == 0 && c.HTTPRequests == 0 && c.TCPConnections > 0
    default: return false
    }
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

// Joining by source IP is a presentation convenience and never proves that
// TCP sockets or HTTP responses correspond to authenticated Xray requests.
// Preserve the older API for existing callers/tests.
func MergeObservedClients(sources []RecentSource, catalog Catalog, live LiveTCP) []ObservedClient {
    return MergeObservedClientsWithHTTP(sources,catalog,live,NginxAccess{})
}

func MergeObservedClientsWithHTTP(sources []RecentSource, catalog Catalog, live LiveTCP, access NginxAccess) []ObservedClient {
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
    routes:=make(map[string]map[string]bool)
    for _,req:=range access.Requests {
        if req.IP=="" || req.Requests<=0 {continue}
        row:=groups[req.IP]
        if row==nil {row=&ObservedClient{IP:req.IP};groups[req.IP]=row}
        row.HTTPRequests+=req.Requests
        row.HTTPSuccess+=req.SuccessHTTP
        if req.LastSeen.After(row.HTTPLastSeen) {row.HTTPLastSeen=req.LastSeen}
        if routes[req.IP]==nil {routes[req.IP]=make(map[string]bool)}
        if req.Inbound!="" {routes[req.IP][req.Inbound]=true}
    }
    out:=make([]ObservedClient,0,len(groups))
    for _,row:=range groups {
        for tag:=range routes[row.IP] {row.HTTPRoutes=append(row.HTTPRoutes,tag)}
        sort.Strings(row.HTTPRoutes)
        out=append(out,*row)
    }
    priority:=func(row ObservedClient) int {
        if row.Accepted>0 {return 3}
        if row.HTTPRequests>0 {return 2}
        if row.TCPConnections>0 {return 1}
        return 0
    }
    sort.Slice(out,func(i,j int)bool{
        a,b:=out[i],out[j]
        if priority(a)!=priority(b) {return priority(a)>priority(b)}
        // Within the Xray tier, prefer recent accepted activity; within HTTP
        // use recent request time; and only TCP sources sort by open sockets.
        if a.Accepted>0 {
            if !a.LastSeen.Equal(b.LastSeen) {return a.LastSeen.After(b.LastSeen)}
            if a.Accepted!=b.Accepted {return a.Accepted>b.Accepted}
        } else if a.HTTPRequests>0 {
            if !a.HTTPLastSeen.Equal(b.HTTPLastSeen) {return a.HTTPLastSeen.After(b.HTTPLastSeen)}
            if a.HTTPRequests!=b.HTTPRequests {return a.HTTPRequests>b.HTTPRequests}
        } else if a.TCPConnections!=b.TCPConnections {
            return a.TCPConnections>b.TCPConnections
        }
        return a.IP<b.IP
    })
    return out
}
