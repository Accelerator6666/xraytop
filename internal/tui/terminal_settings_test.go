package tui

import (
 "strings"
 "testing"
 "time"
 "github.com/Accelerator6666/xraytop/internal/collector"
)
func TestTerminalSettingsResponsivePanel(t *testing.T){
 at:=time.Unix(1000,0)
 for _,width:=range []int{40,58,80,100,140,180}{
  s:=Screen{Snapshot:collector.Snapshot{At:at},Width:width,Height:24,View:9,
    TerminalPrefs:TerminalPreferences{Columns:140,Rows:45,ASCII:true},TerminalSource:"saved",
    TerminalPTYWidth:width,TerminalPTYHeight:24}
  out:=s.Paint()
  for _,want:=range []string{"TERMINAL SETTINGS","Width:","Height:","ASCII graphics:"}{
   if !strings.Contains(out,want){t.Errorf("width=%d missing %q from %s",width,want,out)}
  }
  expected:=footerForWidth(width-2,true)
  if !strings.Contains(out,expected) {t.Errorf("settings footer was clipped at %d: want %q, got %q",width,expected,out)}
 }
}
func TestASCIIFallbackGraphics(t *testing.T){
 out:=(Screen{Snapshot:collector.Snapshot{At:time.Unix(0,0)},Width:100,Height:24,View:0,ASCII:true}).Paint()
 for _,glyph:=range []string{"─","█","▁","·"}{
   if strings.Contains(out,glyph){t.Errorf("ASCII fallback still contains %q",glyph)}
 }
 if !strings.Contains(out,"-----"){t.Fatal("ASCII ruler missing")}
}
