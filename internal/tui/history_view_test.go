package tui

import (
    "strings"
    "testing"
    "time"

    "github.com/Accelerator6666/xraytop/internal/collector"
    "github.com/Accelerator6666/xraytop/internal/history"
)

func TestHistoryTrendMissingVsZero(t *testing.T) {
    now:=time.Unix(6000*60,0)
    points:=[]history.Point{{Minute:5997,TCP:0},{Minute:5998,TCP:4},{Minute:6000,TCP:0}}
    got:=historyTrend(points,now,5,5,"tcp")
    if !strings.HasPrefix(got,"·") || !strings.Contains(got,"█") || !strings.Contains(got,"▁") {
        t.Fatalf("missing gaps and observed zero are not distinct: %s",got)
    }
}
func TestHistoryViewListAndDetails(t *testing.T) {
    now:=time.Unix(6000*60,0)
    snap:=collector.Snapshot{At:now,IPHistory:[]history.Summary{
        {IP:"192.0.2.1",FirstSeen:now.Add(-time.Hour),LastSeen:now,CurrentTCP:3,PeakTCP:5,LastXray:7,LastHTTP:2,
            Points:[]history.Point{{Minute:5999,TCP:5,Xray:7,HTTP:2},{Minute:6000,TCP:3,Xray:7,HTTP:2}}},
        {IP:"198.51.100.2",FirstSeen:now.Add(-2*time.Hour),LastSeen:now.Add(-10*time.Minute),CurrentTCP:0,PeakTCP:17,
            Points:[]history.Point{{Minute:5999,TCP:0}}},
    }}
    s:=Screen{Snapshot:snap,Width:160,Height:35,View:8,HistoryWindow:15}
    plain:=s.Paint()
    if strings.Contains(plain,"100-10") || !strings.Contains(plain,"01-0") {t.Fatalf("history must show valid MM-DD, got: %s",plain)}
    for _,k:=range []string{"IP HISTORY","192.0.2.1","198.51.100.2","TCP trend","[Enter]","Quit"} {
        if !strings.Contains(plain,k) {t.Fatalf("missing %s: %s",k,plain)}
    }
    s.HistoryDetail=true
    detail:=s.Paint()
    for _,k:=range []string{"SOURCE IP DETAIL: 192.0.2.1","TCP   ","XRAY  ","HTTP  ","5-minute rolling"} {
        if !strings.Contains(detail,k) {t.Fatalf("missing %s: %s",k,detail)}
    }
    s.HistoryDetail=false;s.HistorySort="peak"
    if s.orderedHistory()[0].IP!="198.51.100.2" {t.Fatal("peak sort not applied")}
    s.HistorySort="tcp"
    if s.orderedHistory()[0].IP!="192.0.2.1" {t.Fatal("tcp sort not applied")}
}
func TestEmptyHistoryNarrowFooter(t *testing.T) {
    s:=Screen{Width:80,Height:20,View:8,Snapshot:collector.Snapshot{At:time.Now()}}
    out:=s.Paint()
    for _,k:=range []string{"No stored IP history","Quit","9"} {
        if !strings.Contains(out,k) {t.Fatalf("missing %s: %s",k,out)}
    }
}

func TestHistoryResponsiveSSHWidths(t *testing.T) {
 now:=time.Unix(6000*60,0)
 snap:=collector.Snapshot{At:now,IPHistory:[]history.Summary{
   {IP:"192.0.2.1",FirstSeen:now.Add(-time.Hour),LastSeen:now,CurrentTCP:2,PeakTCP:15,LastXray:7,LastHTTP:11,
    Points:[]history.Point{{Minute:6000,TCP:2,Xray:7,HTTP:11}}},
   {IP:"2001:db8:abcd:abcd:abcd:abcd:abcd:abcd",FirstSeen:now.Add(-2*time.Hour),LastSeen:now,
    CurrentTCP:4,PeakTCP:6,LastXray:9,LastHTTP:2},
 }}
 for _,width:=range []int{58,66,78,80,100,110,140,160}{
    screen:=Screen{Snapshot:snap,Width:width,Height:30,View:8,HistoryWindow:15}
    out:=screen.Paint()
    for _,part:=range []string{"192.0.2.1","TCP","Peak","Xray","HTTP","showing 1-2"}{
       if !strings.Contains(out,part){t.Errorf("width %d lost %s: %q",width,part,out)}
    }
    for _,line:=range strings.Split(out,"\n"){
       if n:=len([]rune(line)); n>width {
          t.Errorf("width=%d rendered line=%d: %q",width,n,line)
       }
    }
    if width>=68 && !strings.Contains(out,"  2   15    7   11") {
      t.Errorf("width %d lost count columns: %s",width,out)
    }
    if width==80 && !strings.Contains(out,"Trend") {t.Errorf("80-col trend header lost: %s",out)}
 }
}
func TestHistoryFilteredEmptyAndIPv6Detail(t *testing.T){
 now:=time.Unix(6000*60,0)
 snap:=collector.Snapshot{At:now,IPHistory:[]history.Summary{
  {IP:"2001:db8:abcd:abcd:abcd:abcd:abcd:abcd",FirstSeen:now.Add(-time.Hour),LastSeen:now},
 }}
 s:=Screen{Snapshot:snap,Width:80,Height:24,View:8,IPQuery:"no-match"}
 if !strings.Contains(s.Paint(),"No stored IP history matches"){t.Fatal("missing no-match message")}
 s.IPQuery="2001:db8";s.HistoryDetail=true
 out:=s.Paint()
 if !strings.Contains(out,"SOURCE IP DETAIL: 2001:db8:abcd:abcd:abcd:abcd:abcd:abcd"){t.Fatal(out)}
}
