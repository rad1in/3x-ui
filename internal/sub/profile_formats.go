package sub

import (
	"errors"
	"strconv"
	"strings"
)

var errNoProfileProxies = errors.New("no compatible proxies found for this app")

const profileTestURL = "http://www.gstatic.com/generate_204"

// profileNode is one proxy rendered for a Surge, Quantumult X or Loon profile.
type profileNode struct {
	name string
	line string
	// probe is false for the usage info node, which must stay out of url-test groups.
	probe bool
}

type profileOptions struct {
	managedURL      string
	intervalSeconds int
}

type profileNodeFunc func(p map[string]any, name string) (string, bool)

// profileName strips what these apps use as separators so a remark cannot split a line.
func profileName(name string) string {
	clean := strings.Join(strings.Fields(strings.NewReplacer(",", " ", "=", " ", `"`, " ").Replace(name)), " ")
	if clean == "" {
		return "proxy"
	}
	return clean
}

// profileNodes converts the proxies a format can carry, giving each a unique name.
func profileNodes(proxies []map[string]any, node profileNodeFunc) []profileNode {
	seen := map[string]int{}
	var out []profileNode
	for _, p := range proxies {
		raw, _ := p["name"].(string)
		name := profileName(raw)
		n := seen[name]
		seen[name] = n + 1
		if n > 0 {
			name += " " + strconv.Itoa(n+1)
		}
		line, ok := node(p, name)
		if !ok {
			continue
		}
		out = append(out, profileNode{name: name, line: line, probe: !isDummyProxy(p)})
	}
	return out
}

// plainValue reports whether v can sit in a comma-separated, unquoted field.
func plainValue(v string) bool {
	return !strings.ContainsAny(v, ",\r\n\"")
}

func str(p map[string]any, key string) string {
	s, _ := p[key].(string)
	return s
}

func flag(p map[string]any, key string) bool {
	b, _ := p[key].(bool)
	return b
}

// profileTransport reads the Clash transport of a proxy; ok is false for those
// these apps cannot dial (gRPC, xhttp, HTTP upgrade).
func profileTransport(p map[string]any) (ws bool, path, host string, ok bool) {
	switch str(p, "network") {
	case "", "tcp":
		return false, "", "", true
	case "ws":
		opts, _ := p["ws-opts"].(map[string]any)
		headers, _ := opts["headers"].(map[string]any)
		host, _ = headers["Host"].(string)
		path, _ = opts["path"].(string)
		return true, path, host, plainValue(path) && plainValue(host)
	}
	return false, "", "", false
}

func profileSNI(p map[string]any) string {
	if sni := str(p, "servername"); sni != "" {
		return sni
	}
	return str(p, "sni")
}

func reality(p map[string]any) (pub, shortID string, on bool) {
	opts, _ := p["reality-opts"].(map[string]any)
	if opts == nil {
		return "", "", false
	}
	pub, _ = opts["public-key"].(string)
	shortID, _ = opts["short-id"].(string)
	return pub, shortID, true
}

func allPlain(values ...string) bool {
	for _, v := range values {
		if !plainValue(v) {
			return false
		}
	}
	return true
}

// vmessMethod maps a Clash VMess cipher to the method names Quantumult X and Loon accept.
func vmessMethod(cipher string) string {
	switch cipher {
	case "aes-128-gcm", "none":
		return cipher
	case "chacha20-poly1305", "chacha20-ietf-poly1305":
		return "chacha20-poly1305"
	}
	return ""
}

