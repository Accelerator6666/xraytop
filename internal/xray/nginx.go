package xray

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// NginxRoute indicates a NGINX listener with a reverse-proxy target that
// matches an Xray configured inbound. A port may map to several inbounds.
// It is NOT proof that any particular TCP connection authenticated with Xray.
type NginxRoute struct {
	Port      uint16
	Inbound   string
	Transport string
}

type NginxIngress struct {
	CheckedAt time.Time
	Ports     []uint16
	Routes    []NginxRoute
	// Locations are URL-path-to-inbound matches, for the combined access log.
	// They are configuration metadata, not proof of authenticated VLESS traffic.
	Locations []NginxLocation
}

type NginxLocation struct {
	Pattern   string
	Exact     bool
	Inbound   string
	Transport string
}

// DiscoverNginxIngress parses the on-disk NGINX config and its include files
// without spawning nginx -T. No certificates, pid files, or NGINX logs are
// opened by the discovery process. This is intentionally configuration-state
// inspection, not proof that an edited config was loaded by running NGINX.
func DiscoverNginxIngress(ctx context.Context,catalog Catalog) (NginxIngress,error) {
    return DiscoverNginxIngressFromFile(ctx,catalog,"/etc/nginx/nginx.conf")
}

func DiscoverNginxIngressFromFile(ctx context.Context,catalog Catalog,configPath string) (NginxIngress,error) {
    if len(catalog.Inbounds)==0 {return NginxIngress{},errors.New("Xray inbound config unavailable for NGINX matching")}
    nodes,err:=ReadNginxConfigTree(ctx,configPath)
    if err!=nil {return NginxIngress{},fmt.Errorf("NGINX read-only config: %w",err)}
    result:=parseNginxNodes(nodes,catalog)
    result.CheckedAt=time.Now()
    if len(result.Ports)==0 {return result,errors.New("no NGINX reverse-proxy targets match Xray inbound listen endpoints")}
    return result,nil
}

type nginxNode struct {
	name     string
	args     []string
	children []*nginxNode
}

// nginxLex tokenizes directives and brace-delimited blocks; quotes and
// comments are respected so quoted paths and # values are not misparsed.
func nginxLex(data []byte) []string {
	out := make([]string, 0, 256)
	var word strings.Builder
	var quote byte
	comment, escaped := false, false
	flush := func() {
		if word.Len() != 0 {
			out = append(out, word.String())
			word.Reset()
		}
	}
	for _, c := range data {
		if comment {
			if c == '\n' { comment = false }
			continue
		}
		if quote != 0 {
			if escaped {
				word.WriteByte(c)
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			} else {
				word.WriteByte(c)
			}
			continue
		}
		switch c {
		case '#':
			flush()
			comment = true
		case '"', '\'':
			quote = c
		case ' ', '\t', '\r', '\n':
			flush()
		case '{', '}', ';':
			flush()
			out = append(out, string(c))
		default:
			word.WriteByte(c)
		}
	}
	flush()
	return out
}

func nginxTree(tokens []string) []*nginxNode {
	var root []*nginxNode
	stack := []*[]*nginxNode{&root}
	var words []string
	commit := func(block bool) {
		if len(words) == 0 { return }
		node := &nginxNode{name: words[0], args: append([]string(nil), words[1:]...)}
		*stack[len(stack)-1] = append(*stack[len(stack)-1], node)
		words = nil
		if block { stack = append(stack, &node.children) }
	}
	for _, tok := range tokens {
		switch tok {
		case ";":
			commit(false)
		case "{":
			commit(true)
		case "}":
			words = nil
			if len(stack) > 1 { stack = stack[:len(stack)-1] }
		default:
			words = append(words, tok)
		}
	}
	return root
}

func nginxDescendants(nodes []*nginxNode, name string, result *[]*nginxNode) {
	for _, n := range nodes {
		if n.name == name { *result = append(*result, n) }
		nginxDescendants(n.children, name, result)
	}
}

func nginxPort(raw string) (uint16, bool) {
	if strings.HasPrefix(raw, "unix:") { return 0, false }
	port := raw
	if strings.Contains(raw, ":") {
		if strings.HasPrefix(raw, "[") {
			_, p, err := net.SplitHostPort(raw)
			if err != nil { return 0, false }
			port = p
		} else {
			_, p, ok := strings.Cut(raw, ":")
			if !ok { return 0, false }
			port = p
		}
	}
	value, err := strconv.ParseUint(port, 10, 16)
	return uint16(value), err == nil && value != 0
}

