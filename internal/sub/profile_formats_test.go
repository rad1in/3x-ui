package sub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

var (
	vmessWsTLS = map[string]any{
		"name": "n1", "type": "vmess", "server": "h.example", "port": 443, "uuid": "U", "cipher": "auto",
		"network": "ws", "tls": true, "servername": "sni.example", "skip-cert-verify": true,
		"ws-opts": map[string]any{"path": "/ws", "headers": map[string]any{"Host": "cdn.example"}},
	}
	vlessReality = map[string]any{
		"name": "n2", "type": "vless", "server": "r.example", "port": 443, "uuid": "U2", "flow": "xtls-rprx-vision",
		"network": "tcp", "tls": true, "servername": "real.sni",
		"reality-opts": map[string]any{"public-key": "PUB", "short-id": "abcd"},
	}
	trojanWs = map[string]any{
		"name": "n3", "type": "trojan", "server": "t.example", "port": 443, "password": "pw", "sni": "t.sni",
		"network": "ws", "ws-opts": map[string]any{"path": "/t"},
	}
	ss2022 = map[string]any{
		"name": "n4", "type": "ss", "server": "s.example", "port": 8388, "cipher": "2022-blake3-aes-128-gcm", "password": "K1:K2",
	}
	ssPlugin = map[string]any{
		"name": "n5", "type": "ss", "server": "s.example", "port": 8388, "cipher": "aes-128-gcm", "password": "pw", "plugin": "obfs",
	}
	hy2Plain = map[string]any{
		"name": "n6", "type": "hysteria2", "server": "y.example", "port": 8443, "password": "hp", "sni": "y.sni",
	}
	hy2Salamander = map[string]any{
		"name": "n7", "type": "hysteria2", "server": "y.example", "port": 8443, "password": "hp", "sni": "y.sni",
		"obfs": "salamander", "obfs-password": "op",
	}
	tuicV5 = map[string]any{
		"name": "n8", "type": "tuic", "server": "u.example", "port": 4443, "uuid": "TU", "password": "tp", "sni": "u.sni",
	}
	xhttpVless = map[string]any{
		"name": "n9", "type": "vless", "server": "x.example", "port": 443, "uuid": "U3", "network": "xhttp", "tls": true,
	}
	commaPassword = map[string]any{
		"name": "n10", "type": "trojan", "server": "c.example", "port": 443, "password": "a,b",
	}
)

