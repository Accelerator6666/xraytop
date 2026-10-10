// Package history persists bounded, local-only source-IP observation snapshots.
// These are not per-IP bandwidth counters or authenticated session records.
package history

import (
    "encoding/json"
    "errors"
    "fmt"
    "net/netip"
    "os"
    "path/filepath"
    "sort"
    "time"

    "github.com/Accelerator6666/xraytop/internal/xray"
)

const (
    Retention = 24 * time.Hour
    MaxIPs = 128
    MaxFileBytes = 16 << 20
    Version = 1
)

type Point struct {
    Minute int64 `json:"m"`
    TCP int `json:"c"`
    Xray int `json:"x"`
    HTTP int `json:"h"`
}

type Record struct {
    IP string `json:"ip"`
    FirstSeen int64 `json:"first"`
    LastSeen int64 `json:"last"`
    Points []Point `json:"points"`
}
type fileData struct {
    Version int `json:"version"`
    Records map[string]*Record `json:"records"`
}

// Summary is a defensive copy so a TUI can sort/filter without mutating storage.
type Summary struct {
    IP string
    FirstSeen time.Time
    LastSeen time.Time
    CurrentTCP int
    PeakTCP int
    LastXray int // latest sampled 5-minute Xray accepted count
    LastHTTP int // latest sampled 5-minute matching HTTP count
    Points []Point
}

type Store struct {
    path string
    tracks map[string]*Record
    lastMinute int64
}

// DefaultPath keeps personally sensitive IP history in the user's private home.
func DefaultPath() string {
    if state:=os.Getenv("XDG_STATE_HOME"); state!="" && filepath.IsAbs(state) {
        return filepath.Join(state,"xraytop","ip-history.json")
    }
    home,err:=os.UserHomeDir()
    if err!=nil {return ""}
    return filepath.Join(home,".local","state","xraytop","ip-history.json")
}

func Open(path string) (*Store,error) {
    s:=&Store{path:path,tracks:make(map[string]*Record)}
    if path=="" {return s,nil}
    info,err:=os.Lstat(path)
    if errors.Is(err,os.ErrNotExist) {return s,nil}
    if err!=nil {return s,err}
    if !info.Mode().IsRegular() {return s,fmt.Errorf("history is not a regular file: %s",path)}
    if info.Size()>MaxFileBytes {return s,fmt.Errorf("history file exceeds %d-byte limit",MaxFileBytes)}
    data,err:=os.ReadFile(path)
    if err!=nil {return s,err}
    if len(data)==0 {return s,nil}
    var disk fileData
    if err:=json.Unmarshal(data,&disk);err!=nil {return s,fmt.Errorf("decode history: %w",err)}
    if disk.Version!=Version {return s,fmt.Errorf("unsupported history version %d",disk.Version)}
    if len(disk.Records)>MaxIPs {return s,fmt.Errorf("history exceeds %d IP limit",MaxIPs)}
    now:=time.Now()
    cutoff:=now.Add(-Retention).Unix()/60
    for ip,rec:=range disk.Records {
        if rec==nil || rec.IP!=ip {continue}
        parsed,e:=netip.ParseAddr(ip)
        if e!=nil || !parsed.IsValid() || parsed.IsUnspecified() {continue}
        rec.Points=prunePoints(rec.Points,cutoff)
        if len(rec.Points)==0 || rec.LastSeen<now.Add(-Retention).Unix() {continue}
        s.tracks[ip]=rec
    }
    return s,nil
}

func prunePoints(points []Point,cutoff int64) []Point {
    // Sanitize untrusted disk content; never expose wild counts or huge arrays.
    clean:=make([]Point,0,min(len(points),1441))
    for _,p:=range points {
        if p.Minute<cutoff || p.TCP<0 || p.Xray<0 || p.HTTP<0 ||
            p.TCP>1_000_000 || p.Xray>1_000_000 || p.HTTP>1_000_000 {continue}
        clean=append(clean,p)
    }
    sort.Slice(clean,func(i,j int)bool{return clean[i].Minute<clean[j].Minute})
    // Deduplicate minute buckets and bound per-IP memory.
    out:=clean[:0]
    for _,p:=range clean {
        if len(out)>0 && out[len(out)-1].Minute==p.Minute {
            out[len(out)-1]=p
        }else {out=append(out,p)}
    }
    if len(out)>1441 {out=out[len(out)-1441:]}
    return out
}

