package tui

import (
    "fmt"
    "sort"
    "strings"
    "time"

    "github.com/Accelerator6666/xraytop/internal/history"
)

func (s Screen) orderedHistory() []history.Summary {
    rows:=append([]history.Summary(nil),s.Snapshot.IPHistory...)
	if q:=strings.ToLower(strings.TrimSpace(s.IPQuery));q!="" {
		filtered:=rows[:0]
		for _,row:=range rows{if strings.Contains(strings.ToLower(row.IP),q){filtered=append(filtered,row)}}
		rows=filtered
	}
    sortMode:=s.HistorySort
    if sortMode=="" {sortMode="recent"}
    sort.SliceStable(rows,func(i,j int)bool {
        a,b:=rows[i],rows[j]
        switch sortMode {
        case "peak":
            if a.PeakTCP!=b.PeakTCP {return a.PeakTCP>b.PeakTCP}
        case "tcp":
            if a.CurrentTCP!=b.CurrentTCP {return a.CurrentTCP>b.CurrentTCP}
        case "xray":
            if a.LastXray!=b.LastXray {return a.LastXray>b.LastXray}
        default:
            if !a.LastSeen.Equal(b.LastSeen) {return a.LastSeen.After(b.LastSeen)}
        }
        return a.IP<b.IP
    })
    return rows
}

