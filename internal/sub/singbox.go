package sub

import (
	"errors"
	"strings"

	"github.com/goccy/go-json"
)

var errNoSingboxProxies = errors.New("no sing-box-compatible proxies found: sing-box has no xhttp, mKCP, VLESS encryption, Hysteria v1 or AmneziaWG")

// singboxOutbound converts one Mihomo proxy from the Clash builder into a sing-box
// outbound, or an endpoint for WireGuard; nil when sing-box cannot express it.
func singboxOutbound(p map[string]any) (out map[string]any, endpoint bool) {
	name, _ := p["name"].(string)
	server, _ := p["server"].(string)
	port := clashInt(p["port"])
	if name == "" || server == "" || port <= 0 {
		return nil, false
	}
	out = map[string]any{"tag": name, "server": server, "server_port": port}
	switch p["type"] {
	case "vmess":
		out["type"] = "vmess"
		out["uuid"] = p["uuid"]
		out["alter_id"] = 0
		if cipher, _ := p["cipher"].(string); cipher != "" {
			out["security"] = cipher
		}
	case "vless":
		if enc, _ := p["encryption"].(string); enc != "" {
			return nil, false
		}
		out["type"] = "vless"
		out["uuid"] = p["uuid"]
		if flow, _ := p["flow"].(string); flow != "" {
			out["flow"] = flow
		}
	case "trojan":
		out["type"] = "trojan"
		out["password"] = p["password"]
	case "ss":
		if _, plugin := p["plugin"]; plugin {
			return nil, false
		}
		out["type"] = "shadowsocks"
		out["method"] = p["cipher"]
		out["password"] = p["password"]
	case "socks5":
		out["type"] = "socks"
		return out, false
	case "hysteria2":
		return singboxHysteria2(p, out), false
	case "tuic":
		out["type"] = "tuic"
		out["uuid"] = p["uuid"]
		out["password"] = p["password"]
		if cc, _ := p["congestion-controller"].(string); cc != "" {
			out["congestion_control"] = cc
		}
		if mode, _ := p["udp-relay-mode"].(string); mode != "" {
			out["udp_relay_mode"] = mode
		}
		if zeroRTT, _ := p["reduce-rtt"].(bool); zeroRTT {
			out["zero_rtt_handshake"] = true
		}
		out["tls"] = singboxTLS(p, true)
		return out, false
	case "wireguard":
		return singboxWireguard(p), true
	default:
		return nil, false
	}

	if !singboxTransport(p, out) {
		return nil, false
	}
	if tlsOn, _ := p["tls"].(bool); tlsOn {
		out["tls"] = singboxTLS(p, true)
	}
	return out, false
}

func singboxTransport(p, out map[string]any) bool {
	network, _ := p["network"].(string)
	switch network {
	case "", "tcp":
		return true
	case "ws":
		t := map[string]any{"type": "ws"}
		opts, _ := p["ws-opts"].(map[string]any)
		if path, _ := opts["path"].(string); path != "" {
			t["path"] = path
		}
		if headers, _ := opts["headers"].(map[string]any); headers != nil {
			t["headers"] = headers
		}
		out["transport"] = t
		return true
	case "grpc":
		t := map[string]any{"type": "grpc"}
		opts, _ := p["grpc-opts"].(map[string]any)
		if name, _ := opts["grpc-service-name"].(string); name != "" {
			t["service_name"] = name
		}
		out["transport"] = t
		return true
	case "httpupgrade":
		t := map[string]any{"type": "httpupgrade"}
		opts, _ := p["http-upgrade-opts"].(map[string]any)
		if path, _ := opts["path"].(string); path != "" {
			t["path"] = path
		}
		if headers, _ := opts["headers"].(map[string]any); headers != nil {
			if host, _ := headers["Host"].(string); host != "" {
				t["host"] = host
			}
		}
		out["transport"] = t
		return true
	default:
		return false
	}
}

func singboxTLS(p map[string]any, enabled bool) map[string]any {
	t := map[string]any{"enabled": enabled}
	sni, _ := p["servername"].(string)
	if sni == "" {
		sni, _ = p["sni"].(string)
	}
	if sni != "" {
		t["server_name"] = sni
	}
	if insecure, _ := p["skip-cert-verify"].(bool); insecure {
		t["insecure"] = true
	}
	if alpn := clashStringList(p["alpn"]); len(alpn) > 0 {
		t["alpn"] = alpn
	}
	fingerprint, _ := p["client-fingerprint"].(string)
	if reality, _ := p["reality-opts"].(map[string]any); reality != nil {
		r := map[string]any{"enabled": true}
		if key, _ := reality["public-key"].(string); key != "" {
			r["public_key"] = key
		}
		if id, _ := reality["short-id"].(string); id != "" {
			r["short_id"] = id
		}
		t["reality"] = r
		if fingerprint == "" {
			fingerprint = "chrome"
		}
	}
	if fingerprint != "" {
		t["utls"] = map[string]any{"enabled": true, "fingerprint": fingerprint}
	}
	return t
}

