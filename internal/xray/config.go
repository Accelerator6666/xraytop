package xray

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Endpoint describes a protocol and its transport, not a live connection.
// It deliberately contains no credentials, UUIDs, destinations or client settings.
type Endpoint struct {
	Tag, Protocol, Transport, Security, Listen string
}

// Catalog describes the local, on-disk Xray configuration. It is best-effort:
// a running instance may have been reconfigured since the file was loaded.
type Catalog struct {
	Inbounds, Outbounds []Endpoint
	Source              string
}

type configEntry struct {
	Tag            string          `json:"tag"`
	Protocol       string          `json:"protocol"`
	Listen         json.RawMessage `json:"listen"`
	Port           json.RawMessage `json:"port"`
	StreamSettings struct {
		Method   string `json:"method"`
		Network  string `json:"network"`
		Security string `json:"security"`
	} `json:"streamSettings"`
}

type configEnvelope struct {
	Inbounds  []configEntry `json:"inbounds"`
	Outbounds []configEntry `json:"outbounds"`
}

// LoadCatalog merges descriptors from specified files and/or a confdir.
// Later files replace matching tagged endpoints, matching Xray's confdir rules.
// It intentionally does not attempt to validate the full Xray configuration.
func LoadCatalog(files []string, confdir string) (Catalog, error) {
	paths := append([]string(nil), files...)
	if confdir != "" {
		entries, err := os.ReadDir(confdir)
		if err != nil {
			return Catalog{}, fmt.Errorf("confdir: %w", err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			// Xray's confdir only loads *.json files; also accept JSONC for an
			// explicitly named file via -config, but do not load unrelated files.
			if strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
				paths = append(paths, filepath.Join(confdir, entry.Name()))
			}
		}
	}
	if len(paths) == 0 {
		return Catalog{}, nil
	}
	cat := Catalog{Source: strings.Join(paths, ", ")}
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return cat, fmt.Errorf("read config %q: %w", path, err)
		}
		data, err := io.ReadAll(io.LimitReader(f, 16<<20+1))
		_ = f.Close()
		if err != nil {
			return cat, fmt.Errorf("read config %q: %w", path, err)
		}
		if len(data) > 16<<20 {
			return cat, fmt.Errorf("config %q exceeds 16 MiB", path)
		}
		var doc configEnvelope
		if err := json.Unmarshal(stripJSONComments(data), &doc); err != nil {
			return cat, fmt.Errorf("parse config %q (JSON/JSONC required): %w", path, err)
		}
		for _, entry := range doc.Inbounds {
			cat.Inbounds = upsert(cat.Inbounds, descriptor(entry, true))
		}
		for _, entry := range doc.Outbounds {
			cat.Outbounds = upsert(cat.Outbounds, descriptor(entry, false))
		}
	}
	return cat, nil
}

func upsert(rows []Endpoint, endpoint Endpoint) []Endpoint {
	if endpoint.Tag != "" {
		for i := range rows {
			if rows[i].Tag == endpoint.Tag {
				rows[i] = endpoint
				return rows
			}
		}
	}
	return append(rows, endpoint)
}

func descriptor(e configEntry, inbound bool) Endpoint {
	tr := strings.TrimSpace(e.StreamSettings.Method)
	if tr == "" {
		tr = e.StreamSettings.Network
	}
	// The absence of streamSettings on direct/block outbounds does not prove
	// that a proxy transport exists, so report none rather than RAW.
	if !inbound && tr == "" {
		tr = "-"
	}
	endpoint := Endpoint{
		Tag:       e.Tag,
		Protocol:  normalizeProtocol(e.Protocol),
		Transport: normalizeTransport(tr),
		Security:  normalizeSecurity(e.StreamSettings.Security),
	}
	if inbound {
		endpoint.Listen = listenLocation(e.Listen, e.Port)
	}
	return endpoint
}

func normalizeProtocol(s string) string {
	if strings.TrimSpace(s) == "" {
		return "Unknown"
	}
	switch strings.ToLower(s) {
	case "vless":
		return "VLESS"
	case "vmess":
		return "VMess"
	case "trojan":
		return "Trojan"
	case "socks":
		return "SOCKS"
	case "http":
		return "HTTP"
	case "shadowsocks":
		return "Shadowsocks"
	case "hysteria":
		return "Hysteria"
	case "freedom":
		return "Freedom"
	case "blackhole":
		return "Blackhole"
	case "dokodemo-door":
		return "Dokodemo-door"
	}
	return s
}

func normalizeTransport(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "raw", "tcp":
		return "RAW"
	case "-":
		return "-"
	case "grpc":
		return "gRPC"
	case "ws", "websocket":
		return "WebSocket"
	case "xhttp", "splithttp":
		return "XHTTP"
	case "httpupgrade":
		return "HTTPUpgrade"
	case "kcp", "mkcp":
		return "mKCP"
	case "hysteria":
		return "Hysteria"
	case "h2", "http":
		return "HTTP/2 (legacy)"
	case "h3":
		return "HTTP/3 (legacy)"
	case "quic":
		return "QUIC (legacy)"
	}
	return strings.TrimSpace(s)
}

