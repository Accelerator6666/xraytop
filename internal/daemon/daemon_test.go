package daemon

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/Accelerator6666/xraytop/internal/collector"
 "github.com/Accelerator6666/xraytop/internal/history"
)

func TestSnapshotRoundtripAndNoHistoryBloat(t *testing.T) {
 file:=filepath.Join(t.TempDir(),"private","snapshot.json")
 now:=time.Now()
 snap:=collector.Snapshot{At:now,PID:1234,APIEnabled:true,IPHistory:[]history.Summary{{IP:"192.0.2.1"}}}
 if err:=Write(file,snap);err!=nil{t.Fatal(err)}
 read,e:=Read(file)
 if e!=nil || read.PID!=1234 || !read.APIEnabled || len(read.IPHistory)!=0 {t.Fatalf("%+v %v",read,e)}
 st,e:=os.Stat(file)
 if e!=nil || st.Mode().Perm()!=0600 {t.Fatalf("private state mode %v %v",st,e)}
 if err:=Write(file,collector.Snapshot{At:now,PID:5678});err!=nil{t.Fatal(err)}
 read,e=Read(file)
 if e!=nil || read.PID!=5678 {t.Fatalf("atomic overwrite %+v %v",read,e)}
}
func TestStaleSnapshotAndCorrupt(t *testing.T) {
 file:=filepath.Join(t.TempDir(),"snapshot.json")
 if err:=Write(file,collector.Snapshot{At:time.Now().Add(-time.Hour)});err!=nil{t.Fatal(err)}
 if _,err:=Read(file);err==nil || !strings.Contains(err.Error(),"stale"){t.Fatalf("want stale error, got %v",err)}
 if err:=os.WriteFile(file,[]byte("bad-json"),0600);err!=nil{t.Fatal(err)}
 if _,err:=Read(file);err==nil{t.Fatal("accepted corrupt json")}
}
func TestLockAndFollow(t *testing.T) {
 file:=filepath.Join(t.TempDir(),"snapshot.json")
 lock,err:=Lock(file)
 if err!=nil{t.Fatal(err)}
 defer lock.Close()
 if another,err:=Lock(file);err==nil{another.Close();t.Fatal("second daemon obtained same lock")}
 hfile:=filepath.Join(filepath.Dir(file),"history.json")
 hs,e:=history.Open(hfile);if e!=nil{t.Fatal(e)}
 if err:=hs.Observe(time.Now(),nil);err!=nil{t.Fatal(err)}
 if err:=Write(file,collector.Snapshot{At:time.Now(),PID:77});err!=nil{t.Fatal(err)}
 follow:=&Follow{SnapshotPath:file,HistoryPath:hfile}
 snap:=follow.Sample(context.Background())
 if snap.PID!=77 || snap.HistoryError!="" {t.Fatalf("%+v",snap)}
}
