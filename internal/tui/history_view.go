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

func (s Screen) historyLines(width, maxLines int) []string {
    lines:=[]string{
        " IP HISTORY  (private local snapshots; 24h retention; 1-minute sampling)",
    }
    add:=func(line string) {lines=append(lines,truncate(line,width))}
    window:=s.HistoryWindow
    if window!=5 && window!=15 && window!=30 && window!=60 {window=15}
    mode:=s.HistorySort
    if mode=="" {mode="recent"}
    add(fmt.Sprintf(" Window: %dm [t] | Sort: %s [s] | Select: [j/k] or arrows | [Enter] detail | [9] history",window,mode))
    if s.Snapshot.HistoryError!="" {
        add(" Storage warning: "+s.Snapshot.HistoryError)
    }
    rows:=s.orderedHistory()
    if len(rows)==0 {
        add(" No stored IP history yet. Leave XrayTop running to collect 1-minute samples.")
        add(" Gaps mean not sampled; zero TCP means observed with no ingress TCP sockets.")
        return lines
    }
    index:=s.HistoryIndex
    if index<0 {index=0}
    if index>=len(rows) {index=len(rows)-1}
    if s.HistoryDetail {
        row:=rows[index]
        add(" SOURCE IP DETAIL: "+row.IP+"    [Enter] return to list")
        add(fmt.Sprintf(" First observed %s | Last evidence %s",
            row.FirstSeen.Local().Format("2006-01-02 15:04:05"),
            row.LastSeen.Local().Format("2006-01-02 15:04:05")))
        add(fmt.Sprintf(" Latest sample: TCP %d | 24h peak TCP %d | 5m-window Xray accepted %d | 5m-window HTTP %d",
            row.CurrentTCP,row.PeakTCP,row.LastXray,row.LastHTTP))
        add(" ")
        plotWidth:=window
        if plotWidth>width-15 {plotWidth=width-15}
        if plotWidth<1 {plotWidth=1}
        add(fmt.Sprintf(" TCP   %s",historyTrend(row.Points,s.Snapshot.At,window,plotWidth,"tcp")))
        add(fmt.Sprintf(" XRAY  %s",historyTrend(row.Points,s.Snapshot.At,window,plotWidth,"xray")))
        add(fmt.Sprintf(" HTTP  %s",historyTrend(row.Points,s.Snapshot.At,window,plotWidth,"http")))
        add(" ")
        add(" Each point = one minute.  · = not sampled   ▁ = sampled zero.")
        add(" XRAY / HTTP show 5-minute rolling event counts per snapshot, NOT totals or traffic bytes.")
        add(" TCP is host ESTABLISHED sockets; none of these prove an online VLESS session.")
        return lines
    }
    add(" Source IP                               First observed    Last evidence     TCP  Peak  Xray HTTP   TCP trend")
    available:=max(1,maxLines-len(lines)-2)
    start:=index-available/2
    if start<0 {start=0}
    if start>len(rows)-available {start=max(0,len(rows)-available)}
    end:=start+available
    if end>len(rows) {end=len(rows)}
    for i:=start;i<end;i++ {
        row:=rows[i]
        marker:=" "
        if i==index {marker=">"}
        trendWidth:=min(window,24)
        if width<145 {trendWidth=min(trendWidth,max(1,width-104))}
        add(fmt.Sprintf("%s%-39s %-15s %-15s %4d %5d %5d %4d   %s",
            marker,truncate(row.IP,39),
            row.FirstSeen.Local().Format("01-02 15:04"),
            row.LastSeen.Local().Format("01-02 15:04"),
            row.CurrentTCP,row.PeakTCP,row.LastXray,row.LastHTTP,
            historyTrend(row.Points,s.Snapshot.At,window,trendWidth,"tcp")))
    }
    add(fmt.Sprintf(" %d stored IPs | showing %d-%d | trend points are sampled observations",len(rows),start+1,end))
    add(" XRAY/HTTP counts are rolling 5-minute windows; missing samples are NOT assumed offline.")
    return lines
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