// Observe records a single snapshot per wall-clock minute. TCP is a current
// ESTABLISHED socket count; Xray/HTTP are rolling 5-minute windows and must
// never be summed over samples as if they were unique events.
func (s *Store) Observe(now time.Time,clients []xray.ObservedClient) error {
    if s==nil || s.path=="" {return nil}
    minute:=now.Unix()/60
    if s.lastMinute==minute {return nil}
    s.lastMinute=minute
    cutoff:=now.Add(-Retention).Unix()/60
    for ip,rec:=range s.tracks {
        rec.Points=prunePoints(rec.Points,cutoff)
        if rec.LastSeen<now.Add(-Retention).Unix() || len(rec.Points)==0 {
            delete(s.tracks,ip)
        }
    }
    // Every existing track gets a zero point unless a current observation
    // replaces it. This records disconnects without asserting Xray session ends.
    current:=make(map[string]xray.ObservedClient)
    for _,row:=range clients {
        ip,e:=netip.ParseAddr(row.IP)
        if e!=nil || !ip.IsValid() || ip.IsUnspecified() || row.Evidence()=="-" {continue}
        current[ip.Unmap().String()]=row
    }
    sorted:=make([]xray.ObservedClient,0,len(current))
    for _,row:=range current {sorted=append(sorted,row)}
    // Caller sends XRAY > HTTP > TCP order; preserve that priority if we cap
    // the set. Within current records, prefer recent Xray activity.
    sort.Slice(sorted,func(i,j int)bool {
        priority:=func(v xray.ObservedClient)int {
            if v.Accepted>0{return 3};if v.HTTPRequests>0{return 2};return 1
        }
        if priority(sorted[i])!=priority(sorted[j]) {return priority(sorted[i])>priority(sorted[j])}
        if sorted[i].TCPConnections!=sorted[j].TCPConnections {return sorted[i].TCPConnections>sorted[j].TCPConnections}
        return sorted[i].IP<sorted[j].IP
    })
    for _,row:=range sorted {
        ip,err:=netip.ParseAddr(row.IP)
        if err!=nil {continue}
        key:=ip.Unmap().String()
        rec:=s.tracks[key]
        if rec==nil {
            if len(s.tracks)>=MaxIPs {
                // Evict least-recently observed IP, not a currently matched one
                // when there is a dormant record available.
                var oldest string
                var at int64
                for k,r:=range s.tracks {
                    _,active:=current[k]
                    if active {continue}
                    if oldest=="" || r.LastSeen<at {oldest,at=k,r.LastSeen}
                }
                if oldest=="" {continue}
                delete(s.tracks,oldest)
            }
            rec=&Record{IP:key,FirstSeen:now.Unix()}
            s.tracks[key]=rec
        }
        seen:=now.Unix()
        // Xray/NGINX events may be older than the sampling instant.
        // Only TCP observation can be considered current at sampling time.
        if row.TCPConnections==0 {
            seen=0
            if !row.LastSeen.IsZero() {seen=row.LastSeen.Unix()}
            if row.HTTPLastSeen.Unix()>seen {seen=row.HTTPLastSeen.Unix()}
        }
        if seen>rec.LastSeen {rec.LastSeen=seen}
    }
    for ip,rec:=range s.tracks {
        row:=current[ip]
        p:=Point{Minute:minute,TCP:row.TCPConnections,Xray:row.Accepted,HTTP:row.HTTPRequests}
        if len(rec.Points)>0 && rec.Points[len(rec.Points)-1].Minute==minute {
            rec.Points[len(rec.Points)-1]=p
        }else {rec.Points=append(rec.Points,p)}
        rec.Points=prunePoints(rec.Points,cutoff)
    }
    return s.save()
}

func (s *Store) Summaries() []Summary {
    if s==nil {return nil}
    out:=make([]Summary,0,len(s.tracks))
    for _,r:=range s.tracks {
        if len(r.Points)==0 {continue}
        row:=Summary{IP:r.IP,FirstSeen:time.Unix(r.FirstSeen,0),LastSeen:time.Unix(r.LastSeen,0),
            Points:append([]Point(nil),r.Points...)}
        for _,p:=range row.Points {if p.TCP>row.PeakTCP {row.PeakTCP=p.TCP}}
        last:=row.Points[len(row.Points)-1]
        row.CurrentTCP,row.LastXray,row.LastHTTP=last.TCP,last.Xray,last.HTTP
        out=append(out,row)
    }
    sort.Slice(out,func(i,j int)bool {
        if !out[i].LastSeen.Equal(out[j].LastSeen) {return out[i].LastSeen.After(out[j].LastSeen)}
        return out[i].IP<out[j].IP
    })
    return out
}

func (s *Store) save() error {
    if err:=os.MkdirAll(filepath.Dir(s.path),0700);err!=nil {return err}
    disk:=fileData{Version:Version,Records:s.tracks}
    encoded,err:=json.Marshal(disk)
    if err!=nil {return err}
    if len(encoded)>MaxFileBytes {return fmt.Errorf("history capacity exceeded (%d bytes)",len(encoded))}
    f,err:=os.CreateTemp(filepath.Dir(s.path),".xraytop-*.tmp")
    if err!=nil {return err}
    tmp:=f.Name()
    defer os.Remove(tmp)
    if err=f.Chmod(0600);err==nil {_,err=f.Write(encoded)}
    if err==nil {err=f.Sync()}
    closeErr:=f.Close()
    if err!=nil {return err}
    if closeErr!=nil {return closeErr}
    return os.Rename(tmp,s.path)
}
