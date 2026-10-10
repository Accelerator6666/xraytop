package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/Accelerator6666/xraytop/internal/collector"
	"github.com/Accelerator6666/xraytop/internal/tui"
	"github.com/Accelerator6666/xraytop/internal/xray"
)

var version = "v0.9.0-dev"

// configPaths supports repeated --config arguments for multi-file setups.
type configPaths []string

func (p *configPaths) String() string     { return strings.Join(*p, ", ") }
func (p *configPaths) Set(s string) error { *p = append(*p, s); return nil }

func main() {
	versionFlag := flag.Bool("version", false, "show version")
	var configs configPaths
	flag.Var(&configs, "config", "local Xray JSON/JSONC config file (repeatable; autodetected from process when omitted)")
	configDir := flag.String("config-dir", "", "Xray -confdir path (optional; autodetected when omitted)")
	api := flag.String("api", "127.0.0.1:10085", "local Xray StatsService address (empty disables API)")
	binary := flag.String("xray-bin", "xray", "path to xray executable used for statsquery")
	interval := flag.Duration("interval", 2*time.Second, "monitor polling interval (minimum 1s)")
	once := flag.Bool("once", false, "print one plain snapshot and exit")
	onceView := flag.Int("once-view", 1, "view number used with --once (1-8; 6=source IPs, 7=IP+inbound, 8=NGINX requests)")
	journal := flag.String("journal-unit", "xray", "systemd unit for recent accepted source-IP records (empty disables)")
	portsFlag := flag.String("ingress-ports", "", "manual ingress TCP ports; overrides NGINX discovery (e.g. 443,8964)")
	nginxAuto := flag.Bool("nginx-auto", true, "discover matching Xray ingress ports from local nginx -T (disabled by --ingress-ports or --nginx-auto=false)")
	nginxAccess := flag.String("nginx-access-log", "/var/log/nginx/access.log", "local NGINX combined access log for optional source IP/route observations (empty disables)")
	clientFilter := flag.String("clients-filter", "all", "source IP filter for view 6: all|xray|http|tcp (f cycles in the TUI)")
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
	if *onceView < 1 || *onceView > 8 {
		fmt.Fprintln(os.Stderr, "--once-view must be between 1 and 8")
		os.Exit(2)
	}
	if *interval < time.Second {
		fmt.Fprintln(os.Stderr, "--interval must be at least 1s")
		os.Exit(2)
	}
	switch *clientFilter {
	case "all", "xray", "http", "tcp":
	default:
		fmt.Fprintln(os.Stderr, "--clients-filter must be all, xray, http, or tcp")
		os.Exit(2)
	}
	ports, err := xray.ParseIngressPorts(*portsFlag)
	if err != nil { fmt.Fprintln(os.Stderr, "xraytop:", err); os.Exit(2) }
	c := &collector.Collector{Client: xray.Client{Binary: *binary, Server: *api}, ConfigFiles: configs, ConfigDir: *configDir, JournalUnit: *journal, IngressPorts: ports, NginxAuto: *nginxAuto, NginxAccessPath: *nginxAccess}
	if *once {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		snap := c.Sample(ctx)
		fmt.Print(tui.Screen{Snapshot: snap, Width: 100, Height: 24, View: *onceView - 1, ClientFilter: *clientFilter, Color: false}.Paint(), "\n")
		return
	}
	app := tui.App{Collector: c, Interval: *interval, ClientFilter: *clientFilter, NoColor: *noColor || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"}
	if err := app.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "xraytop:", err)
		os.Exit(1)
	}
}
