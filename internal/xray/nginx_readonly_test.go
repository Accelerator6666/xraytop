package xray

import (
    "context"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "testing"
)

func TestReadOnlyNginxIncludesFifteenPorts(t *testing.T) {
    dir:=t.TempDir()
    if err:=os.MkdirAll(filepath.Join(dir,"sites-enabled"),0700);err!=nil{t.Fatal(err)}
    main:="events {} \nhttp { include conf.d.conf; include sites-enabled/*; }\n"
    upstream:="upstream xray_vless_ws_9930 { server unix:/dev/shm/Xray-VLESS-WS.socket; }\n"
    var server strings.Builder
    server.WriteString("server {")
    for i:=1;i<=15;i++ {server.WriteString(" listen "+itoa(i*1000+443)+" ssl http2;")}
    server.WriteString("location /ws { proxy_pass http://xray_vless_ws_9930; } ")
    server.WriteString("location /grpc { grpc_pass unix:/dev/shm/Xray-VLESS-gRPC.socket; } }")
    for file,content:=range map[string]string{
       "nginx.conf":main,
       "conf.d.conf":upstream,
       "sites-enabled/default":server.String(),
    } {
       path:=filepath.Join(dir,file)
       if err:=os.WriteFile(path,[]byte(content),0600);err!=nil{t.Fatal(err)}
    }
    cat:=Catalog{Inbounds:[]Endpoint{
        {Tag:"ws",Listen:"unix:/dev/shm/Xray-VLESS-WS.socket",Transport:"WebSocket"},
        {Tag:"grpc",Listen:"unix:/dev/shm/Xray-VLESS-gRPC.socket",Transport:"gRPC"},
    }}
    result,err:=DiscoverNginxIngressFromFile(context.Background(),cat,filepath.Join(dir,"nginx.conf"))
    if err!=nil{t.Fatal(err)}
    if len(result.Ports)!=15 || len(result.Routes)!=30 || len(result.Locations)!=2 {
        t.Fatalf("ports %v routes %v locations %v",result.Ports,result.Routes,result.Locations)
    }
}
func itoa(n int)string {return strconv.Itoa(n)}

func TestNginxReadOnlyDiscoveryRejectsMissingIncludeAndCycle(t *testing.T) {
    dir:=t.TempDir()
    main:=filepath.Join(dir,"nginx.conf")
    if err:=os.WriteFile(main,[]byte("http { include missing.conf; }"),0600);err!=nil{t.Fatal(err)}
    if _,err:=ReadNginxConfigTree(context.Background(),main);err==nil{t.Fatal("missing included file must error")}
    if err:=os.WriteFile(main,[]byte("http { include nginx.conf; }"),0600);err!=nil{t.Fatal(err)}
    if _,err:=ReadNginxConfigTree(context.Background(),main);err==nil{t.Fatal("recursive include must error")}
}

func TestNginxReadOnlyNeverOpensCertificateOrLog(t *testing.T) {
    dir:=t.TempDir()
    p:=filepath.Join(dir,"nginx.conf")
    config:="http { server { listen 1443 ssl; ssl_certificate /root/missing-cert; error_log /unwritable/nginx.log; location /ws { proxy_pass unix:/dev/shm/test.socket; } } }"
    if err:=os.WriteFile(p,[]byte(config),0600);err!=nil{t.Fatal(err)}
    cat:=Catalog{Inbounds:[]Endpoint{{Tag:"ws",Listen:"unix:/dev/shm/test.socket",Transport:"WebSocket"}}}
    r,err:=DiscoverNginxIngressFromFile(context.Background(),cat,p)
    if err!=nil{t.Fatal(err)}
    if len(r.Ports)!=1 || len(r.Routes)!=1 {t.Fatalf("%+v",r)}
}

func TestNginxReadOnlyIncludeWildcardMissingAllowed(t *testing.T){
   dir:=t.TempDir()
   main:=filepath.Join(dir,"nginx.conf")
   if err:=os.WriteFile(main,[]byte("http { include missing/*.conf; }"),0600);err!=nil{t.Fatal(err)}
   if _,err:=ReadNginxConfigTree(context.Background(),main);err!=nil{t.Fatal(err)}
}