func normalizeSecurity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none":
		return "-"
	case "tls":
		return "TLS"
	case "reality":
		return "REALITY"
	}
	return s
}

func listenLocation(listen, port json.RawMessage) string {
	var address string
	_ = json.Unmarshal(listen, &address)
	var p string
	if len(port) != 0 && string(port) != "null" {
		if err := json.Unmarshal(port, &p); err != nil {
			p = strings.Trim(string(port), "\" ")
		}
	}
	if strings.Contains(address, "/") || strings.HasPrefix(address, "@") {
		return "unix:" + strings.SplitN(address, ",", 2)[0]
	}
	if p == "" || p == "0" {
		if address != "" {
			return address
		}
		return "-"
	}
	if address == "" {
		return ":" + p
	}
	if strings.Contains(address, ":") {
		return "[" + address + "]:" + p
	}
	return address + ":" + p
}

// stripJSONComments preserves quoted strings (including URLs), escaped quotes,
// and newlines. Comments are replaced with whitespace to retain diagnostics.
func stripJSONComments(src []byte) []byte {
	dst := bytes.Clone(src)
	inString, escaped, lineComment, blockComment := false, false, false, false
	for i := 0; i < len(dst); i++ {
		b := src[i]
		switch {
		case lineComment:
			if b == '\n' || b == '\r' {
				lineComment = false
			} else {
				dst[i] = ' '
			}
		case blockComment:
			if b == '*' && i+1 < len(dst) && src[i+1] == '/' {
				dst[i], dst[i+1] = ' ', ' '
				i++
				blockComment = false
			} else if b != '\n' && b != '\r' {
				dst[i] = ' '
			}
		case inString:
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				inString = false
			}
		case b == '"':
			inString = true
		case b == '/' && i+1 < len(dst) && src[i+1] == '/':
			lineComment = true
			dst[i], dst[i+1] = ' ', ' '
			i++
		case b == '/' && i+1 < len(dst) && src[i+1] == '*':
			blockComment = true
			dst[i], dst[i+1] = ' ', ' '
			i++
		}
	}
	return dst
}

// DiscoverConfigPaths inspects the process command line; it does not probe
// remote hosts or execute Xray. Relative paths are resolved against /proc/PID/cwd.
// -c/-config may repeat, and -confdir is optional.
func DiscoverConfigPaths(procRoot string, pid int) ([]string, string, error) {
	if pid <= 0 {
		return nil, "", nil
	}
	if procRoot == "" {
		procRoot = "/proc"
	}
	base := filepath.Join(procRoot, strconv.Itoa(pid))
	data, err := os.ReadFile(filepath.Join(base, "cmdline"))
	if err != nil {
		return nil, "", err
	}
	fields := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	cwd, _ := os.Readlink(filepath.Join(base, "cwd"))
	resolve := func(s string) string {
		if s == "" || filepath.IsAbs(s) {
			return s
		}
		if cwd == "" {
			return s
		}
		return filepath.Join(cwd, s)
	}
	var paths []string
	var dir string
	for i := 1; i < len(fields); i++ {
		flag := fields[i]
		var kind, value string
		switch {
		case flag == "-c" || flag == "--config" || flag == "-config" || flag == "-confdir" || flag == "--confdir":
			if i+1 >= len(fields) {
				return paths, dir, errors.New("missing Xray configuration flag value")
			}
			kind, value = flag, fields[i+1]
			i++
		case strings.HasPrefix(flag, "-c="):
			kind, value = "-c", strings.TrimPrefix(flag, "-c=")
		case strings.HasPrefix(flag, "-config="):
			kind, value = "-config", strings.TrimPrefix(flag, "-config=")
		case strings.HasPrefix(flag, "--config="):
			kind, value = "--config", strings.TrimPrefix(flag, "--config=")
		case strings.HasPrefix(flag, "-confdir="):
			kind, value = "-confdir", strings.TrimPrefix(flag, "-confdir=")
		case strings.HasPrefix(flag, "--confdir="):
			kind, value = "--confdir", strings.TrimPrefix(flag, "--confdir=")
		default:
			continue
		}
		if value == "" {
			continue
		}
		if kind == "-confdir" || kind == "--confdir" {
			dir = resolve(value)
		} else {
			paths = append(paths, resolve(value))
		}
	}
	return paths, dir, nil
}

// SortEndpoints provides deterministic rows even when reading merged configs.
func (c *Catalog) SortEndpoints() {
	sort.SliceStable(c.Inbounds, func(i, j int) bool { return c.Inbounds[i].Tag < c.Inbounds[j].Tag })
	sort.SliceStable(c.Outbounds, func(i, j int) bool { return c.Outbounds[i].Tag < c.Outbounds[j].Tag })
}
