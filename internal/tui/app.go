package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Accelerator6666/xraytop/internal/collector"
)

type App struct {
	Collector *collector.Collector
	Interval  time.Duration
	NoColor   bool
	ClientFilter string
}

func (a App) Run(ctx context.Context) error {
	if a.Collector == nil {
		return errors.New("nil collector")
	}
	if !isTerminal(os.Stdin.Fd()) || !isTerminal(os.Stdout.Fd()) {
		return errors.New("interactive mode requires a terminal (use --once for scripts)")
	}
	var term terminal
	if err := term.rawMode(); err != nil {
		return err
	}
	defer term.restore()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGWINCH)
	defer signal.Stop(sig)
	keys := make(chan byte, 8)
	go func() {
		var ch [1]byte
		for {
			if _, err := os.Stdin.Read(ch[:]); err != nil {
				return
			}
			select {
			case keys <- ch[0]:
			default:
			}
		}
	}()
	snaps := make(chan collector.Snapshot, 1)
	requests := make(chan struct{}, 1)
	requests <- struct{}{}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-requests:
				queryCtx, done := context.WithTimeout(ctx, 3*time.Second)
				snap := a.Collector.Sample(queryCtx)
				done()
				select {
				case snaps <- snap:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	interval := a.Interval
	if interval < time.Second {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	current := collector.Snapshot{At: time.Now()}
	history := History{}
	view := 0
	clientFilter := a.ClientFilter
	historyWindow:=15
	historyIndex:=0
	historyDetail:=false
	historySort:="recent"
	escapeState:=0
	if clientFilter=="" {clientFilter="all"}
	redraw := func() {
		w, h := terminalSize()
		fmt.Print(Screen{Snapshot: current, History: history, View: view, ClientFilter:clientFilter, HistoryWindow:historyWindow,HistoryIndex:historyIndex,HistoryDetail:historyDetail,HistorySort:historySort, Width: w, Height: h, Color: !a.NoColor}.Paint())
	}
	redraw()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-sig:
			if event == syscall.SIGWINCH {
				redraw()
			} else {
				return nil
			}
		case <-ticker.C:
			select {
			case requests <- struct{}{}:
			default:
			}
		case snap := <-snaps:
			current = snap
			if snap.APIError == "" && snap.Traffic.HasRates {
				history.Add(snap.Traffic.UpRate, snap.Traffic.DownRate)
			}
			redraw()
		case key := <-keys:
			// Translate ANSI Up/Down arrows into j/k without affecting
			// other views or waiting on a blocking terminal read.
			if key==27 {escapeState=1;continue}
			if escapeState==1 {escapeState=0;if key=='[' {escapeState=2;continue}}
			if escapeState==2 {
				escapeState=0
				if key=='A' {key='k'} else if key=='B' {key='j'} else {continue}
			}
			switch key {
			case 'q', 'Q', 3:
				return nil
			case '1':
				view = 0
			case '2':
				view = 1
			case '3':
				view = 2
			case '4':
				view = 3
			case '5':
				view = 4
			case '6':
				view = 5
			case '7':
				view = 6
			case '8':
				view = 7
			case '9':
				view = 8
			case 'j','J':
				if view==8 && historyIndex<len(current.IPHistory)-1 {historyIndex++}
			case 'k','K':
				if view==8 && historyIndex>0 {historyIndex--}
			case 10,13:
				if view==8 && len(current.IPHistory)>0 {historyDetail=!historyDetail}
			case 't','T':
				if view==8 {
					switch historyWindow {case 5:historyWindow=15;case 15:historyWindow=30;case 30:historyWindow=60;default:historyWindow=5}
				}
			case 's','S':
				if view==8 {
					switch historySort {case "recent":historySort="peak";case "peak":historySort="tcp";case "tcp":historySort="xray";default:historySort="recent"}
				}
			case 'f', 'F':
				if view==5 {
					switch clientFilter {
					case "all": clientFilter="xray"
					case "xray": clientFilter="http"
					case "http": clientFilter="tcp"
					default: clientFilter="all"
					}
				}
			case 'r', 'R':
				select {
				case requests <- struct{}{}:
				default:
				}
			case 9:
				view = (view + 1) % 9
			}
			redraw()
		}
	}
}