// historyLines uses responsive columns. A wide layout keeps the full IP,
// first/last evidence, counts and trend. An 80-column SSH terminal retains
// *all four counts* instead of cutting them off after the timestamp.
func (s Screen) historyLines(width, maxLines int) []string {
 lines:=[]string{" IP HISTORY  (private local snapshots; 24h retention; 1-minute sampling)"}
 add:=func(line string){lines=append(lines,truncate(line,width))}
 window:=s.HistoryWindow
 if window!=5 && window!=15 && window!=30 && window!=60 {window=15}
 mode:=s.HistorySort
 if mode==""{mode="recent"}
 if width<80 {
   add(fmt.Sprintf(" %dm [t] | sort %s [s] | IP search [/] | select [j/k]",window,mode))
 }else{
   add(fmt.Sprintf(" Window %dm [t] | Sort %s [s] | IP %q [/] | [j/k] | [Enter] detail",window,mode,s.IPQuery))
 }
 if s.Snapshot.HistoryError!=""{add(" Storage warning: "+s.Snapshot.HistoryError)}
 rows:=s.orderedHistory()
 if len(rows)==0 {
   if s.IPQuery!="" {
     add(fmt.Sprintf(" No stored IP history matches %q. Press / to edit search.",s.IPQuery))
   }else{
     add(" No stored IP history yet. Leave the collector running to record samples.")
   }
   add(" Gaps mean not sampled; zero TCP means no observed ingress sockets.")
   return lines
 }
 index:=s.HistoryIndex
 if index<0 {index=0}
 if index>=len(rows) {index=len(rows)-1}
 if s.HistoryDetail {
   row:=rows[index]
   add(" SOURCE IP DETAIL: "+row.IP+"    [Enter] return to list")
   add(" First observed: "+row.FirstSeen.Local().Format("2006-01-02 15:04:05"))
   add(" Last evidence:  "+row.LastSeen.Local().Format("2006-01-02 15:04:05"))
   add(fmt.Sprintf(" TCP %d | 24h peak %d | Xray(5m) %d | HTTP(5m) %d",
     row.CurrentTCP,row.PeakTCP,row.LastXray,row.LastHTTP))
   add(" ")
   // The segmented gauge is absolute on an explicit scale; it never
   // normalizes a steady 2 sockets into a misleading 100% load.
   gaugeWidth:=min(24,max(8,(width-24)/3))
   for _,metric:=range []struct{label,kind string;value,peak int}{
     {"TCP","tcp",row.CurrentTCP,row.PeakTCP},
     {"XRAY","xray",row.LastXray,historyMetricPeak(row.Points,"xray",window,s.Snapshot.At)},
     {"HTTP","http",row.LastHTTP,historyMetricPeak(row.Points,"http",window,s.Snapshot.At)},
   }{
     scale:=btopScale(metric.kind,max(metric.value,metric.peak))
     gauge:=btopGauge(metric.value,scale,gaugeWidth)
     prefix:=fmt.Sprintf(" %-4s %s %d/%d  ",metric.label,gauge,metric.value,scale)
     trendSpace:=width-len(prefix)-2
     if trendSpace>4 {
        samples:=min(window,trendSpace-2)
        prefix+=" "+btopTimeline(row.Points,s.Snapshot.At,window,samples,metric.kind,scale)
     }
     add(prefix)
   }
   add(" ")
   add(" btop-style meters: # current on fixed scale; timeline left=old right=new.")
   add(" Legend: space=missing, .=observed zero, : = + # @=increasing value.")
   add(" XRAY / HTTP are rolling five-minute observations, NOT cumulative.")
   add(" TCP sockets and accepted events are NOT authenticated sessions.")
   return lines
 }
 // The scale is shared across rows, rather than recalculated per IP.
 // A constant count of two sockets becomes 2/16, not a full-height bar.
 maxTCP:=0
 for _,row:=range rows {
    maxTCP=max(maxTCP,row.PeakTCP)
 }
 tcpScale:=btopScale("tcp",maxTCP)
 // Three width tiers: full (>=110), compact one-row (>=66), narrow two-row.
 // The fixed 39-character IP field is only used when enough space remains.
 full:=width>=110
 twoLine:=width<66
 ipWidth:=39
 if !full {ipWidth=min(39,max(16,width-49))}
 if full {
   add(fmt.Sprintf(" %-39s %-11s %-11s %4s %4s %4s %4s  %s",
      "Source IP","First seen","Last seen","TCP","Peak","Xray","HTTP",fmt.Sprintf("TCP 0-%d",tcpScale)))
 } else if twoLine {
   add(" Source IP / last evidence / TCP, peak, Xray, HTTP, TCP trend")
 } else {
   add(fmt.Sprintf(" %-*s %-11s %3s %4s %4s %4s  %s",
       ipWidth,"Source IP","Last seen","TCP","Peak","Xray","HTTP",fmt.Sprintf("TCP/%d",tcpScale)))
 }
 rowsPerItem:=1
 if twoLine {rowsPerItem=2}
 availableLines:=max(1,maxLines-len(lines)-2)
 pageSize:=max(1,availableLines/rowsPerItem)
 start:=index-pageSize/2
 if start<0{start=0}
 if start>len(rows)-pageSize {start=max(0,len(rows)-pageSize)}
 end:=min(len(rows),start+pageSize)
 for i:=start;i<end;i++{
   row:=rows[i]
   marker:=" "
   if i==index{marker=">"}
   last:=row.LastSeen.Local().Format("01-02 15:04")
   if twoLine {
     add(marker+row.IP)
     meta:=fmt.Sprintf(" Last %s | TCP %d Peak %d Xray %d HTTP %d",
       last,row.CurrentTCP,row.PeakTCP,row.LastXray,row.LastHTTP)
     remaining:=width-len([]rune(meta))-3
     if remaining>=4 {
       meta+="  "+btopTimeline(row.Points,s.Snapshot.At,window,max(1,min(remaining-2,15)),"tcp",tcpScale)
     }
     add(meta)
   }else if full {
     base:=fmt.Sprintf("%s%-39s %-11s %-11s %4d %4d %4d %4d  ",
       marker,truncate(row.IP,39),row.FirstSeen.Local().Format("01-02 15:04"),
       last,row.CurrentTCP,row.PeakTCP,row.LastXray,row.LastHTTP)
     trendWidth:=min(window,max(1,width-len([]rune(base))-2))
     if width-len([]rune(base))>=3 {add(base+btopTimeline(row.Points,s.Snapshot.At,window,trendWidth,"tcp",tcpScale))} else {add(base)}
   }else{
     base:=fmt.Sprintf("%s%-*s %-11s %3d %4d %4d %4d  ",
       marker,ipWidth,truncate(row.IP,ipWidth),last,
       row.CurrentTCP,row.PeakTCP,row.LastXray,row.LastHTTP)
     trendWidth:=min(window,max(1,width-len([]rune(base))-2))
     if width-len([]rune(base))>=3 {add(base+btopTimeline(row.Points,s.Snapshot.At,window,trendWidth,"tcp",tcpScale))} else {add(base)}
   }
 }
 add(fmt.Sprintf(" %d stored IPs | showing %d-%d | [j/k] scroll",len(rows),start+1,end))
 add(fmt.Sprintf(" Timeline: space missing, . sampled zero; TCP scale 0-%d sockets (not users).",tcpScale))
 return lines
}

