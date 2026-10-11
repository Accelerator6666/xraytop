// Package doctor performs read-only health checks for the local collector.
// It never changes Xray, NGINX, systemd or filesystem permissions.
package doctor

import (
 "context"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "time"

 "github.com/Accelerator6666/xraytop/internal/daemon"
 "github.com/Accelerator6666/xraytop/internal/xray"
)

type Options struct {
 SnapshotFile string
 XrayConfig string
 ConfigDir string
 NginxConfig string
 AccessLog string
 XrayBin string
 API string
 Service string
}
type Check struct {
 Name string
 Status string // PASS, WARN, FAIL
 Detail string
}
type Report struct { Checks []Check }
func (r *Report) add(name,status,detail string) {
 // Only known constant messages or shortened Go errors; never expose config data.
 if len(detail)>280 {detail=detail[:280]+"..."}
 detail=strings.ReplaceAll(strings.ReplaceAll(detail,"\r"," "),"\n"," ")
 r.Checks=append(r.Checks,Check{name,status,detail})
}
func (r Report) Failures() int {n:=0;for _,c:=range r.Checks {if c.Status=="FAIL"{n++}};return n}
func (r Report) String() string {
 var b strings.Builder
 b.WriteString("XRAYTOP DOCTOR (read-only; does not modify or restart services)\n")
 for _,c:=range r.Checks {fmt.Fprintf(&b,"[%s] %-17s %s\n",c.Status,c.Name,c.Detail)}
 return b.String()
}

func Run(ctx context.Context,opt Options) Report {
 r:=Report{}
 if opt.Service=="" {opt.Service="xraytop-collector.service"}
 if opt.SnapshotFile=="" {opt.SnapshotFile="/var/lib/xraytop/snapshot.json"}
 if opt.NginxConfig=="" {opt.NginxConfig="/etc/nginx/nginx.conf"}
 // Test the systemd unit without any restart or privilege escalation.
 taskCtx,cancel:=context.WithTimeout(ctx,2*time.Second)
 out,err:=exec.CommandContext(taskCtx,"systemctl","is-active",opt.Service).CombinedOutput()
 cancel()
 if err!=nil {
   r.add("Collector","FAIL",fmt.Sprintf("%s is not active (%s)",opt.Service,strings.TrimSpace(string(out))))
 } else if strings.TrimSpace(string(out))=="active" {
   r.add("Collector","PASS",opt.Service+" is active")
 } else {
   r.add("Collector","FAIL",fmt.Sprintf("%s status %s",opt.Service,strings.TrimSpace(string(out))))
 }
 snapshot,err:=daemon.Read(opt.SnapshotFile)
 if err!=nil {r.add("Snapshot","FAIL",err.Error())} else {
   r.add("Snapshot","PASS",fmt.Sprintf("fresh sample %s",snapshot.At.Local().Format(time.RFC3339)))
   for _,part:=range []struct{name,errorText string}{
     {"Xray stats",snapshot.APIError},{"NGINX routes",snapshot.NginxError},
     {"TCP",""+snapshot.LiveError},{"NGINX access",snapshot.AccessError},
     {"Xray journal",snapshot.RecentError},{"History",snapshot.HistoryError},
   } {
     if part.errorText!="" {r.add(part.name,"WARN",part.errorText)}
   }
   if snapshot.NginxError=="" {
     r.add("Ingress", "PASS",fmt.Sprintf("%d NGINX ports / %d verified routes",len(snapshot.Nginx.Ports),len(snapshot.Nginx.Routes)))
   }
 }
 if opt.XrayConfig=="" && opt.ConfigDir=="" {
    r.add("Xray config","WARN","no explicit config path; use --config or --config-dir for file validation")
 }else{
   var paths []string
   if opt.XrayConfig!="" {paths=[]string{opt.XrayConfig}}
   cat,err:=xray.LoadCatalog(paths,opt.ConfigDir)
   if err!=nil {r.add("Xray config","FAIL",err.Error())}else{
     r.add("Xray config","PASS",fmt.Sprintf("%d inbounds / %d outbounds",len(cat.Inbounds),len(cat.Outbounds)))
     nginx,err:=xray.DiscoverNginxIngressFromFile(ctx,cat,opt.NginxConfig)
     if err!=nil {r.add("NGINX config","WARN",err.Error())}else {
       r.add("NGINX config","PASS",fmt.Sprintf("%d matching ports / %d routes",len(nginx.Ports),len(nginx.Routes)))
     }
   }
 }
 if opt.AccessLog!="" {
   f,err:=os.Open(opt.AccessLog)
   if err!=nil {r.add("Access log","WARN",err.Error())}else{
     info,err:=f.Stat();_ =f.Close()
     if err!=nil {r.add("Access log","WARN",err.Error())} else if !info.Mode().IsRegular() {
       r.add("Access log","WARN","not a regular file")
     } else {r.add("Access log","PASS",fmt.Sprintf("readable (%d bytes)",info.Size()))}
   }
 }
 if opt.API=="" {r.add("StatsService","WARN","disabled with --api ''")} else {
   binary:=opt.XrayBin
   if binary==""{binary="xray"}
   apiCtx,cancel:=context.WithTimeout(ctx,3*time.Second)
   stats,err:=xray.Client{Binary:binary,Server:opt.API}.Query(apiCtx)
   cancel()
   if err!=nil {r.add("StatsService","WARN",err.Error())}else{
     r.add("StatsService","PASS",fmt.Sprintf("%d counters at %s",len(stats),opt.API))
   }
 }
 // Provide the local data directory explicitly, never list IP addresses.
 dir:=filepath.Dir(opt.SnapshotFile)
 if fi,err:=os.Stat(dir);err==nil && fi.IsDir() {
   if fi.Mode().Perm()&0007!=0 {r.add("State directory","WARN","directory has permissions for others")}
   else {r.add("State directory","PASS",dir)}
 }else {r.add("State directory","WARN",dir+" unavailable")}
 return r
}
