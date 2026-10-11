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
   plotWidth:=min(window,max(1,width-12))
   add(" TCP   "+historyTrendPlain(row.Points,s.Snapshot.At,window,plotWidth,"tcp"))
   add(" XRAY  "+historyTrendPlain(row.Points,s.Snapshot.At,window,plotWidth,"xray"))
   add(" HTTP  "+historyTrendPlain(row.Points,s.Snapshot.At,window,plotWidth,"http"))
   add(" ")
   add(" Each point = 1 min. . missing | _ observed zero | : = + # @ intensity.")
   add(" XRAY / HTTP are 5-minute rolling counts, NOT cumulative traffic bytes.")
   add(" TCP sockets and accepted events are NOT authenticated online sessions.")
   return lines
 }
 // Three width tiers: full (>=110), compact one-row (>=66), narrow two-row.
 // The fixed 39-character IP field is only used when enough space remains.
 full:=width>=110
 twoLine:=width<66
 ipWidth:=39
 if !full {ipWidth=min(39,max(16,width-49))}
 if full {
   add(fmt.Sprintf(" %-39s %-11s %-11s %4s %4s %4s %4s  %s",
      "Source IP","First seen","Last seen","TCP","Peak","Xray","HTTP","TCP trend"))
 } else if twoLine {
   add(" Source IP / last evidence / TCP, peak, Xray, HTTP, TCP trend")
 } else {
   add(fmt.Sprintf(" %-*s %-11s %3s %4s %4s %4s  %s",
       ipWidth,"Source IP","Last seen","TCP","Peak","Xray","HTTP","Trend"))
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
       meta+="  "+historyTrendPlain(row.Points,s.Snapshot.At,window,min(remaining,15),"tcp")
     }
     add(meta)
   }else if full {
     base:=fmt.Sprintf("%s%-39s %-11s %-11s %4d %4d %4d %4d  ",
       marker,truncate(row.IP,39),row.FirstSeen.Local().Format("01-02 15:04"),
       last,row.CurrentTCP,row.PeakTCP,row.LastXray,row.LastHTTP)
     trendWidth:=min(window,max(1,width-len([]rune(base))))
     add(base+historyTrendPlain(row.Points,s.Snapshot.At,window,trendWidth,"tcp"))
   }else{
     base:=fmt.Sprintf("%s%-*s %-11s %3d %4d %4d %4d  ",
       marker,ipWidth,truncate(row.IP,ipWidth),last,
       row.CurrentTCP,row.PeakTCP,row.LastXray,row.LastHTTP)
     trendWidth:=min(window,max(1,width-len([]rune(base))))
     add(base+historyTrendPlain(row.Points,s.Snapshot.At,window,trendWidth,"tcp"))
   }
 }
 add(fmt.Sprintf(" %d stored IPs | showing %d-%d | [j/k] scroll",len(rows),start+1,end))
 add(" Trend: . missing, _ zero, : = + # @ relative level; XRAY/HTTP = rolling 5m.")
 return lines
}

// historyTrendPlain uses ASCII-only, fixed-width symbols so incompatible SSH
// fonts cannot render solid-box Unicode bar glyphs. Unlike a simple 0/1
// sparkline, it preserves relative intensity without implying exact counts:
// . = missing, _ = sampled zero, : = low, = / + = middle, # / @ = high.
func historyTrendPlain(points []history.Point,at time.Time,window,columns int,metric string)string {
 trend:=historyTrend(points,at,window,columns,metric)
 return strings.NewReplacer("·",".","▁","_","▂",":","▃",":","▄","=","▅","=","▆","+","▇","#","█","@").Replace(trend)
}

func historyTrend(points []history.Point,at time.Time,window,columns int,metric string) string {
    if window<1 {return ""}
    if columns>window {columns=window}
    if columns<1 {columns=1}
    // For a list cell with fewer columns, display the most recent columns.
    buckets:=make(map[int64]int,len(points))
    for _,p:=range points {
        value:=p.TCP
        switch metric {
        case "xray": value=p.Xray
        case "http": value=p.HTTP
        }
        buckets[p.Minute]=value
    }
    current:=at.Unix()/60
    vals:=make([]int,columns)
    known:=make([]bool,columns)
    peak:=0
    for i:=0;i<columns;i++ {
        // Scale longer windows down to columns by max-in-bucket,
        // while preserving unknown buckets explicitly.
        start:=current-int64(window)+int64(i*window/columns)+1
        stop:=current-int64(window)+int64((i+1)*window/columns)
        for minute:=start;minute<=stop;minute++ {
            if v,ok:=buckets[minute];ok {
                known[i]=true
                if v>vals[i] {vals[i]=v}
            }
        }
        if vals[i]>peak {peak=vals[i]}
    }
    const bars="▁▂▃▄▅▆▇█"
    glyphs:=[]rune(bars)
    var b strings.Builder
    for i,v:=range vals {
        if !known[i] {b.WriteRune('·');continue}
        n:=0
        if peak>0 {n=v*7/peak}
        if n>7 {n=7}
        b.WriteRune(glyphs[n])
    }
    return b.String()
}