func nginxBackend(raw string) string {
	// An upstream reference like http://xray_ws/path must resolve through the
	// upstream block. A unix: target is compared by exact socket path.
	if strings.HasPrefix(raw, "unix:") { return raw }
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

// ParseNginxIngress determines proxy-bearing NGINX listeners by exact backend
// matching to configured Xray inbound sockets or loopback TCP endpoints.
// Never infer an Xray service from port number, server_name or upstream name.
func ParseNginxIngress(data []byte, catalog Catalog) NginxIngress {
	return parseNginxNodes(nginxTree(nginxLex(data)),catalog)
}

// parseNginxNodes accepts already-expanded NGINX includes without materializing
// the full effective config or copying private config contents to logs.
func parseNginxNodes(root []*nginxNode,catalog Catalog) NginxIngress {
	var upstreams []*nginxNode
	nginxDescendants(root, "upstream", &upstreams)
	aliases := make(map[string][]string)
	for _, node := range upstreams {
		if len(node.args) == 0 { continue }
		for _, child := range node.children {
			if child.name == "server" && len(child.args) > 0 {
				aliases[node.args[0]] = append(aliases[node.args[0]], child.args[0])
			}
		}
	}

	// Match only specific endpoints; never infer an inbound protocol from
	// an upstream's arbitrary name (for example "xray_ws_9930").
	byBackend := make(map[string][]Endpoint)
	for _, inbound := range catalog.Inbounds {
		if strings.HasPrefix(inbound.Listen, "unix:") || strings.HasPrefix(inbound.Listen, "127.0.0.1:") ||
			strings.HasPrefix(inbound.Listen, "[::1]:") {
			byBackend[inbound.Listen] = append(byBackend[inbound.Listen], inbound)
		}
	}
	matches := func(raw string) []Endpoint {
		target := nginxBackend(raw)
		backends := []string{target}
		if resolved, ok := aliases[target]; ok {
			backends = resolved
		}
		var found []Endpoint
		seen := make(map[string]bool)
		for _, backend := range backends {
			// NGINX unix upstreams use the same unix:path convention as the
			// Xray catalog; loopback TCP addresses are matched literally.
			if strings.HasPrefix(backend, "unix:") {
				backend = strings.TrimSuffix(backend, ":")
			}
			for _, inbound := range byBackend[backend] {
				if !seen[inbound.Tag] {
					seen[inbound.Tag] = true
					found = append(found, inbound)
				}
			}
		}
		return found
	}

	var servers []*nginxNode
	nginxDescendants(root, "server", &servers)
	routeKeys := make(map[string]bool)
	ports := make(map[uint16]bool)
	var routes []NginxRoute
	var locations []NginxLocation
	locationKeys := make(map[string]bool)
	for _, server := range servers {
		var listenPorts []uint16
		for _, child := range server.children {
			if child.name == "listen" && len(child.args) > 0 {
				if p, ok := nginxPort(child.args[0]); ok { listenPorts = append(listenPorts, p) }
			}
		}
		if len(listenPorts) == 0 { continue }
		var proxyDirectives []*nginxNode
		nginxDescendants(server.children, "proxy_pass", &proxyDirectives)
		nginxDescendants(server.children, "grpc_pass", &proxyDirectives)
		for _, directive := range proxyDirectives {
			if len(directive.args) == 0 { continue }
			for _, inbound := range matches(directive.args[0]) {
				for _, port := range listenPorts {
					key := fmt.Sprintf("%d\x00%s", port, inbound.Tag)
					if routeKeys[key] { continue }
					routeKeys[key] = true
					routes = append(routes, NginxRoute{
						Port: port, Inbound: inbound.Tag, Transport: inbound.Transport,
					})
					ports[port] = true
				}
			}
		}
		// Access logs use URI but default combined logs do not record $server_port.
		// Keep a location only when its backend matches a configured Xray
		// inbound exactly; later matching rejects ambiguous location mappings.
		var locs []*nginxNode
		nginxDescendants(server.children, "location", &locs)
		for _, loc := range locs {
			if len(loc.args) == 0 { continue }
			pattern, exact := loc.args[0], false
			switch pattern {
			case "=", "^~":
				if len(loc.args) < 2 { continue }
				exact = pattern == "="
				pattern = loc.args[1]
			case "~", "~*":
				continue // Regular-expression locations cannot be matched reliably here.
			}
			if !strings.HasPrefix(pattern, "/") { continue }
			var passes []*nginxNode
			nginxDescendants(loc.children, "proxy_pass", &passes)
			nginxDescendants(loc.children, "grpc_pass", &passes)
			for _, pass := range passes {
				if len(pass.args) == 0 { continue }
				for _, inbound := range matches(pass.args[0]) {
					key := fmt.Sprintf("%s\x00%t\x00%s", pattern, exact, inbound.Tag)
					if locationKeys[key] { continue }
					locationKeys[key] = true
					locations = append(locations, NginxLocation{Pattern:pattern,Exact:exact,Inbound:inbound.Tag,Transport:inbound.Transport})
				}
			}
		}
	}
	result := NginxIngress{Routes: routes, Locations: locations}
	for p := range ports { result.Ports = append(result.Ports, p) }
	sort.Slice(result.Ports, func(i, j int) bool { return result.Ports[i] < result.Ports[j] })
	sort.Slice(result.Routes, func(i, j int) bool {
		if result.Routes[i].Port != result.Routes[j].Port { return result.Routes[i].Port < result.Routes[j].Port }
		return result.Routes[i].Inbound < result.Routes[j].Inbound
	})
	return result
}
