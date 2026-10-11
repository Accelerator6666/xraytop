package tui

import (
 "os"
 "path/filepath"
 "reflect"
 "testing"
)

func TestANSISizeRepliesAndKeyboardPreservation(t *testing.T){
 var d sizeReplyDecoder
 seen:=false
 for _,b:=range []byte("\x1b[8;45;140t"){
  keys,cols,rows,ok:=d.Feed(b)
  if len(keys)!=0 && ok {t.Fatal("mixed key and reply")}
  if ok {seen=true;if cols!=140||rows!=45{t.Fatalf("%d x %d",cols,rows)}}
 }
 if !seen||d.Pending(){t.Fatal("reply not recognized or left buffered")}
 for _,c:=range []string{"\x1b[A","\x1b[B","\x1b[?25h","q"}{
   var got []byte
   for _,b:=range []byte(c) {keys,_,_,_:=d.Feed(b);got=append(got,keys...)}
   got=append(got,d.Flush()...)
   if string(got)!=c {t.Fatalf("lost keyboard input: %q => %q",c,got)}
 }
 for _,bad:=range []string{"\x1b[8;3;8t","\x1b[8;400;140t","\x1b[8;45;999t"}{
   var ok bool
   for _,b:=range []byte(bad){_,_,_,ok=d.Feed(b);if ok{t.Fatal("invalid reply accepted")}}
   _=d.Flush()
 }
}
func TestTerminalPrefsRoundtripAndSizes(t *testing.T){
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 p:=TerminalPreferences{Columns:140,Rows:45,ASCII:true,DisableQuery:true}
 if err:=SaveTerminalPreferences(p);err!=nil{t.Fatal(err)}
 loaded,err:=LoadTerminalPreferences()
 if err!=nil||!reflect.DeepEqual(p,loaded){t.Fatalf("%+v %v",loaded,err)}
 name,err:=terminalPrefsPath()
 if err!=nil{t.Fatal(err)}
 info,err:=os.Stat(name)
 if err!=nil||info.Mode().Perm()!=0600{t.Fatalf("mode %v err %v",info,err)}
 w,h,source:=resolvedSize(80,24,120,40,p,0,0)
 if w!=140||h!=45||source!="saved"{t.Fatalf("%d %d %s",w,h,source)}
 w,h,source=resolvedSize(80,24,120,40,TerminalPreferences{},0,0)
 if w!=120||h!=40||source!="ANSI"{t.Fatalf("%d %d %s",w,h,source)}
 w,h,source=resolvedSize(80,24,0,0,TerminalPreferences{},0,0)
 if w!=80||h!=24||source!="PTY"{t.Fatalf("%d %d %s",w,h,source)}
 w,h,source=resolvedSize(80,24,0,0,TerminalPreferences{},140,50)
 if w!=140||h!=50||source!="CLI"{t.Fatalf("%d %d %s",w,h,source)}
 // An existing symlink must not be overwritten.
 if err=os.Remove(name);err!=nil{t.Fatal(err)}
 target:=filepath.Join(t.TempDir(),"target")
 if err=os.WriteFile(target,[]byte("SAFE"),0600);err!=nil{t.Fatal(err)}
 if err=os.Symlink(target,name);err!=nil{t.Fatal(err)}
 if err=SaveTerminalPreferences(p);err==nil{t.Fatal("accepted symlink")}
}
func TestPresetChoices(t *testing.T){
 if v:=nextPreset(80,[]int{0,80,100});v!=100{t.Fatal(v)}
 if v:=nextPreset(100,[]int{0,80,100});v!=0{t.Fatal(v)}
}
