package xray

import (
 "errors"
 "fmt"
 "io"
 "os"
 "strings"
 "time"
)

// NginxTailer maintains a bounded, in-memory, five-minute metadata window.
// Unlike repeated 2-MiB tail scans, it reads only bytes appended since the
// previous poll. A cold start is bootstrapped from the final 2 MiB.
// Any incomplete window (rotation/copytruncate/overflow) is flagged instead
// of silently reporting exact complete counts.
type NginxTailer struct {
 file *os.File
 identity os.FileInfo
 path string
 offset int64
 partial []byte
 events []nginxLogEvent
 incompleteUntil time.Time
}
const nginxTailBootstrapBytes int64=2<<20
const nginxTailReadLimit int64=4<<20
const nginxMaxEvents=100000
const nginxMaxPartial=16<<10

func (t *NginxTailer) Close() error {
 if t.file==nil{return nil}
 err:=t.file.Close()
 t.file=nil
 return err
}
func (t *NginxTailer) readChunk(now time.Time,routes []NginxLocation,size int64)error{
 remaining:=size-t.offset
 if remaining<=0{return nil}
 if remaining>nginxTailReadLimit {
   t.offset=size-nginxTailBootstrapBytes
   t.partial=nil
   t.incompleteUntil=now.Add(5*time.Minute)
   remaining=nginxTailBootstrapBytes
 }
 buf:=make([]byte,remaining)
 n,err:=t.file.ReadAt(buf,t.offset)
 if err!=nil && !errors.Is(err,io.EOF){return err}
 if n<=0{return nil}
 start:=t.offset
 t.offset+=int64(n)
 buf=buf[:n]
 if len(t.partial)==0 && start!=0 {
   // Verify the first byte belongs to a new line, not the middle of a
   // partially skipped log entry.
   prev:=[]byte{0}
   if _,e:=t.file.ReadAt(prev,start-1);e==nil && prev[0]!='\n' {
     i:=strings.IndexByte(string(buf),'\n')
     if i<0 {return nil}
     buf=buf[i+1:]
   }
 }
 buf=append(t.partial,buf...)
 t.partial=nil
 end:=0
 for i,c:=range buf {
   if c!='\n'{continue}
   if i-end<=nginxMaxPartial {
     if event,ok:=parseNginxAccessLine(string(buf[end:i]),routes,now);ok {
       t.events=append(t.events,event)
     }
   }else{t.incompleteUntil=now.Add(5*time.Minute)}
   end=i+1
 }
 if len(buf)-end>nginxMaxPartial {
   t.incompleteUntil=now.Add(5*time.Minute)
 }else{
   t.partial=append([]byte(nil),buf[end:]...)
 }
 return nil
}
func (t *NginxTailer) Read(path string,routes []NginxLocation,now time.Time)(NginxAccess,error) {
 result:=NginxAccess{CheckedAt:now}
 if path==""{return result,errors.New("NGINX access log disabled")}
 if len(routes)==0{return result,errors.New("no unambiguous NGINX Xray locations")}
 fi,err:=os.Stat(path)
 if err!=nil{return result,fmt.Errorf("read NGINX access log: %w",err)}
 if !fi.Mode().IsRegular(){return result,errors.New("NGINX access log is not regular")}
 if t.file==nil || t.path!=path || !os.SameFile(fi,t.identity){
   // Read any last bytes from the old inode before accepting the new file.
   if t.file!=nil {
     if old,er:=t.file.Stat();er==nil {_=t.readChunk(now,routes,old.Size())}
     _=t.Close()
     t.incompleteUntil=now.Add(5*time.Minute) // rotation between polls may lose events
   }
   f,e:=os.Open(path)
   if e!=nil{return result,fmt.Errorf("read NGINX access log: %w",e)}
   t.file=f
   t.path=path
   t.identity=fi
   t.offset=0
   t.partial=nil
   if fi.Size()>nginxTailBootstrapBytes {
     t.offset=fi.Size()-nginxTailBootstrapBytes
     t.incompleteUntil=now.Add(5*time.Minute)
   }
 }else if fi.Size()<t.offset {
   // copytruncate or replacement with same inode: reset cursor, do not
   // claim the last 5 minutes were completely observed.
   t.offset=0
   t.partial=nil
   t.incompleteUntil=now.Add(5*time.Minute)
 }
 if err:=t.readChunk(now,routes,fi.Size());err!=nil{return result,fmt.Errorf("NGINX incremental read: %w",err)}
 kept:=t.events[:0]
 cutoff:=now.Add(-5*time.Minute)
 for _,ev:=range t.events {
   if !ev.at.Before(cutoff) && !ev.at.After(now.Add(30*time.Second)){kept=append(kept,ev)}
 }
 t.events=kept
 if len(t.events)>nginxMaxEvents {
   t.events=append([]nginxLogEvent(nil),t.events[len(t.events)-nginxMaxEvents:]...)
   t.incompleteUntil=now.Add(5*time.Minute)
 }
 result=aggregateNginxEvents(t.events,now)
 result.Truncated=now.Before(t.incompleteUntil)
 return result,nil
}