// surgeNode renders a Surge policy; Surge has no VLESS, Reality, gRPC or obfuscated Hysteria 2.
func surgeNode(p map[string]any, name string) (string, bool) {
	server, port := str(p, "server"), strconv.Itoa(clashInt(p["port"]))
	head := name + " = "
	if server == "" || clashInt(p["port"]) <= 0 || !plainValue(server) {
		return "", false
	}
	tlsParams := func(sni string, insecure bool) string {
		out := ""
		if sni != "" {
			out += ", sni=" + sni
		}
		if insecure {
			out += ", skip-cert-verify=true"
		}
		return out
	}
	wsParams := func(path, host string) string {
		out := ", ws=true"
		if path != "" {
			out += ", ws-path=" + path
		}
		if host != "" {
			out += ", ws-headers=Host:" + host
		}
		return out
	}
	insecure := flag(p, "skip-cert-verify")
	switch str(p, "type") {
	case "ss":
		if p["plugin"] != nil || !allPlain(str(p, "cipher"), str(p, "password")) {
			return "", false
		}
		return head + "ss, " + server + ", " + port + ", encrypt-method=" + str(p, "cipher") + ", password=" + str(p, "password") + ", udp-relay=true", true
	case "vmess":
		ws, path, host, ok := profileTransport(p)
		if !ok || !allPlain(str(p, "uuid"), profileSNI(p)) {
			return "", false
		}
		line := head + "vmess, " + server + ", " + port + ", username=" + str(p, "uuid")
		if m := vmessMethod(str(p, "cipher")); m == "aes-128-gcm" {
			line += ", encrypt-method=aes-128-gcm"
		} else if m == "chacha20-poly1305" {
			line += ", encrypt-method=chacha20-ietf-poly1305"
		}
		line += ", vmess-aead=true"
		if flag(p, "tls") {
			line += ", tls=true" + tlsParams(profileSNI(p), insecure)
		}
		if ws {
			line += wsParams(path, host)
		}
		return line, true
	case "trojan":
		ws, path, host, ok := profileTransport(p)
		if !ok || !allPlain(str(p, "password"), profileSNI(p)) {
			return "", false
		}
		line := head + "trojan, " + server + ", " + port + ", password=" + str(p, "password") + tlsParams(profileSNI(p), insecure)
		if ws {
			line += wsParams(path, host)
		}
		return line, true
	case "hysteria2":
		if str(p, "obfs") != "" || !allPlain(str(p, "password"), profileSNI(p)) {
			return "", false
		}
		return head + "hysteria2, " + server + ", " + port + ", password=" + str(p, "password") + tlsParams(profileSNI(p), insecure), true
	case "tuic":
		if !allPlain(str(p, "uuid"), str(p, "password"), profileSNI(p)) {
			return "", false
		}
		return head + "tuic-v5, " + server + ", " + port + ", uuid=" + str(p, "uuid") + ", password=" + str(p, "password") + tlsParams(profileSNI(p), insecure), true
	case "socks5":
		return socksNode(head, "socks5", server, port, p, "socks5-tls", ", ", ", ", false)
	}
	return "", false
}

// socksNode renders a SOCKS5 policy; tlsKeyword is non-empty for formats with a TLS variant keyword.
func socksNode(head, keyword, server, port string, p map[string]any, tlsKeyword, sep, passSep string, quotePass bool) (string, bool) {
	user, pass := str(p, "username"), str(p, "password")
	if !allPlain(user, pass) {
		return "", false
	}
	if flag(p, "tls") && tlsKeyword != "" {
		keyword = tlsKeyword
	}
	line := head + keyword + sep + server + sep + port
	if user != "" {
		if quotePass {
			pass = `"` + pass + `"`
		}
		line += sep + user + passSep + pass
	}
	return line, true
}

// quanxNode renders a Quantumult X server_local entry; it has no gRPC, xhttp, Hysteria or TUIC.
func quanxNode(p map[string]any, name string) (string, bool) {
	server := str(p, "server")
	if server == "" || clashInt(p["port"]) <= 0 || !plainValue(server) {
		return "", false
	}
	addr := server + ":" + strconv.Itoa(clashInt(p["port"]))
	tail := ", fast-open=false, udp-relay=true, tag=" + name
	verify := ""
	if flag(p, "skip-cert-verify") {
		verify = ", tls-verification=false"
	}
	// obfs mode and host: ws/wss carry the Host header (also the SNI under TLS), plain TLS carries the SNI.
	obfs := func(ws bool, path, host string) (string, bool) {
		sni := profileSNI(p)
		if host == "" {
			host = sni
		}
		tlsOn := flag(p, "tls")
		out := ""
		switch {
		case ws && tlsOn:
			out = ", obfs=wss"
		case ws:
			out = ", obfs=ws"
		case tlsOn:
			out = ", obfs=over-tls"
		}
		if (ws || tlsOn) && host != "" {
			out += ", obfs-host=" + host
		}
		if ws && path != "" {
			out += ", obfs-uri=" + path
		}
		if pub, shortID, on := reality(p); on && tlsOn {
			out += ", reality-base64-pubkey=" + pub + ", reality-hex-shortid=" + shortID
		}
		return out, allPlain(host, path)
	}
	switch str(p, "type") {
	case "ss":
		if p["plugin"] != nil || !allPlain(str(p, "cipher"), str(p, "password")) {
			return "", false
		}
		return "shadowsocks=" + addr + ", method=" + str(p, "cipher") + ", password=" + str(p, "password") + tail, true
	case "vmess":
		ws, path, host, ok := profileTransport(p)
		method := vmessMethod(str(p, "cipher"))
		if str(p, "cipher") == "auto" {
			method = "chacha20-poly1305"
		}
		o, plain := obfs(ws, path, host)
		if !ok || method == "" || !plain || !allPlain(str(p, "uuid")) {
			return "", false
		}
		return "vmess=" + addr + ", method=" + method + ", password=" + str(p, "uuid") + o + verify + tail, true
	case "vless":
		ws, path, host, ok := profileTransport(p)
		o, plain := obfs(ws, path, host)
		if !ok || !plain || !allPlain(str(p, "uuid")) || str(p, "encryption") != "" {
			return "", false
		}
		flow := ""
		if f := str(p, "flow"); f == "xtls-rprx-vision" {
			flow = ", vless-flow=" + f
		} else if f != "" {
			return "", false
		}
		return "vless=" + addr + ", method=none, password=" + str(p, "uuid") + o + flow + verify + tail, true
	case "trojan":
		ws, path, host, ok := profileTransport(p)
		if !ok || !allPlain(str(p, "password")) {
			return "", false
		}
		sni := profileSNI(p)
		if host == "" {
			host = sni
		}
		if !allPlain(host, path) {
			return "", false
		}
		line := "trojan=" + addr + ", password=" + str(p, "password")
		if ws {
			line += ", obfs=wss"
			if host != "" {
				line += ", obfs-host=" + host
			}
			if path != "" {
				line += ", obfs-uri=" + path
			}
		} else {
			line += ", over-tls=true"
			if sni != "" {
				line += ", tls-host=" + sni
			}
		}
		return line + verify + tail, true
	case "socks5":
		user, pass := str(p, "username"), str(p, "password")
		if !allPlain(user, pass) {
			return "", false
		}
		line := "socks5=" + addr
		if user != "" {
			line += ", username=" + user + ", password=" + pass
		}
		if flag(p, "tls") {
			line += ", over-tls=true"
		}
		return line + tail, true
	}
	return "", false
}

