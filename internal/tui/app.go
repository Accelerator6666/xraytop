package tui

import (
 "context"
 "errors"
 "fmt"
 "os"
 "os/signal"
 "strings"
 "syscall"
 "time"

 "github.com/Accelerator6666/xraytop/internal/collector"
)

type App struct {
 Collector *collector.Collector
 Source func(context.Context) collector.Snapshot // strictly read-only daemon viewer
 Interval time.Duration
 NoColor bool
 ClientFilter string
 IPQuery string
 WidthOverride int
 HeightOverride int
 ForceASCII bool
}

func (a App) Run(ctx context.Context) error {
 if a.Collector==nil && a.Source==nil{return errors.New("nil collector and snapshot source")}
 if !isTerminal(os.Stdin.Fd())||!isTerminal(os.Stdout.Fd()){
   return errors.New("interactive mode requires a terminal (use --once for scripts)")
 }
 var term terminal
 if err:=term.rawMode();err!=nil{return err}
 defer term.restore()

 prefs,prefsErr:=LoadTerminalPreferences()
 if prefsErr!=nil {prefs=TerminalPreferences{}}
 draft:=prefs
 settingsOpen:=false
 settingsMsg:=""
 ctx,cancel:=context.WithCancel(ctx)
 defer cancel()
 signals:=make(chan os.Signal,1)
 signal.Notify(signals,syscall.SIGTERM,syscall.SIGINT,syscall.SIGWINCH)
 defer signal.Stop(signals)

 input:=make(chan byte,128)
 go func(){
   var b [1]byte
   for {
     if _,err:=os.Stdin.Read(b[:]);err!=nil{return}
     select{case input<-b[0]:default:}
   }
 }()

 snaps:=make(chan collector.Snapshot,1)
 requests:=make(chan struct{},1)
 requests<-struct{}{}
 go func(){
   for {
     select {
     case <-ctx.Done():return
     case <-requests:
       sampleCtx,done:=context.WithTimeout(ctx,3*time.Second)
       var snap collector.Snapshot
       if a.Source!=nil {snap=a.Source(sampleCtx)} else {snap=a.Collector.Sample(sampleCtx)}
       done()
       select{case snaps<-snap:case <-ctx.Done():return}
     }
   }
 }()
 interval:=a.Interval
 if interval<time.Second{interval=2*time.Second}
 ticker:=time.NewTicker(interval)
 defer ticker.Stop()

 current:=collector.Snapshot{At:time.Now()}
 history:=History{}
 view:=0
 filter:=a.ClientFilter
 if filter==""{filter="all"}
 query:=a.IPQuery
 searchBackup:=""
 searchEditing:=false
 historyWindow:=15
 historyIndex:=0
 historyDetail:=false
 historySort:="recent"
 escapeState:=0

 ansiW,ansiH:=0,0
 var lastProbe time.Time
 probe:=func(){
    if prefs.DisableQuery||strings.EqualFold(os.Getenv("TERM"),"dumb"){return}
    if time.Since(lastProbe)<200*time.Millisecond{return}
    lastProbe=time.Now()
    // xterm-compatible CSI 18t: request text area in rows and columns.
    // Unsupported terminals ignore it. No waiting on the calling goroutine.
    _,_=fmt.Fprint(os.Stdout,"\x1b[18t")
 }
 redraw:=func(){
    w,h:=terminalSize()
    active:=prefs
    if settingsOpen{active=draft}
    w,h,source:=resolvedSize(w,h,ansiW,ansiH,active,a.WidthOverride,a.HeightOverride)
    ascii:=active.ASCII||a.ForceASCII
    show:=view
    if settingsOpen{show=9}
    fmt.Print(Screen{
      Snapshot:current,History:history,View:show,ClientFilter:filter,
      IPQuery:query,SearchEditing:searchEditing,HistoryWindow:historyWindow,
      HistoryIndex:historyIndex,HistoryDetail:historyDetail,
      HistorySort:historySort,Width:w,Height:h,Color:!a.NoColor,
      ASCII:ascii,TerminalPrefs:draft,TerminalSource:source,
      TerminalPTYWidth:w,TerminalPTYHeight:h,TerminalANSIWidth:ansiW,
      TerminalANSIHeight:ansiH,SettingsMessage:settingsMsg,
    }.Paint())
 }
 handleKey:=func(key byte) bool {
   if settingsOpen{
     switch key {
     case 27,'z','Z':settingsOpen=false;settingsMsg=""
     case 'w','W':draft.Columns=nextPreset(draft.Columns,[]int{0,80,100,120,140,160,180})
     case 'h','H':draft.Rows=nextPreset(draft.Rows,[]int{0,24,30,40,45,50,60})
     case 'a','A':draft.ASCII=!draft.ASCII
     case 'p','P':draft.DisableQuery=!draft.DisableQuery
     case 'r','R':draft=TerminalPreferences{};settingsMsg="Defaults selected; press [s] to save"
     case 's','S':
       if err:=SaveTerminalPreferences(draft);err!=nil{
         settingsMsg="Save failed: "+err.Error()
       }else{
         prefs=draft;settingsOpen=false;settingsMsg=""
         if !prefs.DisableQuery{probe()}
       }
     }
     redraw()
     return false
   }
   if searchEditing{
     switch key {
     case 27:query=searchBackup;searchEditing=false
     case 10,13:searchEditing=false;historyIndex=0;historyDetail=false
     case 127,8:if len(query)>0 {query=query[:len(query)-1];historyIndex=0;historyDetail=false}
     case 21:query="";historyIndex=0;historyDetail=false
     default:if key>=32 && key<=126 && len(query)<64 {query+=string(key);historyIndex=0;historyDetail=false}
     }
     redraw()
     return false
   }
   // Existing arrow handling, after the size-reply decoder has already
   // stripped CSI 8;rows;cols t from the shared input stream.
   if key==27{escapeState=1;return false}
   if escapeState==1{
     escapeState=0
     if key=='['{escapeState=2;return false}
   }else if escapeState==2{
     escapeState=0
     if key=='A'{key='k'}else if key=='B'{key='j'}else{return false}
   }
   switch key {
   case 'q','Q',3:return true
   case 'z','Z':
     draft=prefs;settingsOpen=true;settingsMsg=""
     if prefsErr!=nil {settingsMsg="Existing preferences unreadable; saving replaces them"}
   case '/':if view==5||view==8{searchBackup=query;searchEditing=true}
   case '1':view=0
   case '2':view=1
   case '3':view=2
   case '4':view=3
   case '5':view=4
   case '6':view=5
   case '7':view=6
   case '8':view=7
   case '9':view=8
   case 'j','J':
      ordered:=(Screen{Snapshot:current,IPQuery:query}).orderedHistory()
      if view==8 && historyIndex<len(ordered)-1{historyIndex++}
   case 'k','K':if view==8 && historyIndex>0{historyIndex--}
   case 10,13:
      if view==8&&len((Screen{Snapshot:current,IPQuery:query}).orderedHistory())>0{historyDetail=!historyDetail}
   case 't','T':
      if view==8 {switch historyWindow {case 5:historyWindow=15;case 15:historyWindow=30;case 30:historyWindow=60;default:historyWindow=5}}
   case 's','S':
      if view==8 {switch historySort {case "recent":historySort="peak";case "peak":historySort="tcp";case "tcp":historySort="xray";default:historySort="recent"}}
   case 'f','F':
      if view==5{switch filter {case "all":filter="xray";case "xray":filter="http";case "http":filter="tcp";default:filter="all"}}
   case 'r','R':
      select{case requests<-struct{}{}:default:}
   case 9:view=(view+1)%9
   }
   redraw()
   return false
 }

 var decoder sizeReplyDecoder
 var pendingTimer <-chan time.Time
 probe()
 redraw()
 for {
   select {
   case <-ctx.Done():return nil
   case sig:=<-signals:
      if sig!=syscall.SIGWINCH{return nil}
      // On resize clear the old reply; a fresh query is needed for a new
      // terminal geometry. PTY remains the safe fallback.
      ansiW,ansiH=0,0
      probe()
      redraw()
   case <-ticker.C:
      select{case requests<-struct{}{}:default:}
   case snap:=<-snaps:
      current=snap
      if snap.APIError=="" && snap.Traffic.HasRates {
        history.Add(snap.Traffic.UpRate,snap.Traffic.DownRate)
      }
      redraw()
   case b:=<-input:
      bytes,w,h,matched:=decoder.Feed(b)
      if matched {
        if !prefs.DisableQuery{ansiW,ansiH=w,h;redraw()}
      }
      if decoder.Pending(){pendingTimer=time.After(90*time.Millisecond)}else{pendingTimer=nil}
      for _,key:=range bytes {if handleKey(key){return nil}}
   case <-pendingTimer:
      pendingTimer=nil
      for _,key:=range decoder.Flush(){if handleKey(key){return nil}}
   }
 }
}
