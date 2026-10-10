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
	redraw := func() {
		w, h := terminalSize()
		fmt.Print(Screen{Snapshot: current, History: history, View: view, Width: w, Height: h, Color: !a.NoColor}.Paint())
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
			case 'r', 'R':
				select {
				case requests <- struct{}{}:
				default:
				}
			case 9:
				view = (view + 1) % 5
			}
			redraw()
		}
	}
}
