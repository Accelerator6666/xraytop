package tui

import (
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
)

type TerminalPreferences struct {
 Columns int `json:"columns,omitempty"`
 Rows int `json:"rows,omitempty"`
 ASCII bool `json:"ascii,omitempty"`
 DisableQuery bool `json:"disable_query,omitempty"`
}
func (p TerminalPreferences) Validate()error{
 if (p.Columns!=0 && (p.Columns<40||p.Columns>180)) ||
    (p.Rows!=0 && (p.Rows<13||p.Rows>200)) {
   return errors.New("terminal geometry must be 0 (auto), 40..180 columns and 13..200 rows")
 }
 return nil
}
func terminalPrefsPath()(string,error){
 dir,err:=os.UserConfigDir()
 if err!=nil{return "",err}
 if !filepath.IsAbs(dir){return "",errors.New("config directory must be absolute")}
 return filepath.Join(dir,"xraytop","terminal.json"),nil
}
func LoadTerminalPreferences()(TerminalPreferences,error){
 path,err:=terminalPrefsPath()
 if err!=nil{return TerminalPreferences{},err}
 f,err:=os.OpenFile(path,os.O_RDONLY,0)
 if errors.Is(err,os.ErrNotExist){return TerminalPreferences{},nil}
 if err!=nil{return TerminalPreferences{},err}
 defer f.Close()
 info,err:=f.Stat()
 if err!=nil{return TerminalPreferences{},err}
 if !info.Mode().IsRegular()||info.Size()>4096 {
   return TerminalPreferences{},errors.New("terminal config must be a regular file <=4 KiB")
 }
 var p TerminalPreferences
 dec:=json.NewDecoder(io.LimitReader(f,4097))
 dec.DisallowUnknownFields()
 if err=dec.Decode(&p);err!=nil{return TerminalPreferences{},err}
 if err=p.Validate();err!=nil{return TerminalPreferences{},err}
 var rest any
 if err=dec.Decode(&rest);err!=io.EOF{return TerminalPreferences{},errors.New("unexpected trailing terminal configuration data")}
 return p,nil
}
func SaveTerminalPreferences(p TerminalPreferences)error{
 if err:=p.Validate();err!=nil{return err}
 path,err:=terminalPrefsPath()
 if err!=nil{return err}
 dir:=filepath.Dir(path)
 if fi,err:=os.Lstat(dir);err==nil {
   if !fi.IsDir()||fi.Mode()&os.ModeSymlink!=0 {return errors.New("terminal settings directory must be a real directory")}
 }else if errors.Is(err,os.ErrNotExist) {
   if err=os.MkdirAll(dir,0700);err!=nil{return err}
 }else{return err}
 if fi,err:=os.Lstat(path);err==nil {
    if !fi.Mode().IsRegular()||fi.Mode()&os.ModeSymlink!=0 {
      return errors.New("terminal settings target is not a regular file")
    }
 }else if !errors.Is(err,os.ErrNotExist){return err}
 contents,err:=json.MarshalIndent(p,"","  ")
 if err!=nil{return err}
 tmp,err:=os.CreateTemp(dir,".terminal-*")
 if err!=nil{return err}
 defer os.Remove(tmp.Name())
 defer tmp.Close()
 if err=tmp.Chmod(0600);err!=nil{return err}
 if _,err=tmp.Write(append(contents,'\n'));err!=nil{return err}
 if err=tmp.Sync();err!=nil{return err}
 if err=tmp.Close();err!=nil{return err}
 if err=os.Rename(tmp.Name(),path);err!=nil{return fmt.Errorf("save terminal settings: %w",err)}
 return nil
}
func resolvedSize(ptyW,ptyH,ansiW,ansiH int,p TerminalPreferences,cliW,cliH int)(w,h int,source string){
 source="PTY"
 w,h=ptyW,ptyH
 if !p.DisableQuery && ansiW>=40 && ansiH>=13 {w,h,source=ansiW,ansiH,"ANSI"}
 if p.Columns>0 {w=p.Columns;source="saved"}
 if p.Rows>0 {h=p.Rows;source="saved"}
 if cliW>0 {w=cliW;source="CLI"}
 if cliH>0 {h=cliH;source="CLI"}
 if w<40 {w=40};if w>180{w=180}
 if h<13 {h=13};if h>200{h=200}
 return w,h,source
}
func nextPreset(value int,choices []int)int{
 for i,option:=range choices{if value==option{return choices[(i+1)%len(choices)]}}
 return choices[0]
}
