package xray

import (
 "bufio"
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net"
 "os/exec"
 "sort"
 "strconv"
 "strings"
 "time"
 "unicode"
)

// JournalTailer stores only accepted-event IP/inbound/time metadata in memory;
// never stores UUIDs, destinations or entire journal messages. Cursor is not
// persisted across daemon restarts: startup replays the latest five minutes.
type JournalTailer struct {
 unit string
 cursor string
 events []RecentSource
 incompleteUntil time.Time
}
const journalMaxBatchLines=10000
const journalMaxBatchBytes=8<<20
const journalMaxEvents=100000

func parseAcceptedJournalMessage(message string,at time.Time)(RecentSource,bool){
 fromIndex:=strings.Index(message," from ")
 if fromIndex<0{return RecentSource{},false}
 endpoint,remaining,ok:=strings.Cut(message[fromIndex+len(" from "):]," accepted ")
 if !ok{return RecentSource{},false}
 host,_,err:=net.SplitHostPort(endpoint)
 if err!=nil{return RecentSource{},false}
 ip:=net.ParseIP(host)
 if ip==nil||ip.IsUnspecified(){return RecentSource{},false}
 start:=strings.LastIndex(remaining," [")
 if start<0{return RecentSource{},false}
 tag,_,ok:=strings.Cut(remaining[start+2:]," >> ")
 if !ok{return RecentSource{},false}
 tag=strings.TrimSpace(tag)
 if tag==""||len(tag)>100||strings.IndexFunc(tag,func(r rune)bool{return unicode.IsControl(r)})!=-1 {
   return RecentSource{},false
 }
 return RecentSource{IP:ip.String(),Inbound:tag,Accepted:1,FirstSeen:at,LastSeen:at},true
}

func buildRecentActivity(events []RecentSource, now time.Time) RecentActivity {
 res:=RecentActivity{CheckedAt:now}
 groups:=make(map[string]*RecentSource)
 cutoff:=now.Add(-5*time.Minute)
 for _,ev:=range events {
   if ev.LastSeen.Before(cutoff)||ev.LastSeen.After(now.Add(30*time.Second)){continue}
   res.Events++
   key:=ev.IP+"\x00"+ev.Inbound
   group:=groups[key]
   if group==nil {v:=ev;groups[key]=&v} else {
     group.Accepted++
     if ev.LastSeen.Before(group.FirstSeen){group.FirstSeen=ev.LastSeen}
     if ev.LastSeen.After(group.LastSeen){group.LastSeen=ev.LastSeen}
   }
 }
 for _,r:=range groups{res.Sources=append(res.Sources,*r)}
 sort.Slice(res.Sources,func(i,j int)bool{
   a,b:=res.Sources[i],res.Sources[j]
   if !a.LastSeen.Equal(b.LastSeen){return a.LastSeen.After(b.LastSeen)}
   if a.Accepted!=b.Accepted{return a.Accepted>b.Accepted}
   if a.IP!=b.IP{return a.IP<b.IP}
   return a.Inbound<b.Inbound
 })
 return res
}

// Read executes journalctl without a shell. It reads JSON lines with a
// bounded stream and advances by __CURSOR; no arbitrary journal entries are
// persisted. Errors are explicit and do not silently fabricate an empty log.
func (t *JournalTailer) Read(ctx context.Context,unit string)(RecentActivity,error){
 now:=time.Now()
 zero:=RecentActivity{CheckedAt:now}
 if !validJournalUnit(unit){return zero,errors.New("invalid local journal unit name")}
 if t.unit!=unit{t.unit=unit;t.cursor="";t.events=nil}
 args:=[]string{"--unit",unit,"--no-pager","--quiet","--output=json"}
 if t.cursor!=""{args=append(args,"--after-cursor",t.cursor)}else{
   args=append(args,"--since","5 minutes ago")
 }
 cmd:=exec.CommandContext(ctx,"journalctl",args...)
 pipe,err:=cmd.StdoutPipe()
 if err!=nil{return zero,err}
 var stderr bytes.Buffer
 cmd.Stderr=&stderr
 if err=cmd.Start();err!=nil{return zero,fmt.Errorf("journalctl start: %w",err)}
 scanner:=bufio.NewScanner(pipe)
 scanner.Buffer(make([]byte,4096),512<<10)
 count,total:=0,0
 overflow:=false
 events:=make([]RecentSource,0)
 cursor:=t.cursor
 for scanner.Scan(){
   line:=scanner.Bytes()
   count++
   total+=len(line)
   if count>journalMaxBatchLines||total>journalMaxBatchBytes {
      overflow=true
      break
   }
   var item struct{
      Cursor string `json:"__CURSOR"`
      Message string `json:"MESSAGE"`
      Timestamp string `json:"__REALTIME_TIMESTAMP"`
   }
   if e:=json.Unmarshal(line,&item);e!=nil {continue}
   if item.Cursor!=""{cursor=item.Cursor}
   micro,e:=strconv.ParseInt(item.Timestamp,10,64)
   if e!=nil{continue}
   at:=time.UnixMicro(micro)
   if ev,ok:=parseAcceptedJournalMessage(item.Message,at);ok {
      events=append(events,ev)
   }
 }
 scanErr:=scanner.Err()
 if overflow{_ =cmd.Process.Kill()}
 waitErr:=cmd.Wait()
 if scanErr!=nil{return zero,fmt.Errorf("journalctl stream: %w",scanErr)}
 if waitErr!=nil&&!overflow {
    detail:=strings.TrimSpace(stderr.String())
    if len(detail)>180{detail=detail[:180]}
    return zero,fmt.Errorf("journalctl: %s: %w",detail,waitErr)
 }
 t.cursor=cursor
 t.events=append(t.events,events...)
 keep:=t.events[:0]
 for _,ev:=range t.events{
   if !ev.LastSeen.Before(now.Add(-5*time.Minute))&&!ev.LastSeen.After(now.Add(30*time.Second)){keep=append(keep,ev)}
 }
 t.events=keep
 if len(t.events)>journalMaxEvents{
   t.events=append([]RecentSource(nil),t.events[len(t.events)-journalMaxEvents:]...)
   t.incompleteUntil=now.Add(5*time.Minute)
 }
 if overflow{t.incompleteUntil=now.Add(5*time.Minute)}
 res:=buildRecentActivity(t.events,now)
 res.Truncated=now.Before(t.incompleteUntil)
 return res,nil
}