func singboxHysteria2(p, out map[string]any) map[string]any {
	obfs, _ := p["obfs"].(string)
	if obfs != "" && obfs != "salamander" {
		return nil
	}
	out["type"] = "hysteria2"
	out["password"] = p["password"]
	if obfs == "salamander" {
		out["obfs"] = map[string]any{"type": "salamander", "password": p["obfs-password"]}
	}
	if ports, _ := p["ports"].(string); ports != "" {
		var ranges []string
		for part := range strings.SplitSeq(ports, ",") {
			if part = strings.TrimSpace(part); part == "" {
				continue
			}
			lo, hi, isRange := strings.Cut(part, "-")
			if !isRange {
				hi = lo
			}
			ranges = append(ranges, strings.TrimSpace(lo)+":"+strings.TrimSpace(hi))
		}
		if len(ranges) > 0 {
			out["server_ports"] = ranges
		}
	}
	out["tls"] = singboxTLS(p, true)
	return out
}

// singboxWireguard is an endpoint, the form sing-box 1.11+ takes WireGuard in.
func singboxWireguard(p map[string]any) map[string]any {
	name, _ := p["name"].(string)
	server, _ := p["server"].(string)
	var addresses []string
	if ip, _ := p["ip"].(string); ip != "" {
		addresses = append(addresses, ip+"/32")
	}
	if ip, _ := p["ipv6"].(string); ip != "" {
		addresses = append(addresses, ip+"/128")
	}
	if len(addresses) == 0 {
		return nil
	}
	peer := map[string]any{
		"address":     server,
		"port":        clashInt(p["port"]),
		"public_key":  p["public-key"],
		"allowed_ips": []string{"0.0.0.0/0", "::/0"},
	}
	if psk, _ := p["pre-shared-key"].(string); psk != "" {
		peer["pre_shared_key"] = psk
	}
	if ka := clashInt(p["persistent-keepalive"]); ka > 0 {
		peer["persistent_keepalive_interval"] = ka
	}
	ep := map[string]any{
		"type":        "wireguard",
		"tag":         name,
		"address":     addresses,
		"private_key": p["private-key"],
		"peers":       []any{peer},
	}
	if mtu := clashInt(p["mtu"]); mtu > 0 {
		ep["mtu"] = mtu
	}
	return ep
}

// singboxConfig is a complete client profile for sing-box 1.12+: TUN plus a
// local mixed port, a selector over an auto url-test, and DNS through the proxy.
func singboxConfig(proxies []map[string]any) ([]byte, int) {
	var outbounds, endpoints []any
	var tags, testable []string
	for _, p := range proxies {
		ob, isEndpoint := singboxOutbound(p)
		if ob == nil {
			continue
		}
		tag, _ := ob["tag"].(string)
		tags = append(tags, tag)
		if isEndpoint {
			endpoints = append(endpoints, ob)
		} else {
			outbounds = append(outbounds, ob)
		}
		if !isDummyProxy(p) {
			testable = append(testable, tag)
		}
	}
	if len(tags) == 0 {
		return nil, 0
	}
	choices := tags
	head := []any{}
	if len(testable) > 1 {
		head = append(head, map[string]any{
			"type": "urltest", "tag": "auto", "outbounds": testable,
			"url": "https://www.gstatic.com/generate_204", "interval": "10m",
		})
		choices = append([]string{"auto"}, tags...)
	}
	head = append([]any{map[string]any{"type": "selector", "tag": "proxy", "outbounds": choices, "default": choices[0]}}, head...)
	outbounds = append(head, append(outbounds, map[string]any{"type": "direct", "tag": "direct"})...)

	config := map[string]any{
		"log": map[string]any{"level": "warn"},
		"dns": map[string]any{
			"servers": []any{
				map[string]any{"type": "https", "tag": "remote", "server": "1.1.1.1", "detour": "proxy"},
				map[string]any{"type": "local", "tag": "local"},
			},
			"final": "remote",
		},
		"inbounds": []any{
			map[string]any{
				"type": "tun", "tag": "tun-in", "address": []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
				"auto_route": true, "strict_route": true,
			},
			map[string]any{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": 2080},
		},
		"outbounds": outbounds,
		"route": map[string]any{
			"rules": []any{
				map[string]any{"action": "sniff"},
				map[string]any{"protocol": "dns", "action": "hijack-dns"},
				map[string]any{"ip_is_private": true, "outbound": "direct"},
			},
			"final":                   "proxy",
			"auto_detect_interface":   true,
			"default_domain_resolver": "local",
		},
	}
	if len(endpoints) > 0 {
		config["endpoints"] = endpoints
	}
	body, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, 0
	}
	return body, len(tags)
}

func singboxSupported(p map[string]any) bool {
	ob, _ := singboxOutbound(p)
	return ob != nil
}

// GetSingbox renders the subscription as a sing-box profile from the same
// proxies the Clash output lists, minus those sing-box cannot carry.
func (s *SubClashService) GetSingbox(subId string, host string) (string, string, error) {
	keep := func(proxies []map[string]any) ([]map[string]any, error) {
		kept := make([]map[string]any, 0, len(proxies))
		for _, p := range proxies {
			if singboxSupported(p) {
				kept = append(kept, p)
			}
		}
		if len(kept) == 0 && len(proxies) > 0 {
			return nil, errNoSingboxProxies
		}
		return kept, nil
	}
	proxies, header, found, err := s.collectProxies(subId, host, keep)
	if err != nil || !found {
		return "", "", err
	}
	body, _ := singboxConfig(proxies)
	return string(body), header, nil
}
