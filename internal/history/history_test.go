package history

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"

    "github.com/Accelerator6666/xraytop/internal/xray"
)

func TestHistoryPersistenceAndMissingMinute(t *testing.T) {
    name:=filepath.Join(t.TempDir(),"state","ip-history.json")
    store,err:=Open(name);if err!=nil {t.Fatal(err)}
    now:=time.Now().Truncate(time.Minute)
    rows:=[]xray.ObservedClient{{IP:"192.0.2.11",TCPConnections:4,Accepted:2,LastSeen:now}}
    if err:=store.Observe(now,rows);err!=nil {t.Fatal(err)}
    if err:=store.Observe(now.Add(15*time.Second),rows);err!=nil {t.Fatal(err)}
    one:=store.Summaries()
    if len(one)!=1 || len(one[0].Points)!=1 || one[0].CurrentTCP!=4 {t.Fatalf("%+v",one)}
    if err:=store.Observe(now.Add(time.Minute),nil);err!=nil {t.Fatal(err)}
    reopened,err:=Open(name);if err!=nil {t.Fatal(err)}
    got:=reopened.Summaries()
    if len(got)!=1 || got[0].CurrentTCP!=0 || got[0].PeakTCP!=4 || len(got[0].Points)!=2 {t.Fatalf("reopen %+v",got)}
    st,err:=os.Stat(name);if err!=nil {t.Fatal(err)}
    if st.Mode().Perm()!=0600 {t.Fatalf("perms %s",st.Mode().Perm())}
    if err:=store.Observe(now.Add(2*time.Minute),[]xray.ObservedClient{{IP:"198.51.100.9",HTTPRequests:3,HTTPLastSeen:now.Add(2*time.Minute)}});err!=nil{t.Fatal(err)}
    got=store.Summaries()
    if len(got)!=2 {t.Fatalf("%+v",got)}
}

func TestHistoryRetentionAndCap(t *testing.T) {
    s:=&Store{path:filepath.Join(t.TempDir(),"hist.json"),tracks:make(map[string]*Record)}
    now:=time.Now().Truncate(time.Minute)
    s.tracks["192.0.2.1"]=&Record{IP:"192.0.2.1",FirstSeen:now.Add(-48*time.Hour).Unix(),LastSeen:now.Add(-48*time.Hour).Unix(),Points:[]Point{{Minute:now.Add(-48*time.Hour).Unix()/60,TCP:1}}}
    if err:=s.Observe(now,[]xray.ObservedClient{{IP:"198.51.100.1",TCPConnections:1}});err!=nil{t.Fatal(err)}
    if len(s.Summaries())!=1 || s.Summaries()[0].IP!="198.51.100.1" {t.Fatalf("%+v",s.Summaries())}
}
func TestHistoryRejectsCorruptOrSymlink(t *testing.T) {
    root:=t.TempDir()
    path:=filepath.Join(root,"file.json")
    if err:=os.WriteFile(path,[]byte("not-json"),0600);err!=nil{t.Fatal(err)}
    if _,err:=Open(path);err==nil {t.Fatal("must report invalid JSON")}
    target:=filepath.Join(root,"target")
    if err:=os.WriteFile(target,[]byte("ok"),0600);err!=nil{t.Fatal(err)}
    if err:=os.Remove(path);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(target,path);err!=nil{t.Fatal(err)}
    if _,err:=Open(path);err==nil {t.Fatal("must reject symlink")}
    if !strings.Contains(DefaultPath(),"xraytop") {t.Fatal("bad default history path")}
}
func TestInvalidIPNotStored(t *testing.T) {
    store,_:=Open(filepath.Join(t.TempDir(),"state.json"))
    err:=store.Observe(time.Now(),[]xray.ObservedClient{{IP:"not-an-ip",TCPConnections:3},{IP:"127.0.0.1",TCPConnections:1}})
    if err!=nil {t.Fatal(err)}
    if len(store.Summaries())!=1 {t.Fatal(store.Summaries())}
}