func TestProfileNodeLines(t *testing.T) {
	cases := []struct {
		name  string
		proxy map[string]any
		surge string
		quanx string
		loon  string
	}{
		{
			"vmess over WebSocket and TLS", vmessWsTLS,
			`n1 = vmess, h.example, 443, username=U, vmess-aead=true, tls=true, sni=sni.example, skip-cert-verify=true, ws=true, ws-path=/ws, ws-headers=Host:cdn.example`,
			`vmess=h.example:443, method=chacha20-poly1305, password=U, obfs=wss, obfs-host=cdn.example, obfs-uri=/ws, tls-verification=false, fast-open=false, udp-relay=true, tag=n1`,
			`n1 = vmess,h.example,443,auto,"U",transport=ws,path=/ws,host=cdn.example,over-tls=true,tls-name=sni.example,skip-cert-verify=true,udp=true,alterId=0`,
		},
		{
			"VLESS Reality Vision", vlessReality,
			"",
			`vless=r.example:443, method=none, password=U2, obfs=over-tls, obfs-host=real.sni, reality-base64-pubkey=PUB, reality-hex-shortid=abcd, vless-flow=xtls-rprx-vision, fast-open=false, udp-relay=true, tag=n2`,
			`n2 = vless,r.example,443,"U2",transport=tcp,over-tls=true,sni=real.sni,public-key="PUB",short-id=abcd,flow=xtls-rprx-vision,udp=true`,
		},
		{
			"Trojan over WebSocket", trojanWs,
			`n3 = trojan, t.example, 443, password=pw, sni=t.sni, ws=true, ws-path=/t`,
			`trojan=t.example:443, password=pw, obfs=wss, obfs-host=t.sni, obfs-uri=/t, fast-open=false, udp-relay=true, tag=n3`,
			`n3 = trojan,t.example,443,"pw",tls-name=t.sni,transport=ws,path=/t,udp=true`,
		},
		{
			"Shadowsocks 2022", ss2022,
			`n4 = ss, s.example, 8388, encrypt-method=2022-blake3-aes-128-gcm, password=K1:K2, udp-relay=true`,
			`shadowsocks=s.example:8388, method=2022-blake3-aes-128-gcm, password=K1:K2, fast-open=false, udp-relay=true, tag=n4`,
			`n4 = Shadowsocks,s.example,8388,2022-blake3-aes-128-gcm,"K1:K2",udp=true`,
		},
		{"Shadowsocks with a plugin", ssPlugin, "", "", ""},
		{
			"Hysteria2 without obfuscation", hy2Plain,
			`n6 = hysteria2, y.example, 8443, password=hp, sni=y.sni`,
			"",
			`n6 = Hysteria2,y.example,8443,"hp",tls-name=y.sni,udp=true`,
		},
		{
			"Hysteria2 with Salamander", hy2Salamander,
			"",
			"",
			`n7 = Hysteria2,y.example,8443,"hp",tls-name=y.sni,salamander-password=op,udp=true`,
		},
		{
			"TUIC v5", tuicV5,
			`n8 = tuic-v5, u.example, 4443, uuid=TU, password=tp, sni=u.sni`,
			"", "",
		},
		{"xhttp transport", xhttpVless, "", "", ""},
		{"a password with a comma", commaPassword, "", "", ""},
	}
	formats := []struct {
		name string
		node func(map[string]any, string) (string, bool)
		want func(int) string
	}{
		{"Surge", surgeNode, func(i int) string { return cases[i].surge }},
		{"Quantumult X", quanxNode, func(i int) string { return cases[i].quanx }},
		{"Loon", loonNode, func(i int) string { return cases[i].loon }},
	}
	for i, tc := range cases {
		for _, f := range formats {
			t.Run(tc.name+"/"+f.name, func(t *testing.T) {
				got, ok := f.node(tc.proxy, tc.proxy["name"].(string))
				want := f.want(i)
				if want == "" {
					if ok {
						t.Fatalf("%s cannot carry this proxy but emitted %q", f.name, got)
					}
					return
				}
				if !ok || got != want {
					t.Fatalf("got %q (ok=%v)\nwant %q", got, ok, want)
				}
			})
		}
	}
}

func TestProfileNameSanitizingAndDedupe(t *testing.T) {
	proxies := []map[string]any{
		{"name": "a, b = c", "type": "ss", "server": "s", "port": 1, "cipher": "aes-128-gcm", "password": "p"},
		{"name": "a, b = c", "type": "ss", "server": "s", "port": 2, "cipher": "aes-128-gcm", "password": "p"},
	}
	nodes := profileNodes(proxies, loonNode)
	if len(nodes) != 2 || nodes[0].name != "a b c" || nodes[1].name != "a b c 2" {
		t.Fatalf("nodes = %+v, want sanitized unique names", nodes)
	}
}

