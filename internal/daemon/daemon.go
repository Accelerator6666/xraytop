// Package daemon provides a strictly local, read-only-to-Xray background
// snapshot producer for a separate interactive viewer.
package daemon

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "syscall"
 "time"

 "github.com/Accelerator6666/xraytop/internal/collector"
 "github.com/Accelerator6666/xraytop/internal/history"
)

const MaxSnapshotBytes = 8 << 20
const Version = 1
const MaxSnapshotAge = 15 * time.Second

type envelope struct {
 Version int `json:"version"`
 Snapshot collector.Snapshot `json:"snapshot"`
}

// Lock prevents concurrent background producers from writing the same
// snapshot/history. The lock is held until Close; it does not touch Xray.
func Lock(snapshotPath string) (*os.File,error) {
 dir:=filepath.Dir(snapshotPath)
 if err:=os.MkdirAll(dir,0700);err!=nil{return nil,err}
 path:=filepath.Join(dir,"snapshot.lock")
 f,err:=os.OpenFile(path,os.O_RDWR|os.O_CREATE,0600)
 if err!=nil{return nil,err}
 if err=syscall.Flock(int(f.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);err!=nil {
  _=f.Close()
  return nil,fmt.Errorf("another xraytop daemon holds %s: %w",path,err)
 }
 return f,nil
}

func Write(snapshotPath string,snapshot collector.Snapshot) error {
 // History is persisted separately once per minute. Avoid rewriting large
 // 24-hour chart arrays in each 2-second snapshot.
 snapshot.IPHistory=nil
 payload,err:=json.Marshal(envelope{Version:Version,Snapshot:snapshot})
 if err!=nil{return err}
 if len(payload)>MaxSnapshotBytes{return fmt.Errorf("snapshot exceeds %d bytes",MaxSnapshotBytes)}
 dir:=filepath.Dir(snapshotPath)
 if err:=os.MkdirAll(dir,0700);err!=nil{return err}
 f,err:=os.CreateTemp(dir,".snapshot-*.tmp")
 if err!=nil{return err}
 tmp:=f.Name()
 defer os.Remove(tmp)
 if err=f.Chmod(0600);err==nil {_,err=f.Write(payload)}
 if err==nil {err=f.Sync()}
 closeErr:=f.Close()
 if err!=nil{return err}
 if closeErr!=nil{return closeErr}
 return os.Rename(tmp,snapshotPath)
}

// Read rejects stale snapshots: old process data is never displayed as ONLINE.
// Only local regular files are accepted; no external service or API exposure.
func Read(snapshotPath string) (collector.Snapshot,error) {
 var zero collector.Snapshot
 fi,err:=os.Lstat(snapshotPath)
 if err!=nil{return zero,err}
 if !fi.Mode().IsRegular(){return zero,fmt.Errorf("not a regular snapshot file")}
 if fi.Size()>MaxSnapshotBytes {return zero,fmt.Errorf("snapshot exceeds maximum size")}
 data,err:=os.ReadFile(snapshotPath)
 if err!=nil{return zero,err}
 var env envelope
 if err=json.Unmarshal(data,&env);err!=nil{return zero,fmt.Errorf("decode daemon snapshot: %w",err)}
 if env.Version!=Version {return zero,fmt.Errorf("daemon snapshot version %d unsupported",env.Version)}
 if env.Snapshot.At.IsZero(){return zero,errors.New("daemon snapshot has no timestamp")}
 age:=time.Since(env.Snapshot.At)
 if age>MaxSnapshotAge || age < -5*time.Second {
  return zero,fmt.Errorf("daemon snapshot is stale (%s old)",age.Round(time.Second))
 }
 return env.Snapshot,nil
}

// Follow exclusively reads daemon snapshots and its history store; it NEVER
// starts local Xray/nginx/journal collectors or writes history.
type Follow struct {
 SnapshotPath string
 HistoryPath string
 history []history.Summary
 nextHistory time.Time
 historyError string
}

func (f *Follow) Sample(_ context.Context) collector.Snapshot {
 snap,err:=Read(f.SnapshotPath)
 if err!=nil{return collector.Snapshot{At:time.Now(),HistoryError:"daemon: "+err.Error(),DaemonError:err.Error()}}
 if f.HistoryPath!="" && time.Now().After(f.nextHistory) {
  store,e:=history.Open(f.HistoryPath)
  if e!=nil {f.historyError="load daemon history: "+e.Error()} else {
   f.history=store.Summaries()
   f.historyError=""
  }
  f.nextHistory=time.Now().Add(10*time.Second)
 }
 snap.IPHistory=f.history
 if f.historyError!="" {snap.HistoryError=f.historyError}
 return snap
}

func Run(ctx context.Context,c *collector.Collector,snapshotPath string,interval time.Duration) error {
 if c==nil {return errors.New("nil collector")}
 if snapshotPath=="" {return errors.New("empty daemon snapshot path")}
 if interval<time.Second {return errors.New("daemon interval must be at least 1s")}
 lock,err:=Lock(snapshotPath)
 if err!=nil{return err}
 defer lock.Close()
 ticker:=time.NewTicker(interval)
 defer ticker.Stop()
 for {
  queryCtx,cancel:=context.WithTimeout(ctx,3*time.Second)
  snap:=c.Sample(queryCtx)
  cancel()
  if err:=Write(snapshotPath,snap);err!=nil{return fmt.Errorf("publish daemon snapshot: %w",err)}
  select {
  case <-ctx.Done():return nil
  case <-ticker.C:
  }
 }
}