// loonNode renders a Loon [Proxy] entry; it has no gRPC, xhttp or TUIC.
func loonNode(p map[string]any, name string) (string, bool) {
	server := str(p, "server")
	if server == "" || clashInt(p["port"]) <= 0 || !plainValue(server) {
		return "", false
	}
	head := name + " = "
	base := server + "," + strconv.Itoa(clashInt(p["port"]))
	sni := profileSNI(p)
	if !plainValue(sni) {
		return "", false
	}
	insecure := ""
	if flag(p, "skip-cert-verify") {
		insecure = ",skip-cert-verify=true"
	}
	transport := func(ws bool, path, host string) string {
		if !ws {
			return ",transport=tcp"
		}
		out := ",transport=ws"
		if path != "" {
			out += ",path=" + path
		}
		if host != "" {
			out += ",host=" + host
		}
		return out
	}
	switch str(p, "type") {
	case "ss":
		if p["plugin"] != nil || !allPlain(str(p, "cipher"), str(p, "password")) {
			return "", false
		}
		return head + "Shadowsocks," + base + "," + str(p, "cipher") + `,"` + str(p, "password") + `",udp=true`, true
	case "vmess":
		ws, path, host, ok := profileTransport(p)
		method := vmessMethod(str(p, "cipher"))
		if str(p, "cipher") == "auto" {
			method = "auto"
		}
		if !ok || method == "" || !allPlain(str(p, "uuid")) {
			return "", false
		}
		line := head + "vmess," + base + "," + method + `,"` + str(p, "uuid") + `"` + transport(ws, path, host)
		if flag(p, "tls") {
			line += ",over-tls=true"
			if sni != "" {
				line += ",tls-name=" + sni
			}
			line += insecure
		}
		return line + ",udp=true,alterId=0", true
	case "vless":
		ws, path, host, ok := profileTransport(p)
		if !ok || !allPlain(str(p, "uuid")) || str(p, "encryption") != "" {
			return "", false
		}
		line := head + "vless," + base + `,"` + str(p, "uuid") + `"` + transport(ws, path, host)
		if flag(p, "tls") {
			line += ",over-tls=true"
			if pub, shortID, on := reality(p); on {
				if sni != "" {
					line += ",sni=" + sni
				}
				line += `,public-key="` + pub + `"`
				if shortID != "" {
					line += ",short-id=" + shortID
				}
			} else {
				if sni != "" {
					line += ",tls-name=" + sni
				}
				line += insecure
			}
		}
		if f := str(p, "flow"); f == "xtls-rprx-vision" {
			line += ",flow=" + f
		} else if f != "" {
			return "", false
		}
		return line + ",udp=true", true
	case "trojan":
		ws, path, host, ok := profileTransport(p)
		if !ok || !allPlain(str(p, "password")) {
			return "", false
		}
		line := head + "trojan," + base + `,"` + str(p, "password") + `"`
		if sni != "" {
			line += ",tls-name=" + sni
		}
		line += insecure
		if ws {
			line += transport(ws, path, host)
		}
		return line + ",udp=true", true
	case "hysteria2":
		if !allPlain(str(p, "password")) {
			return "", false
		}
		line := head + "Hysteria2," + base + `,"` + str(p, "password") + `"`
		if sni != "" {
			line += ",tls-name=" + sni
		}
		line += insecure
		switch obfs := str(p, "obfs"); obfs {
		case "":
		case "salamander":
			if !allPlain(str(p, "obfs-password")) {
				return "", false
			}
			line += ",salamander-password=" + str(p, "obfs-password")
		default:
			return "", false
		}
		return line + ",udp=true", true
	case "socks5":
		line, ok := socksNode(head, "socks5", server, strconv.Itoa(clashInt(p["port"])), p, "", ",", ",", true)
		if ok && flag(p, "tls") {
			line += ",over-tls=true"
		}
		return line, ok
	}
	return "", false
}

