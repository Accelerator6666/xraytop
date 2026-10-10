package main

import (
	"context"
	"os/signal"
	"syscall"
	"path/filepath"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/Accelerator6666/xraytop/internal/collector"
	"github.com/Accelerator6666/xraytop/internal/daemon"
	"github.com/Accelerator6666/xraytop/internal/history"
	"github.com/Accelerator6666/xraytop/internal/tui"
	"github.com/Accelerator6666/xraytop/internal/xray"
)

var version = "v0.11.2-dev"

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
	onceView := flag.Int("once-view", 1, "view number used with --once (1-9; 6=source IPs, 7=IP+inbound, 8=NGINX requests, 9=IP history)")
	journal := flag.String("journal-unit", "xray", "systemd unit for recent accepted source-IP records (empty disables)")
	portsFlag := flag.String("ingress-ports", "", "manual ingress TCP ports; overrides NGINX discovery (e.g. 443,8964)")
	nginxAuto := flag.Bool("nginx-auto", true, "read-only NGINX config file discovery (disabled by manual --ingress-ports)")
	nginxConfigFile := flag.String("nginx-config-file", "/etc/nginx/nginx.conf", "read-only NGINX main config path; include files are expanded without nginx -T")
	nginxAccess := flag.String("nginx-access-log", "/var/log/nginx/access.log", "local NGINX combined access log for optional source IP/route observations (empty disables)")
	clientFilter := flag.String("clients-filter", "all", "source IP filter for view 6: all|xray|http|tcp (f cycles in the TUI)")
	historyFile := flag.String("history-file", history.DefaultPath(), "local private 24h IP snapshot JSON file (empty disables history)")
	noColor := flag.Bool("no-color", false, "disable ANSI colors")
	daemonMode := flag.Bool("daemon", false, "run as the headless background snapshot collector")
	dataSource := flag.String("data-source", "auto", "TUI data source: auto|local|daemon (daemon reads snapshots, never collects)")
	snapshotFile := flag.String("snapshot-file", "/var/lib/xraytop/snapshot.json", "local daemon snapshot JSON path (0600)")
	flag.Parse()
	if *versionFlag {
		fmt.Println("xraytop", version)
		return
	}
	if runtime.GOOS != "linux" {
		fmt.Fprintln(os.Stderr, "xraytop only supports Linux")
		os.Exit(1)
	}
	if *onceView < 1 || *onceView > 9 {
		fmt.Fprintln(os.Stderr, "--once-view must be between 1 and 9")
		os.Exit(2)
	}
	if *interval < time.Second {
		fmt.Fprintln(os.Stderr, "--interval must be at least 1s")
		os.Exit(2)
	}
	switch *dataSource {case "auto","local","daemon": default:
		fmt.Fprintln(os.Stderr,"--data-source must be auto, local, or daemon");os.Exit(2)
	}
	if *daemonMode && *dataSource=="daemon" {fmt.Fprintln(os.Stderr,"--daemon cannot use --data-source daemon");os.Exit(2)}
	if *snapshotFile=="" {fmt.Fprintln(os.Stderr,"--snapshot-file cannot be empty");os.Exit(2)}
	switch *clientFilter {
	case "all", "xray", "http", "tcp":
	default:
		fmt.Fprintln(os.Stderr, "--clients-filter must be all, xray, http, or tcp")
		os.Exit(2)
	}
	ports, err := xray.ParseIngressPorts(*portsFlag)
	if err != nil { fmt.Fprintln(os.Stderr, "xraytop:", err); os.Exit(2) }

	// A running systemd daemon is the single history writer; auto mode
	// selects its snapshot when fresh, otherwise preserves old local behavior.
	useDaemon := *dataSource=="daemon"
	if *dataSource=="auto" && !*daemonMode {
		if _,e:=daemon.Read(*snapshotFile); e==nil {useDaemon=true}
	}
	historyPath:=*historyFile
	if *daemonMode || useDaemon {
		// Explicit history-file always wins. Default viewer/daemon history is
		// adjacent to the daemon's snapshot so both use the exact same store.
		explicit:=false
		flag.Visit(func(f *flag.Flag){if f.Name=="history-file" {explicit=true}})
		if !explicit {historyPath=filepath.Join(filepath.Dir(*snapshotFile),"ip-history.json")}
	}
	var store *history.Store
	if !useDaemon && historyPath!="" {
		var e error
		store,e=history.Open(historyPath)
		if e!=nil {fmt.Fprintln(os.Stderr,"xraytop: history:",e);os.Exit(1)}
	}
	var c *collector.Collector
	if !useDaemon {
		c=&collector.Collector{Client: xray.Client{Binary: *binary, Server: *api}, ConfigFiles: configs, ConfigDir: *configDir, JournalUnit: *journal, IngressPorts: ports, NginxAuto: *nginxAuto, NginxConfigFile:*nginxConfigFile, NginxAccessPath: *nginxAccess, HistoryStore:store}
	}
	if *daemonMode {
		ctx,stop:=signal.NotifyContext(context.Background(),syscall.SIGTERM,syscall.SIGINT)
		defer stop()
		if e:=daemon.Run(ctx,c,*snapshotFile,*interval);e!=nil {
			fmt.Fprintln(os.Stderr,"xraytop daemon:",e)
			os.Exit(1)
		}
		return
	}
	var source func(context.Context) collector.Snapshot
	if useDaemon {
		f:=&daemon.Follow{SnapshotPath:*snapshotFile,HistoryPath:historyPath}
		source=f.Sample
	}
	if *once {
		ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second)
		defer cancel()
		var snap collector.Snapshot
		if source!=nil {snap=source(ctx)} else {snap=c.Sample(ctx)}
		fmt.Print(tui.Screen{Snapshot:snap,Width:100,Height:24,View:*onceView-1,ClientFilter:*clientFilter,Color:false}.Paint(),"\n")
		return
	}
	app:=tui.App{Collector:c,Source:source,Interval:*interval,ClientFilter:*clientFilter,NoColor:*noColor||os.Getenv("NO_COLOR")!=""||os.Getenv("TERM")=="dumb"}
	if err:=app.Run(context.Background());err!=nil {
		fmt.Fprintln(os.Stderr,"xraytop:",err)
		os.Exit(1)
	}
}
