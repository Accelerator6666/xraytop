package xray

import (
    "context"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "sort"
    "strings"
)

// The config scanner is deliberately read-only. "nginx -T" is unsuitable for
// sandboxed monitors: its validation stage attempts to open logs and pid files.
const (
    maxNginxConfigFiles = 256
    maxNginxConfigBytes = 4 << 20
    maxNginxIncludeDepth = 24
)

type nginxConfigReader struct {
    ctx context.Context
    base string
    files int
    bytes int64
    stack map[string]bool
}

// ReadNginxConfigTree follows include directives inside their lexical contexts.
// Relative paths resolve against the primary NGINX configuration directory.
// It never evaluates NGINX variables, opens certificates, writes logs, or
// guesses endpoints when an include cannot be read.
func ReadNginxConfigTree(ctx context.Context,primary string)([]*nginxNode,error){
    if primary=="" {return nil,fmt.Errorf("empty NGINX configuration path")}
    abs,err:=filepath.Abs(primary)
    if err!=nil{return nil,err}
    r:=nginxConfigReader{ctx:ctx,base:filepath.Dir(abs),stack:make(map[string]bool)}
    return r.load(abs,0)
}

func (r *nginxConfigReader) load(path string,depth int)([]*nginxNode,error){
    if err:=r.ctx.Err();err!=nil{return nil,err}
    if depth>maxNginxIncludeDepth{return nil,fmt.Errorf("NGINX include nesting limit reached")}
    resolved,err:=filepath.EvalSymlinks(path)
    if err!=nil{return nil,fmt.Errorf("resolve %s: %w",path,err)}
    if r.stack[resolved] {return nil,fmt.Errorf("recursive NGINX include: %s",path)}
    if r.files>=maxNginxConfigFiles {return nil,fmt.Errorf("too many NGINX config files")}
    f,err:=os.Open(resolved)
    if err!=nil{return nil,fmt.Errorf("open %s: %w",path,err)}
    defer f.Close()
    info,err:=f.Stat()
    if err!=nil{return nil,err}
    if !info.Mode().IsRegular(){return nil,fmt.Errorf("not a regular NGINX config: %s",path)}
    if info.Size()>maxNginxConfigBytes-r.bytes {return nil,fmt.Errorf("NGINX config byte limit reached: %s",path)}
    remaining:=int64(maxNginxConfigBytes)-r.bytes
    data,err:=io.ReadAll(io.LimitReader(f,remaining+1))
    if err!=nil{return nil,fmt.Errorf("read %s: %w",path,err)}
    r.bytes+=int64(len(data))
    if r.bytes>maxNginxConfigBytes {return nil,fmt.Errorf("NGINX config byte limit reached")}
    r.files++
    r.stack[resolved]=true
    defer delete(r.stack,resolved)
    return r.expand(nginxTree(nginxLex(data)),depth)
}

func (r *nginxConfigReader) expand(nodes []*nginxNode,depth int)([]*nginxNode,error){
    out:=make([]*nginxNode,0,len(nodes))
    for _,n:=range nodes {
        if err:=r.ctx.Err();err!=nil{return nil,err}
        if n.name=="include" {
            if len(n.args)!=1 {return nil,fmt.Errorf("NGINX include must have one path")}
            pattern:=n.args[0]
            if strings.Contains(pattern,"$") {
                return nil,fmt.Errorf("dynamic NGINX include path unsupported (safe discovery requires literal path)")
            }
            if !filepath.IsAbs(pattern) {pattern=filepath.Join(r.base,pattern)}
            paths,err:=filepath.Glob(pattern)
            if err!=nil{return nil,fmt.Errorf("glob %q: %w",pattern,err)}
            if len(paths)==0 && !strings.ContainsAny(pattern,"*?[") {
                return nil,fmt.Errorf("NGINX include file not found: %s",pattern)
            }
            sort.Strings(paths)
            for _,path:=range paths {
                children,e:=r.load(path,depth+1)
                if e!=nil{return nil,e}
                out=append(out,children...)
            }
            continue
        }
        if len(n.children)>0 {
            children,err:=r.expand(n.children,depth)
            if err!=nil{return nil,err}
            n.children=children
        }
        out=append(out,n)
    }
    return out,nil
}