func nodeNames(nodes []profileNode, probeOnly bool) []string {
	var names []string
	for _, n := range nodes {
		if !probeOnly || n.probe {
			names = append(names, n.name)
		}
	}
	return names
}

// proxyGroups lists the names of the select group's choices and, when more than
// one node can be probed, the url-test group they share.
func proxyGroups(nodes []profileNode) (choices, probed []string) {
	probed = nodeNames(nodes, true)
	choices = nodeNames(nodes, false)
	if len(probed) > 1 {
		choices = append([]string{"Auto"}, choices...)
	}
	return choices, probed
}

var privateCIDRs = []string{"10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16"}

func renderSurge(nodes []profileNode, opts profileOptions) string {
	var b strings.Builder
	if opts.managedURL != "" {
		b.WriteString("#!MANAGED-CONFIG " + opts.managedURL + " interval=" + strconv.Itoa(opts.intervalSeconds) + " strict=false\n\n")
	}
	b.WriteString("[General]\nskip-proxy = 127.0.0.1, 192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12, localhost, *.local\n\n[Proxy]\n")
	for _, n := range nodes {
		b.WriteString(n.line + "\n")
	}
	choices, probed := proxyGroups(nodes)
	b.WriteString("\n[Proxy Group]\nProxy = select, " + strings.Join(choices, ", ") + "\n")
	if len(probed) > 1 {
		b.WriteString("Auto = url-test, " + strings.Join(probed, ", ") + ", url=" + profileTestURL + ", interval=600, tolerance=100\n")
	}
	b.WriteString("\n[Rule]\n")
	for _, cidr := range privateCIDRs {
		b.WriteString("IP-CIDR," + cidr + ",DIRECT,no-resolve\n")
	}
	b.WriteString("FINAL,Proxy\n")
	return b.String()
}

func renderLoon(nodes []profileNode, _ profileOptions) string {
	var b strings.Builder
	b.WriteString("[General]\nskip-proxy = 127.0.0.1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,localhost,*.local\n\n[Proxy]\n")
	for _, n := range nodes {
		b.WriteString(n.line + "\n")
	}
	choices, probed := proxyGroups(nodes)
	b.WriteString("\n[Proxy Group]\nProxy = select," + strings.Join(choices, ",") + "\n")
	if len(probed) > 1 {
		b.WriteString("Auto = url-test," + strings.Join(probed, ",") + ",url = " + profileTestURL + ",interval = 600,tolerance = 100\n")
	}
	b.WriteString("\n[Rule]\n")
	for _, cidr := range privateCIDRs {
		b.WriteString("IP-CIDR," + cidr + ",DIRECT,no-resolve\n")
	}
	b.WriteString("FINAL,Proxy\n")
	return b.String()
}

// renderQuanX is a node list, the form a Quantumult X server_remote resource reads.
func renderQuanX(nodes []profileNode) string {
	var b strings.Builder
	for _, n := range nodes {
		b.WriteString(n.line + "\n")
	}
	return b.String()
}

// profileFormat describes one alias path and how it renders.
type profileFormat struct {
	path   string
	node   profileNodeFunc
	render func([]profileNode, profileOptions) string
}

var (
	surgeFormat = profileFormat{"/surge/", surgeNode, renderSurge}
	quanxFormat = profileFormat{"/quanx/", quanxNode, func(n []profileNode, _ profileOptions) string { return renderQuanX(n) }}
	loonFormat  = profileFormat{"/loon/", loonNode, renderLoon}

	profileFormats = []profileFormat{surgeFormat, quanxFormat, loonFormat}
)

// GetProfile renders the subscription for one app from the proxies the Clash output lists.
func (s *SubClashService) GetProfile(f profileFormat, subId, host string, opts profileOptions) (string, string, error) {
	var nodes []profileNode
	keep := func(proxies []map[string]any) ([]map[string]any, error) {
		nodes = profileNodes(proxies, f.node)
		if len(nodes) == 0 && len(proxies) > 0 {
			return nil, errNoProfileProxies
		}
		return proxies, nil
	}
	_, header, found, err := s.collectProxies(subId, host, keep)
	if err != nil || !found {
		return "", "", err
	}
	return f.render(nodes, opts), header, nil
}
