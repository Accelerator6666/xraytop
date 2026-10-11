package xray

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// NginxRequest is an HTTP request observed in the default NGINX combined
// access log at a URI mapped to an Xray upstream. Neither HTTP 101 nor HTTP
// 200 proves a client authenticated to Xray.
type NginxRequest struct {
	IP        string
	Inbound   string
	Transport string
	Requests  int
	SuccessHTTP int
	LastSeen  time.Time
}

type NginxAccess struct {
	CheckedAt time.Time
	Requests []NginxRequest
	Total int
	Matched int
	// Truncated means older log lines were skipped by the read bound.
	Truncated bool
}

// nginxCombined captures only the source IP, time, method, path and status.
// It explicitly does not retain URL, query string, user-agent or referrer.
var nginxCombined = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^]]+)\] "([A-Z]+) (\S+) HTTP/[^"]+" (\d{3})(?: |$)`)

func matchNginxPath(path, method string, status int, routes []NginxLocation) (NginxLocation, bool) {
	// Resolve longest matching NGINX location. Exact locations take precedence.
	// Ambiguity across different server blocks or backends => no attribution.
	found := NginxLocation{}
	score := -1
	ambiguous := false
	for _, route := range routes {
		if route.Pattern == "" { continue }
		if route.Exact && path != route.Pattern { continue }
		if !route.Exact && !strings.HasPrefix(path, route.Pattern) { continue }
		priority := len(route.Pattern)
		if route.Exact { priority += 100000 }
		if priority > score {
			score, found, ambiguous = priority, route, false
		} else if priority == score && (route.Inbound != found.Inbound || route.Transport != found.Transport) {
			ambiguous = true
		}
	}
	if score < 0 || ambiguous { return NginxLocation{}, false }
	// Standard WS and gRPC entry methods. This is only a request-level
	// classification, not VLESS authentication. Status can still be an error.
	switch found.Transport {
	case "WebSocket":
		if method != "GET" { return NginxLocation{}, false }
	case "gRPC":
		if method != "POST" { return NginxLocation{}, false }
	}
	if status < 100 || status > 599 { return NginxLocation{}, false }
	return found,true
}

// nginxLogEvent is intentionally metadata-only: no URI, user-agent,
// headers, referer, or raw log line survive parsing.
type nginxLogEvent struct {
 at time.Time
 request NginxRequest
 matched bool
}
func parseNginxAccessLine(line string,routes []NginxLocation,now time.Time)(nginxLogEvent,bool){
 m:=nginxCombined.FindStringSubmatch(line)
 if len(m)!=6{return nginxLogEvent{},false}
 ip:=net.ParseIP(m[1])
 if ip==nil||ip.IsUnspecified(){return nginxLogEvent{},false}
 at,err:=time.Parse("02/Jan/2006:15:04:05 -0700",m[2])
 if err!=nil||at.Before(now.Add(-5*time.Minute))||at.After(now.Add(30*time.Second)){return nginxLogEvent{},false}
 ev:=nginxLogEvent{at:at}
 status,err:=strconv.Atoi(m[5])
 if err!=nil{return nginxLogEvent{},false}
 path,_,_:=strings.Cut(m[4],"?")
 if !strings.HasPrefix(path,"/")||len(path)>2048{return ev,true}
 route,ok:=matchNginxPath(path,m[3],status,routes)
 if !ok{return ev,true}
 ev.matched=true
 ev.request=NginxRequest{
    IP:ip.String(),Inbound:route.Inbound,Transport:route.Transport,
    Requests:1,LastSeen:at,
 }
 if (route.Transport=="WebSocket" && status==101) ||
    (route.Transport=="gRPC" && status==200) {ev.request.SuccessHTTP=1}
 return ev,true
}

// AggregateNginxEvents reduces sanitized observations to 5-minute IP+inbound
// rows without summing overlapping snapshots.
func aggregateNginxEvents(events []nginxLogEvent,now time.Time) NginxAccess {
 result:=NginxAccess{CheckedAt:now}
 groups:=make(map[string]*NginxRequest)
 for _,ev:=range events {
   if ev.at.Before(now.Add(-5*time.Minute))||ev.at.After(now.Add(30*time.Second)){continue}
   result.Total++
   if !ev.matched{continue}
   result.Matched++
   key:=ev.request.IP+"\x00"+ev.request.Inbound
   row:=groups[key]
   if row==nil {
     copy:=ev.request
     groups[key]=&copy
   } else {
     row.Requests++
     row.SuccessHTTP+=ev.request.SuccessHTTP
     if ev.at.After(row.LastSeen){row.LastSeen=ev.at}
   }
 }
 for _,row:=range groups {result.Requests=append(result.Requests,*row)}
 sort.Slice(result.Requests,func(i,j int)bool{
    a,b:=result.Requests[i],result.Requests[j]
    if a.Requests!=b.Requests{return a.Requests>b.Requests}
    if !a.LastSeen.Equal(b.LastSeen){return a.LastSeen.After(b.LastSeen)}
    return a.IP<b.IP
 })
 return result
}
func ParseNginxAccess(data []byte,routes []NginxLocation,now time.Time) NginxAccess {
 events:=make([]nginxLogEvent,0)
 scanner:=bufio.NewScanner(strings.NewReader(string(data)))
 scanner.Buffer(make([]byte,4096),512*1024)
 for scanner.Scan(){
   if ev,ok:=parseNginxAccessLine(scanner.Text(),routes,now);ok {events=append(events,ev)}
 }
 return aggregateNginxEvents(events,now)
}

// ReadNginxAccess is bounded to the final 2 MiB; it never reads the entire
// access.log, regardless of file size. The window may therefore cover less
// than five minutes on high-volume hosts. No raw request data is retained.
func ReadNginxAccess(path string, routes []NginxLocation) (NginxAccess,error) {
	now:=time.Now()
	result:=NginxAccess{CheckedAt:now}
	if path=="" {return result,errors.New("NGINX access log disabled")}
	if len(routes)==0 {return result,errors.New("no unambiguous NGINX Xray location routes available")}
	f,err:=os.Open(path)
	if err!=nil {return result,fmt.Errorf("read NGINX access log: %w",err)}
	defer f.Close()
	fi,err:=f.Stat()
	if err!=nil {return result,err}
	const limit int64=2<<20
	size:=fi.Size()
	offset:=int64(0)
	if size>limit {offset=size-limit}
	raw:=make([]byte,size-offset)
	n,err:=f.ReadAt(raw,offset)
	if err!=nil && !errors.Is(err,io.EOF) {return result,err}
	raw=raw[:n]
	if offset>0 {
		if i:=strings.IndexByte(string(raw),'\n');i>=0 {raw=raw[i+1:]}else{raw=nil}
	}
	// Drop a partial final line from a concurrent writer, if any.
	if n:=strings.LastIndexByte(string(raw),'\n');n>=0 {raw=raw[:n+1]} else {raw=nil}
	result=ParseNginxAccess(raw,routes,now)
	result.Truncated=offset>0
	return result,nil
}
