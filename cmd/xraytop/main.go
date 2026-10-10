package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Accelerator6666/xraytop/internal/collector"
	"github.com/Accelerator6666/xraytop/internal/tui"
	"github.com/Accelerator6666/xraytop/internal/xray"
)

var version = "v0.1.0-dev"

func main() {
	versionFlag := flag.Bool("version", false, "show version")
	api := flag.String("api", "127.0.0.1:10085", "local Xray StatsService address (empty disables API)")
	binary := flag.String("xray-bin", "xray", "path to xray executable used for statsquery")
	interval := flag.Duration("interval", 2*time.Second, "monitor polling interval (minimum 1s)")
	once := flag.Bool("once", false, "print one plain snapshot and exit")
	noColor := flag.Bool("no-color", false, "disable ANSI colors")
	flag.Parse()
	if *versionFlag {
		fmt.Println("xraytop", version)
		return
	}
	if runtime.GOOS != "linux" {
		fmt.Fprintln(os.Stderr, "xraytop only supports Linux")
		os.Exit(1)
	}
	if *interval < time.Second {
		fmt.Fprintln(os.Stderr, "--interval must be at least 1s")
		os.Exit(2)
	}
	c := &collector.Collector{Client: xray.Client{Binary: *binary, Server: *api}}
	if *once {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		snap := c.Sample(ctx)
		fmt.Print(tui.Screen{Snapshot: snap, Width: 100, Height: 20, View: 0, Color: false}.Paint(), "\n")
		return
	}
	app := tui.App{Collector: c, Interval: *interval, NoColor: *noColor || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"}
	if err := app.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "xraytop:", err)
		os.Exit(1)
	}
}
