package xray

import (
 "os"
 "path/filepath"
 "testing"
 "time"
)
func makeNginxLine(at time.Time,ip string)string{
 return ip+" - - ["+at.Format("02/Jan/2006:15:04:05 -0700")+"] \"GET /ws HTTP/1.1\" 101 0 \"-\" \"-\"\n"
}
func TestNginxTailerAppendAndRotation(t *testing.T){
 dir:=t.TempDir();path:=filepath.Join(dir,"access.log");now:=time.Now()
 routes:=[]NginxLocation{{Pattern:"/ws",Inbound:"ws",Transport:"WebSocket"}}
 if err:=os.WriteFile(path,[]byte(makeNginxLine(now,"192.0.2.1")),0600);err!=nil{t.Fatal(err)}
 var tail NginxTailer;defer tail.Close()
 r,e:=tail.Read(path,routes,now);if e!=nil||r.Matched!=1{t.Fatalf("first %+v %v",r,e)}
 r,e=tail.Read(path,routes,now);if e!=nil||r.Matched!=1{t.Fatalf("double-count %+v %v",r,e)}
 f,e:=os.OpenFile(path,os.O_APPEND|os.O_WRONLY,0600);if e!=nil{t.Fatal(e)}
 if _,e=f.WriteString(makeNginxLine(now,"192.0.2.2"));e!=nil{t.Fatal(e)}
 _=f.Close()
 r,e=tail.Read(path,routes,now);if e!=nil||r.Matched!=2{t.Fatalf("append %+v %v",r,e)}
 if e=os.Rename(path,path+".1");e!=nil{t.Fatal(e)}
 if e=os.WriteFile(path,[]byte(makeNginxLine(now,"192.0.2.3")),0600);e!=nil{t.Fatal(e)}
 r,e=tail.Read(path,routes,now);if e!=nil||r.Matched!=3||!r.Truncated{t.Fatalf("rotation %+v %v",r,e)}
}
func TestNginxTailerPartialLineAndWindow(t *testing.T){
 dir:=t.TempDir();path:=filepath.Join(dir,"access.log");now:=time.Now()
 routes:=[]NginxLocation{{Pattern:"/ws",Inbound:"ws",Transport:"WebSocket"}}
 l:=makeNginxLine(now,"198.51.100.7")
 if e:=os.WriteFile(path,[]byte(l[:len(l)-1]),0600);e!=nil{t.Fatal(e)}
 var tail NginxTailer;defer tail.Close()
 r,e:=tail.Read(path,routes,now);if e!=nil||r.Matched!=0{t.Fatalf("partial %+v %v",r,e)}
 f,e:=os.OpenFile(path,os.O_APPEND|os.O_WRONLY,0600);if e!=nil{t.Fatal(e)}
 _,e=f.WriteString("\n");_ =f.Close();if e!=nil{t.Fatal(e)}
 r,e=tail.Read(path,routes,now);if e!=nil||r.Matched!=1{t.Fatalf("joined %+v %v",r,e)}
 r,e=tail.Read(path,routes,now.Add(6*time.Minute));if e!=nil||r.Matched!=0{t.Fatalf("expired %+v %v",r,e)}
}