func TestProfileDocuments(t *testing.T) {
	nodes := profileNodes([]map[string]any{ss2022, hy2Plain}, surgeNode)
	surge := renderSurge(nodes, profileOptions{managedURL: "https://sub.example/surge/s1", intervalSeconds: 43200})
	for _, want := range []string{
		"#!MANAGED-CONFIG https://sub.example/surge/s1 interval=43200 strict=false",
		"[Proxy]\nn4 = ss, s.example, 8388,",
		"[Proxy Group]\nProxy = select, Auto, n4, n6\nAuto = url-test, n4, n6, url=http://www.gstatic.com/generate_204, interval=600, tolerance=100",
		"[Rule]\n",
		"FINAL,Proxy",
	} {
		if !strings.Contains(surge, want) {
			t.Fatalf("Surge profile lacks %q:\n%s", want, surge)
		}
	}
	loon := renderLoon(profileNodes([]map[string]any{ss2022}, loonNode), profileOptions{})
	for _, want := range []string{"[Proxy]\nn4 = Shadowsocks,", "[Proxy Group]\nProxy = select,n4\n", "[Rule]\nIP-CIDR,10.0.0.0/8,DIRECT,no-resolve\n", "\nFINAL,Proxy\n"} {
		if !strings.Contains(loon, want) {
			t.Fatalf("Loon profile lacks %q:\n%s", want, loon)
		}
	}
	if strings.Contains(loon, "url-test") {
		t.Fatalf("a single node needs no url-test group:\n%s", loon)
	}
	quanx := renderQuanX(profileNodes([]map[string]any{ss2022, tuicV5}, quanxNode))
	if quanx != "shadowsocks=s.example:8388, method=2022-blake3-aes-128-gcm, password=K1:K2, fast-open=false, udp-relay=true, tag=n4\n" {
		t.Fatalf("Quantumult X list = %q", quanx)
	}
}

func TestProfileAliasesServeFromTheSubscription(t *testing.T) {
	seedSubDB(t)
	seedSingboxSub(t, "s1")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewSUBController(router.Group("/"), WithSUBClashEnabled(true), WithSUBUpdateInterval("12"))
	get := func(path string) *httptest.ResponseRecorder {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "http://sub.example.com"+path, nil))
		return resp
	}

	surge := get("/surge/s1")
	if surge.Code != http.StatusOK {
		t.Fatalf("surge = %d: %s", surge.Code, surge.Body.String())
	}
	if first, _, _ := strings.Cut(surge.Body.String(), "\n"); first != "#!MANAGED-CONFIG http://sub.example.com/surge/s1 interval=43200 strict=false" {
		t.Fatalf("managed config line = %q", first)
	}
	if !strings.Contains(surge.Body.String(), "ss = ss, 203.0.113.5, 10005, encrypt-method=2022-blake3-aes-128-gcm, password=AAAAAAAAAAAAAAAAAAAAAA==:BBBBBBBBBBBBBBBBBBBBBB==, udp-relay=true") {
		t.Fatalf("Surge profile lacks the Shadowsocks 2022 node:\n%s", surge.Body.String())
	}
	if strings.Contains(surge.Body.String(), "vless") {
		t.Fatalf("Surge has no VLESS:\n%s", surge.Body.String())
	}

	quanx := get("/quanx/s1")
	if quanx.Code != http.StatusOK || !strings.Contains(quanx.Body.String(), "vless=203.0.113.5:10002, method=none, password=aaaaaaaa-0000-4000-8000-000000000002, obfs=over-tls, obfs-host=real.sni, reality-base64-pubkey=Z84J2IelR9ch3k8VtlVhhs5ycBUlXA7wHBWcBrjqnAw, reality-hex-shortid=abcd,") {
		t.Fatalf("quanx = %d:\n%s", quanx.Code, quanx.Body.String())
	}

	loon := get("/loon/s1")
	if loon.Code != http.StatusOK || !strings.Contains(loon.Body.String(), `hy = Hysteria2,203.0.113.5,10006,"hyauth",tls-name=hy.sni,salamander-password=obfspw,udp=true`) {
		t.Fatalf("loon = %d:\n%s", loon.Code, loon.Body.String())
	}

	if got := get("/surge/nope"); got.Code != http.StatusNotFound {
		t.Fatalf("unknown subscription = %d, want 404", got.Code)
	}
	for _, path := range []string{"/surge/s1", "/quanx/s1", "/loon/s1"} {
		if ct := get(path).Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
			t.Fatalf("%s Content-Type = %q, want text/plain", path, ct)
		}
	}
}