// btopScale sets a minimum absolute reference of 16 TCP sockets, or 32
// accepted/HTTP events per five-minute window. Above that the scale grows in
// powers of two. This makes 2 TCP sockets visibly low (2/16), prevents a
// constant nonzero series from appearing saturated, and lets all IP rows
// share one reference within the history table.
func btopScale(metric string, peak int) int {
 scale:=16
 if metric!="tcp"{scale=32}
 for scale<peak && scale<1<<29 {scale*=2}
 return scale
}

func historyMetricPeak(points []history.Point,metric string,window int,now time.Time)int{
 minMinute:=now.Unix()/60-int64(window)+1
 peak:=0
 for _,p:=range points{
   if p.Minute<minMinute || p.Minute>now.Unix()/60 {continue}
   value:=p.TCP
   if metric=="xray"{value=p.Xray}else if metric=="http"{value=p.HTTP}
   if value>peak{peak=value}
 }
 return peak
}

// btopGauge renders an explicit-value segmented meter, padded to fixed width.
// The rendering layer can colorize filled cells with ANSI background blocks,
// without relying on unsupported Unicode shading glyphs.
func btopGauge(value,scale,segments int)string{
 if segments<1{return ""}
 if scale<1{scale=1}
 if value<0{value=0}
 if value>scale{value=scale}
 // Round to nearest filled cell; >0 should remain visible on a small meter.
 filled:=(value*segments+scale/2)/scale
 if value>0&&filled==0{filled=1}
 if filled>segments{filled=segments}
 return "["+strings.Repeat("|",filled)+strings.Repeat("-",segments-filled)+"]"
}

// btopTimeline shows chronological samples against a stable absolute scale.
// SPACE means unknown/missing data; '.' means a measured zero. Five levels of
// increasing intensity are deliberately printable ASCII for SSH font safety.
func btopTimeline(points []history.Point,at time.Time,window,columns int,metric string,scale int)string {
 if window<1||columns<1{return ""}
 if columns>window{columns=window}
 if scale<1{scale=1}
 buckets:=make(map[int64]int,len(points))
 for _,p:=range points{
    value:=p.TCP
    if metric=="xray"{value=p.Xray}else if metric=="http"{value=p.HTTP}
    buckets[p.Minute]=value
 }
 current:=at.Unix()/60
 var b strings.Builder
 b.WriteByte('[')
 for i:=0;i<columns;i++ {
   start:=current-int64(window)+int64(i*window/columns)+1
   stop:=current-int64(window)+int64((i+1)*window/columns)
   known:=false
   peak:=0
   for minute:=start;minute<=stop;minute++{
      if v,ok:=buckets[minute];ok{
        known=true
        if v>peak{peak=v}
      }
   }
   switch {
   case !known:b.WriteByte(' ')
   case peak<=0:b.WriteByte('.')
   default:
      switch {
      case peak*8<=scale:b.WriteByte(':')
      case peak*4<=scale:b.WriteByte('=')
      case peak*2<=scale:b.WriteByte('+')
      case peak*4<=scale*3:b.WriteByte('#')
      default:b.WriteByte('@')
      }
   }
 }
 b.WriteByte(']')
 return b.String()
}
