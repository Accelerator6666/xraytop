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
   // Use the same light-weight one-character vertical sparkline glyphs as
   // the Home TRAFFIC Up/Down rows. A stable reference scale makes a
   // persistent 2 TCP sockets appear low, not permanently full-height.
   for _,metric:=range []struct{label,kind string;value,peak int}{
     {"TCP","tcp",row.CurrentTCP,row.PeakTCP},
     {"XRAY","xray",row.LastXray,historyMetricPeak(row.Points,"xray",window,s.Snapshot.At)},
     {"HTTP","http",row.LastHTTP,historyMetricPeak(row.Points,"http",window,s.Snapshot.At)},
   }{
     scale:=historyScale(metric.kind,max(metric.value,metric.peak))
     label:=fmt.Sprintf(" %-5s %5d/%-4d  ",metric.label,metric.value,scale)
     cells:=min(window,max(1,width-len([]rune(label))))
     add(label+historySpark(row.Points,s.Snapshot.At,window,cells,metric.kind,scale))
   }
   add(" ")
   add(" Bar height uses a fixed reference; left = older, right = newer.")
   add(" Blank = missing sample; lowest bar = sampled zero.")
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
 tcpScale:=historyScale("tcp",maxTCP)
 // Three width tiers: full (>=110), compact one-row (>=66), narrow two-row.
 // The fixed 39-character IP field is only used when enough space remains.
 full:=width>=110
 twoLine:=width<66
 ipWidth:=39
 if !full {ipWidth=min(39,max(16,width-49))}
 if full {
   add(fmt.Sprintf(" %-39s %-11s %-11s %4s %4s %4s %4s  %s",
      "Source IP","First seen","Last seen","TCP","Peak","Xray","HTTP",fmt.Sprintf("TCP trend (0-%d)",tcpScale)))
 } else if twoLine {
   add(" Source IP / last evidence / TCP, peak, Xray, HTTP, TCP trend")
 } else {
   add(fmt.Sprintf(" %-*s %-11s %3s %4s %4s %4s  %s",
       ipWidth,"Source IP","Last seen","TCP","Peak","Xray","HTTP",fmt.Sprintf("Trend/%d",tcpScale)))
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
     remaining:=width-len([]rune(meta))-2
     if remaining>=4 {
       meta+="  "+historySpark(row.Points,s.Snapshot.At,window,max(1,min(remaining,15)),"tcp",tcpScale)
     }
     add(meta)
   }else if full {
     base:=fmt.Sprintf("%s%-39s %-11s %-11s %4d %4d %4d %4d  ",
       marker,truncate(row.IP,39),row.FirstSeen.Local().Format("01-02 15:04"),
       last,row.CurrentTCP,row.PeakTCP,row.LastXray,row.LastHTTP)
     trendWidth:=min(window,max(1,width-len([]rune(base))))
     if width-len([]rune(base))>=1 {add(base+historySpark(row.Points,s.Snapshot.At,window,trendWidth,"tcp",tcpScale))} else {add(base)}
   }else{
     base:=fmt.Sprintf("%s%-*s %-11s %3d %4d %4d %4d  ",
       marker,ipWidth,truncate(row.IP,ipWidth),last,
       row.CurrentTCP,row.PeakTCP,row.LastXray,row.LastHTTP)
     trendWidth:=min(window,max(1,width-len([]rune(base))))
     if width-len([]rune(base))>=1 {add(base+historySpark(row.Points,s.Snapshot.At,window,trendWidth,"tcp",tcpScale))} else {add(base)}
   }
 }
 add(fmt.Sprintf(" %d stored IPs | showing %d-%d | [j/k] scroll",len(rows),start+1,end))
 add(fmt.Sprintf(" Traffic-style bars: blank missing, lowest bar zero; TCP reference 0-%d sockets.",tcpScale))
 return lines
}

// historyScale sets a minimum absolute reference of 16 TCP sockets, or 32
// accepted/HTTP events per five-minute window. Above that the scale grows in
// powers of two. This makes 2 TCP sockets visibly low (2/16), prevents a
// constant nonzero series from appearing saturated, and lets all IP rows
// share one reference within the history table.
func historyScale(metric string, peak int) int {
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

// historySpark mirrors the Home TRAFFIC spark chart's eight vertical bar
// glyphs. Unlike per-IP max-normalization, it uses an explicit shared/fixed
// scale, so a constant count of 2 sockets never looks fully saturated.
//
// Missing minute: a blank (not a fabricated zero).
// Sampled zero: the lowest ▁ glyph, as in Home TRAFFIC.
// Downsampling selects the highest *observed* value in each time bucket.
func historySpark(points []history.Point,at time.Time,window,columns int,metric string,scale int) string {
 if window<1||columns<1{return ""}
 if columns>window{columns=window}
 if scale<1{scale=1}
 values:=make(map[int64]int,len(points))
 for _,p:=range points {
   value:=p.TCP
   if metric=="xray"{value=p.Xray}else if metric=="http"{value=p.HTTP}
   values[p.Minute]=value
 }
 const bars="▁▂▃▄▅▆▇█"
 glyphs:=[]rune(bars)
 current:=at.Unix()/60
 var b strings.Builder
 for i:=0;i<columns;i++{
   start:=current-int64(window)+int64(i*window/columns)+1
   stop:=current-int64(window)+int64((i+1)*window/columns)
   found:=false
   peak:=0
   for m:=start;m<=stop;m++{
     if v,ok:=values[m];ok {
       found=true
       if v>peak{peak=v}
     }
   }
   if !found{b.WriteByte(' ');continue}
   n:=peak*7/scale
   if peak>0&&n==0{n=1} // positive activity must differ from a sampled zero
   if n<0{n=0}
   if n>7{n=7}
   b.WriteRune(glyphs[n])
 }
 return b.String()
}
